package transitions

// APNs delivery over token-based auth (.p8 key). Pure standard library:
// an ES256 JWT provider token (cached ~40 min, Apple allows 20-60) and an
// HTTP/2 POST to api[.sandbox].push.apple.com. Payload is the same
// content-light tickle as Web Push plus a generic alert string so the OS
// has something to show; a Notification Service Extension on the device
// can enrich it.
//
// Env:
//   APNS_KEY_ID     10-char key id from the Apple portal
//   APNS_TEAM_ID    10-char team id
//   APNS_BUNDLE_ID  app bundle id (becomes apns-topic)
//   APNS_KEY_P8     the .p8 file contents (PEM), OR
//   APNS_KEY_PATH   path to the .p8 file
//   APNS_ENV        "production" | "sandbox" (default sandbox)

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type apnsResult struct {
	status int
	reason string
	err    error
}

// gone means the token is genuinely dead (app uninstalled / token
// permanently invalidated) and the subscription should be pruned. Apple
// signals this with 410 Unregistered.
func (a apnsResult) gone() bool {
	return a.status == http.StatusGone || a.reason == "Unregistered"
}

// envMismatch is Apple's 400 BadDeviceToken: the token is well-formed but
// does not belong to this APNs environment - almost always APNS_ENV
// (sandbox vs production) not matching the build the token came from (a
// debug/dev build mints sandbox tokens, TestFlight/App Store mint
// production ones). NOT pruned: the subscription is fine, the server is
// aiming at the wrong host.
func (a apnsResult) envMismatch() bool {
	return a.status == http.StatusBadRequest && a.reason == "BadDeviceToken"
}

func (a apnsResult) ok() bool { return a.status == http.StatusOK }

func apnsConfigured() bool {
	return strings.TrimSpace(envOr("APNS_KEY_ID", "")) != "" &&
		strings.TrimSpace(envOr("APNS_TEAM_ID", "")) != "" &&
		strings.TrimSpace(envOr("APNS_BUNDLE_ID", "")) != "" &&
		(strings.TrimSpace(envOr("APNS_KEY_P8", "")) != "" || strings.TrimSpace(envOr("APNS_KEY_PATH", "")) != "")
}

func apnsHost() string {
	if strings.EqualFold(strings.TrimSpace(envOr("APNS_ENV", "sandbox")), "production") {
		return "https://api.push.apple.com"
	}
	return "https://api.sandbox.push.apple.com"
}

// apnsEnvLabel is "production" or "sandbox" - for logs. A token minted by
// a debug/TestFlight build hitting the wrong host is the classic
// "BadDeviceToken" cause, so surfacing which host we used matters.
func apnsEnvLabel() string {
	if strings.EqualFold(strings.TrimSpace(envOr("APNS_ENV", "sandbox")), "production") {
		return "production"
	}
	return "sandbox"
}

func apnsPrivateKey() (*ecdsa.PrivateKey, error) {
	raw := strings.TrimSpace(os.Getenv("APNS_KEY_P8"))
	if raw == "" {
		path := strings.TrimSpace(os.Getenv("APNS_KEY_PATH"))
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("APNS_KEY_PATH: %w", err)
		}
		raw = string(b)
	}
	// A .p8 pasted into an env file (docker-compose env_file, systemd, ...)
	// can't hold real newlines, so accept a literal "\n" the same way the
	// Sign-in-with-Apple key does (see zmist .env.example APPLE_PRIVATE_KEY).
	raw = strings.ReplaceAll(raw, `\n`, "\n")
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		return nil, fmt.Errorf("APNS key: not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ec, ok := key.(*ecdsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("APNS key: not an EC key")
	}
	return ec, nil
}

var (
	apnsTokMu  sync.Mutex
	apnsTok    string
	apnsTokExp time.Time
)

func apnsProviderToken() (string, error) {
	apnsTokMu.Lock()
	defer apnsTokMu.Unlock()
	if apnsTok != "" && time.Now().Before(apnsTokExp) {
		return apnsTok, nil
	}
	priv, err := apnsPrivateKey()
	if err != nil {
		return "", err
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iss": strings.TrimSpace(os.Getenv("APNS_TEAM_ID")),
		"iat": time.Now().Unix(),
	})
	tok.Header["kid"] = strings.TrimSpace(os.Getenv("APNS_KEY_ID"))
	signed, err := tok.SignedString(priv)
	if err != nil {
		return "", err
	}
	apnsTok = signed
	apnsTokExp = time.Now().Add(40 * time.Minute)
	return signed, nil
}

// sendAPNs POSTs one tickle to a device token.
func sendAPNs(deviceToken string, aps map[string]interface{}, custom map[string]interface{}) apnsResult {
	if !apnsConfigured() {
		return apnsResult{err: fmt.Errorf("APNS not configured")}
	}
	token, err := apnsProviderToken()
	if err != nil {
		return apnsResult{err: err}
	}

	payload := map[string]interface{}{"aps": aps}
	for k, v := range custom {
		payload[k] = v
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, apnsHost()+"/3/device/"+strings.TrimSpace(deviceToken), bytes.NewReader(body))
	if err != nil {
		return apnsResult{err: err}
	}
	topic := strings.TrimSpace(os.Getenv("APNS_BUNDLE_ID"))
	req.Header.Set("authorization", "bearer "+token)
	req.Header.Set("apns-topic", topic)
	req.Header.Set("apns-push-type", "alert")
	req.Header.Set("apns-priority", "10")
	req.Header.Set("content-type", "application/json")

	if pushDebug() {
		log.Printf("[push:send] apns POST %s/3/device/…%s topic=%s env=%s bodyBytes=%d",
			apnsHost(), tail(deviceToken, 6), topic, apnsEnvLabel(), len(body))
	}

	resp, err := apnsClient().Do(req)
	if err != nil {
		return apnsResult{err: err}
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusOK {
		return apnsResult{status: resp.StatusCode}
	}
	var parsed struct {
		Reason string `json:"reason"`
	}
	_ = json.Unmarshal(raw, &parsed)
	return apnsResult{status: resp.StatusCode, reason: parsed.Reason}
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
