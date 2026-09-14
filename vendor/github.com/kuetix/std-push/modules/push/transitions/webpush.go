package transitions

// Web Push delivery: VAPID (RFC 8292) auth + aes128gcm (RFC 8291 / RFC
// 8188) payload encryption, hand-rolled on the standard library +
// x/crypto/hkdf so the package pulls no extra dependency. The payload is a
// ~30-byte content-light tickle, so aes128gcm (mandated for this hop) is
// carrying nothing sensitive regardless.
//
// Env:
//   VAPID_PUBLIC_KEY   base64url, 65-byte uncompressed P-256 point
//   VAPID_PRIVATE_KEY  base64url, 32-byte P-256 private scalar
//   VAPID_SUBJECT      "mailto:ops@example.com" (or an https: URL)

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/hkdf"
)

var b64 = base64.RawURLEncoding

type webPushResult struct {
	status int
	body   string // push-service response body, captured on a non-2xx
	err    error
}

func (w webPushResult) gone() bool {
	return w.status == http.StatusNotFound || w.status == http.StatusGone
}
func (w webPushResult) ok() bool { return w.status >= 200 && w.status < 300 }

func vapidConfigured() bool {
	return strings.TrimSpace(envOr("VAPID_PUBLIC_KEY", "")) != "" &&
		strings.TrimSpace(envOr("VAPID_PRIVATE_KEY", "")) != ""
}

// vapidPrivateKey rebuilds the P-256 ecdsa private key from the raw scalar.
func vapidPrivateKey() (*ecdsa.PrivateKey, error) {
	raw, err := b64.DecodeString(strings.TrimSpace(os.Getenv("VAPID_PRIVATE_KEY")))
	if err != nil {
		return nil, fmt.Errorf("VAPID_PRIVATE_KEY: %w", err)
	}
	d := new(big.Int).SetBytes(raw)
	curve := elliptic.P256()
	x, y := curve.ScalarBaseMult(raw)
	return &ecdsa.PrivateKey{
		PublicKey: ecdsa.PublicKey{Curve: curve, X: x, Y: y},
		D:         d,
	}, nil
}

// vapidAuthHeader builds "vapid t=<jwt>, k=<pubkey>" for one endpoint.
func vapidAuthHeader(endpoint string) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	priv, err := vapidPrivateKey()
	if err != nil {
		return "", err
	}
	sub := strings.TrimSpace(envOr("VAPID_SUBJECT", "mailto:admin@localhost"))
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"aud": fmt.Sprintf("%s://%s", u.Scheme, u.Host),
		"exp": time.Now().Add(12 * time.Hour).Unix(),
		"sub": sub,
	})
	signed, err := token.SignedString(priv)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("vapid t=%s, k=%s", signed, strings.TrimSpace(os.Getenv("VAPID_PUBLIC_KEY"))), nil
}

// encryptAES128GCM produces an RFC 8188 aes128gcm body for one subscription.
func encryptAES128GCM(plaintext []byte, uaPublicB64, authSecretB64 string) ([]byte, error) {
	uaPubBytes, err := b64.DecodeString(strings.TrimSpace(uaPublicB64))
	if err != nil {
		return nil, fmt.Errorf("p256dh: %w", err)
	}
	authSecret, err := b64.DecodeString(strings.TrimSpace(authSecretB64))
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}

	curve := ecdh.P256()
	uaPub, err := curve.NewPublicKey(uaPubBytes)
	if err != nil {
		return nil, fmt.Errorf("ua public key: %w", err)
	}
	asPriv, err := curve.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	asPub := asPriv.PublicKey().Bytes() // 65-byte uncompressed

	shared, err := asPriv.ECDH(uaPub)
	if err != nil {
		return nil, err
	}

	// PRK_key = HKDF(auth_secret, shared, "WebPush: info\x00<ua><as>", 32)
	authInfo := bytes.NewBuffer([]byte("WebPush: info\x00"))
	authInfo.Write(uaPubBytes)
	authInfo.Write(asPub)
	prk := hkdfExpand(authSecret, shared, authInfo.Bytes(), 32)

	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	cek := hkdfExpand(salt, prk, []byte("Content-Encoding: aes128gcm\x00"), 16)
	nonce := hkdfExpand(salt, prk, []byte("Content-Encoding: nonce\x00"), 12)

	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	// Single record: plaintext followed by the 0x02 "last record" delimiter.
	record := append(append([]byte{}, plaintext...), 0x02)
	ciphertext := gcm.Seal(nil, nonce, record, nil)

	// Header: salt(16) | rs(4, BE) | idlen(1) | keyid(as_public, 65)
	var body bytes.Buffer
	body.Write(salt)
	rs := make([]byte, 4)
	binary.BigEndian.PutUint32(rs, 4096)
	body.Write(rs)
	body.WriteByte(byte(len(asPub)))
	body.Write(asPub)
	body.Write(ciphertext)
	return body.Bytes(), nil
}

func hkdfExpand(salt, ikm, info []byte, length int) []byte {
	r := hkdf.New(sha256.New, ikm, salt, info)
	out := make([]byte, length)
	io.ReadFull(r, out)
	return out
}

// sendWebPush encrypts + POSTs one tickle to a web subscription.
func sendWebPush(sub Subscription, payload []byte, ttlSeconds int) webPushResult {
	if !vapidConfigured() {
		return webPushResult{err: fmt.Errorf("VAPID not configured")}
	}
	body, err := encryptAES128GCM(payload, sub.P256dh, sub.Auth)
	if err != nil {
		return webPushResult{err: err}
	}
	auth, err := vapidAuthHeader(sub.Endpoint)
	if err != nil {
		return webPushResult{err: err}
	}

	req, err := http.NewRequest(http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return webPushResult{err: err}
	}
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", fmt.Sprintf("%d", ttlSeconds))
	req.Header.Set("Urgency", "normal")
	req.Header.Set("Authorization", auth)

	if pushDebug() {
		if u, e := url.Parse(sub.Endpoint); e == nil {
			log.Printf("[push:send] webpush POST %s://%s%s ttl=%d bodyBytes=%d", u.Scheme, u.Host, u.Path, ttlSeconds, len(body))
		}
	}

	resp, err := httpClient().Do(req)
	if err != nil {
		return webPushResult{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		io.Copy(io.Discard, resp.Body)
		return webPushResult{status: resp.StatusCode}
	}
	// A rejection body ("push subscription has unsubscribed or expired",
	// "invalid JWT", "the request must contain a TTL header", ...) is the
	// most useful thing to log - cap it so a hostile endpoint can't flood.
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return webPushResult{status: resp.StatusCode, body: strings.TrimSpace(string(raw))}
}
