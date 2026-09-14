package transitions

// push/subs - per-user push subscription storage. One Redis Hash per user
// at "push:subs:<userId>": field is a stable id derived from the endpoint
// (web) or device token (iOS), value is the subscription JSON. A user has
// as many entries as they have browsers + devices; SendToUsers fans a
// notification to all of them and prunes the ones a push service reports
// gone (404/410).

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

const subsKeyPrefix = "push:subs:"

// Subscription is one browser or device a user can be pushed to.
type Subscription struct {
	Platform  string `json:"platform"` // "web" | "ios"
	Endpoint  string `json:"endpoint,omitempty"`
	P256dh    string `json:"p256dh,omitempty"`
	Auth      string `json:"auth,omitempty"`
	Token     string `json:"token,omitempty"`
	UA        string `json:"ua,omitempty"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type subsTransitions struct {
	workflow.BaseServiceTransition
}

// NewSubsTransitions is the DI constructor for the `push/subs` class.
func NewSubsTransitions() interfaces.ServiceTransitions {
	return &subsTransitions{}
}

func subsKey(userID string) string { return subsKeyPrefix + strings.TrimSpace(userID) }

// subField is the stable Hash field for a subscription: "web:<sha1(endpoint)>"
// or "ios:<token>". Deterministic so a re-subscribe from the same browser
// overwrites rather than duplicates.
func subField(platform, endpoint, token string) string {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "ios", "apns":
		return "ios:" + strings.TrimSpace(token)
	default:
		sum := sha1.Sum([]byte(strings.TrimSpace(endpoint)))
		return "web:" + hex.EncodeToString(sum[:])
	}
}

// Store upserts one subscription for a user. platform "web" needs
// endpoint+p256dh+auth; platform "ios" needs token.
func (t *subsTransitions) Store(userId string, platform string, endpoint string, p256dh string, auth string, token string, ua string) (r domain.FlowStepResult) {
	userId = strings.TrimSpace(userId)
	platform = strings.ToLower(strings.TrimSpace(platform))
	if userId == "" {
		return failResult(http.StatusBadRequest, fmt.Errorf("userId is required"))
	}
	if platform == "ios" || platform == "apns" {
		platform = "ios"
		if strings.TrimSpace(token) == "" {
			return failResult(http.StatusBadRequest, fmt.Errorf("token is required for ios"))
		}
	} else {
		platform = "web"
		if strings.TrimSpace(endpoint) == "" || strings.TrimSpace(p256dh) == "" || strings.TrimSpace(auth) == "" {
			return failResult(http.StatusBadRequest, fmt.Errorf("endpoint, p256dh and auth are required for web"))
		}
	}

	sub := Subscription{
		Platform:  platform,
		Endpoint:  strings.TrimSpace(endpoint),
		P256dh:    strings.TrimSpace(p256dh),
		Auth:      strings.TrimSpace(auth),
		Token:     strings.TrimSpace(token),
		UA:        strings.TrimSpace(ua),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}
	blob, _ := json.Marshal(sub)
	field := subField(platform, sub.Endpoint, sub.Token)

	client := rdb()
	defer client.Close()
	if err := client.HSet(bg(), subsKey(userId), field, string(blob)).Err(); err != nil {
		return failResult(http.StatusInternalServerError, err)
	}
	reclaimForeignField(client, userId, field)

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"stored": true, "field": field, "platform": platform}
	return
}

// reclaimForeignField removes `field` from every OTHER user's push:subs
// hash. A Web Push endpoint / APNs device token identifies exactly one
// browser profile or device; when that device re-registers under a new
// account (a shared computer, an account switch, a reinstall) the stale
// row left under the old account would otherwise keep delivering that
// account's notifications to the device now signed in as someone else.
//
// SCAN-based on purpose: POST /push/subscribe runs a handful of times per
// account lifetime and the keyspace is one small hash per user, so a
// MATCH push:subs:* sweep is a few milliseconds and needs no reverse
// index to keep consistent across Remove / pruneSub / new accounts.
func reclaimForeignField(client *redis.Client, keepUser, field string) {
	self := subsKey(keepUser)
	var cursor uint64
	for {
		keys, next, err := client.Scan(bg(), cursor, subsKeyPrefix+"*", 200).Result()
		if err != nil {
			log.Printf("[push:subs] reclaim scan aborted for field %q: %v", field, err)
			return
		}
		for _, k := range keys {
			if k == self {
				continue
			}
			if n, _ := client.HDel(bg(), k, field).Result(); n > 0 {
				log.Printf("[push:subs] reclaimed %q from %s (now owned by %s)", field, k, self)
			}
		}
		if next == 0 {
			return
		}
		cursor = next
	}
}

// Remove deletes one subscription. Pass either the exact stored field, or
// the raw endpoint / token (whichever the client still has).
func (t *subsTransitions) Remove(userId string, key string) (r domain.FlowStepResult) {
	userId = strings.TrimSpace(userId)
	key = strings.TrimSpace(key)
	if userId == "" || key == "" {
		return failResult(http.StatusBadRequest, fmt.Errorf("userId and key are required"))
	}

	client := rdb()
	defer client.Close()

	candidates := []string{key}
	if !strings.HasPrefix(key, "web:") && !strings.HasPrefix(key, "ios:") {
		candidates = append(candidates,
			subField("web", key, ""),
			subField("ios", "", key),
		)
	}
	removed, _ := client.HDel(bg(), subsKey(userId), candidates...).Result()

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"removed": removed}
	return
}

// ListForUsers returns every subscription for the given user ids, tagged
// with its owner and stored field (so a failed delivery can be pruned).
// userIds arrives from WSL as []interface{} (see redis.go's toStringSlice).
func (t *subsTransitions) ListForUsers(userIds interface{}) (r domain.FlowStepResult) {
	ids := toStringSlice(userIds)
	client := rdb()
	defer client.Close()

	out := make([]map[string]interface{}, 0, len(ids))
	for _, uid := range ids {
		entries, err := client.HGetAll(bg(), subsKey(uid)).Result()
		if err != nil {
			continue
		}
		for field, blob := range entries {
			var sub Subscription
			if json.Unmarshal([]byte(blob), &sub) != nil {
				continue
			}
			out = append(out, map[string]interface{}{
				"userId":   uid,
				"field":    field,
				"platform": sub.Platform,
				"endpoint": sub.Endpoint,
				"p256dh":   sub.P256dh,
				"auth":     sub.Auth,
				"token":    sub.Token,
			})
		}
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"subscriptions": out, "count": len(out)}
	return
}

// ListSafe returns one user's subscriptions with the delivery secrets
// (p256dh/auth, full APNs token) stripped - safe to hand back over a REST
// endpoint so a settings screen can show "2 devices registered" and let
// the user recognise each one. GET /push/subscriptions.
func (t *subsTransitions) ListSafe(userId string) (r domain.FlowStepResult) {
	userId = strings.TrimSpace(userId)
	client := rdb()
	defer client.Close()

	entries, _ := client.HGetAll(bg(), subsKey(userId)).Result()
	out := make([]map[string]interface{}, 0, len(entries))
	web, ios := 0, 0
	for field, blob := range entries {
		var sub Subscription
		if json.Unmarshal([]byte(blob), &sub) != nil {
			continue
		}
		var label string
		switch sub.Platform {
		case "ios":
			ios++
			label = "iOS device"
			if n := len(sub.Token); n >= 6 {
				label = "iOS device ·" + sub.Token[n-6:]
			}
		default:
			web++
			label = "Browser"
			if sub.UA != "" {
				label = "Browser · " + shortUA(sub.UA)
			} else if host := endpointHost(sub.Endpoint); host != "" {
				label = "Browser (" + host + ")"
			}
		}
		// A short non-reversible id: enough to tell two rows apart in the
		// UI without exposing the raw endpoint hash or APNs token that
		// makes up the stored field.
		sum := sha1.Sum([]byte(field))
		out = append(out, map[string]interface{}{
			"id":        hex.EncodeToString(sum[:])[:12],
			"platform":  sub.Platform,
			"label":     label,
			"createdAt": sub.CreatedAt,
		})
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"subscriptions": out,
		"count":         len(out),
		"web":           web,
		"ios":           ios,
	}
	return
}

// endpointHost pulls the push-service host out of a Web Push endpoint URL
// ("https://web.push.apple.com/..." -> "web.push.apple.com").
func endpointHost(endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if i := strings.Index(endpoint, "://"); i >= 0 {
		endpoint = endpoint[i+3:]
	}
	if i := strings.IndexAny(endpoint, "/?"); i >= 0 {
		endpoint = endpoint[:i]
	}
	return endpoint
}

// shortUA trims a User-Agent to something recognisable in a list row.
func shortUA(ua string) string {
	ua = strings.TrimSpace(ua)
	if len(ua) > 60 {
		return ua[:60] + "…"
	}
	return ua
}

// loadSubsForUsers is the internal form ListForUsers wraps - used by
// send.go without a FlowStepResult round-trip.
func loadSubsForUsers(ids []string) []userSub {
	client := rdb()
	defer client.Close()

	out := make([]userSub, 0, len(ids))
	for _, uid := range ids {
		entries, err := client.HGetAll(bg(), subsKey(uid)).Result()
		if err != nil {
			continue
		}
		for field, blob := range entries {
			var sub Subscription
			if json.Unmarshal([]byte(blob), &sub) != nil {
				continue
			}
			out = append(out, userSub{userID: uid, field: field, sub: sub})
		}
	}
	return out
}

type userSub struct {
	userID string
	field  string
	sub    Subscription
}

func pruneSub(userID, field string) {
	client := rdb()
	defer client.Close()
	client.HDel(bg(), subsKey(userID), field)
}

func failResult(status int, err error) (r domain.FlowStepResult) {
	r.Error = err
	r.StatusCode = status
	return
}

func errRequired(field string) error { return fmt.Errorf("%s is required", field) }
