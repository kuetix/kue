package transitions

// push/send - fan a tickle to every subscription of every target user,
// across Web Push and APNs, pruning subscriptions a push service reports
// gone.
//
// The base payload is {c: channelId, t: type, n: count}. A caller may pass
// an optional `content` map (SendToUsers' 5th arg) with human-readable
// bits the host app resolved - title, sender, signal name/emoji, a deep-
// link message id, and the raw text/value. Every non-empty content key
// rides into the web payload + APNs custom data so the client can render
// without a follow-up API call; `title` becomes aps.alert.title, `sender`
// aps.alert.subtitle, and a body is composed from the signal label (safe)
// or the previewBody (raw text - only for recipients whose
// push:prefs.preview is on). nil/empty content = the old content-light
// payload, byte-for-byte.

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/redis/go-redis/v9"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/spf13/cast"
)

type sendTransitions struct {
	workflow.BaseServiceTransition
}

// NewSendTransitions is the DI constructor for the `push/send` class.
func NewSendTransitions() interfaces.ServiceTransitions {
	return &sendTransitions{}
}

func genericBody(sendType string, count int) string {
	// "test" is SendTest's own literal sendType (see below), never a
	// message/report/signal name - checked ahead of prefBucket so an
	// admin-triggered test push reads as one instead of falling into
	// prefBucket's signals-bucket default ("New signal").
	if strings.EqualFold(strings.TrimSpace(sendType), "test") {
		return "Test notification"
	}
	if count > 1 {
		switch prefBucket(sendType) {
		case "reports":
			return fmt.Sprintf("%d new reports", count)
		default:
			return fmt.Sprintf("%d new signals", count)
		}
	}
	switch prefBucket(sendType) {
	case "reports":
		return "New report"
	default:
		return "New signal"
	}
}

// subTarget is a log-safe one-line description of where a subscription
// points - the push-service host for Web Push, the last 6 chars of the
// APNs token for iOS. Never the full token or endpoint (both are
// credentials).
func subTarget(sub Subscription) string {
	if sub.Platform == "ios" {
		if n := len(sub.Token); n >= 6 {
			return "apns:…" + sub.Token[n-6:]
		}
		return "apns:(short-token)"
	}
	if h := endpointHost(sub.Endpoint); h != "" {
		return "web:" + h
	}
	return "web:(no-endpoint)"
}

// redactedContentKeys are dropped from the payload for recipients whose
// push:prefs.preview is off - the raw message text / signal value / the
// rich body built from them. Everything else (channel title, sender,
// signal name+emoji, deep-link id, avatar) is shown regardless.
var redactedContentKeys = map[string]bool{"text": true, "value": true, "previewBody": true}

// contentString safely reads a trimmed string content key.
func contentString(content map[string]interface{}, key string) string {
	s, _ := content[key].(string)
	return strings.TrimSpace(s)
}

// signalLabel is the safe one-liner shown even without preview: "<emoji> <name>".
func signalLabel(content map[string]interface{}) string {
	name := contentString(content, "sig")
	if name == "" {
		return ""
	}
	if e := contentString(content, "emoji"); e != "" {
		return e + " " + name
	}
	return name
}

// buildVariant renders one payload. includeText controls whether the raw
// message text / value / rich body are present; false = the redacted form.
// custom is the same key set as the web payload, for APNs top-level merge.
func buildVariant(channelID, sendType string, count int, content map[string]interface{}, includeText bool) (webPayload []byte, aps, custom map[string]interface{}) {
	custom = map[string]interface{}{"c": channelID, "t": sendType, "n": count}
	for k, v := range content {
		if s, ok := v.(string); ok && strings.TrimSpace(s) == "" {
			continue
		}
		if !includeText && redactedContentKeys[k] {
			continue
		}
		custom[k] = v
	}

	body := signalLabel(content)
	if body == "" {
		body = genericBody(sendType, count)
	}
	if includeText {
		if pb := contentString(content, "previewBody"); pb != "" {
			body = pb
		}
	}

	alert := map[string]interface{}{"body": body}
	if tt := contentString(content, "title"); tt != "" {
		alert["title"] = tt
	}
	if sn := contentString(content, "sender"); sn != "" {
		alert["subtitle"] = sn
	}
	aps = map[string]interface{}{
		"alert":             alert,
		"sound":             "default",
		"mutable-content":   1,
		"content-available": 1,
		"thread-id":         channelID,
	}
	webPayload, _ = json.Marshal(custom)
	return
}

// loadPreviewFlags batch-reads push:prefs:<uid>.preview for a set of users.
func loadPreviewFlags(userIDs []string) map[string]bool {
	out := make(map[string]bool, len(userIDs))
	if len(userIDs) == 0 {
		return out
	}
	client := rdb()
	defer client.Close()
	pipe := client.Pipeline()
	cmds := make(map[string]*redis.StringCmd, len(userIDs))
	for _, id := range userIDs {
		cmds[id] = pipe.HGet(bg(), prefsKey(id), "preview")
	}
	_, _ = pipe.Exec(bg())
	for id, cmd := range cmds {
		if v, _ := cmd.Result(); v == "true" || v == "1" {
			out[id] = true
		}
	}
	return out
}

// sendTickle is the shared core: returns how many notifications were
// actually accepted by a push service. Logs the whole attempt under
// [push:send] so a "no notification arrived" report can be traced to the
// exact step: no subscriptions, a push-service rejection (status/reason),
// or a config gap (vapid/apns=false).
func sendTickle(userIDs []string, channelID, sendType string, count int, content map[string]interface{}) int {
	userIDs = dedupeNonEmpty(userIDs)
	if len(userIDs) == 0 {
		return 0
	}
	if count < 1 {
		count = 1
	}
	if content == nil {
		content = map[string]interface{}{}
	}

	fullWeb, fullAPS, fullCustom := buildVariant(channelID, sendType, count, content, true)
	redWeb, redAPS, redCustom := buildVariant(channelID, sendType, count, content, false)

	// preview flags only matter when there's raw text to gate.
	previewOn := map[string]bool{}
	if contentString(content, "text") != "" || contentString(content, "value") != "" || contentString(content, "previewBody") != "" {
		previewOn = loadPreviewFlags(userIDs)
	}

	subs := loadSubsForUsers(userIDs)
	webCfg, apnsCfg := vapidConfigured(), apnsConfigured()
	log.Printf("[push:send] tickle channel=%s type=%s count=%d users=%d subscriptions=%d vapid=%v apns=%v payloadBytes=%d/%d",
		channelID, sendType, count, len(userIDs), len(subs), webCfg, apnsCfg, len(redWeb), len(fullWeb))
	if len(subs) == 0 {
		log.Printf("[push:send] nothing to deliver: no push subscriptions registered for user(s) %v - the target has not enabled notifications on any browser/device (POST /push/subscribe), or the rows were pruned as gone", userIDs)
		return 0
	}

	sent := 0
	for _, us := range subs {
		target := subTarget(us.sub)
		webPayload, aps, custom := redWeb, redAPS, redCustom
		if previewOn[us.userID] {
			webPayload, aps, custom = fullWeb, fullAPS, fullCustom
		}
		switch us.sub.Platform {
		case "ios":
			if !apnsCfg {
				log.Printf("[push:send] user=%s %s SKIPPED: APNs not configured (APNS_KEY_ID/APNS_TEAM_ID/APNS_BUNDLE_ID/APNS_KEY_P8|APNS_KEY_PATH)", us.userID, target)
				continue
			}
			res := sendAPNs(us.sub.Token, aps, custom)
			switch {
			case res.ok():
				sent++
				log.Printf("[push:send] user=%s %s apns OK (status=%d, env=%s)", us.userID, target, res.status, apnsEnvLabel())
			case res.envMismatch():
				log.Printf("[push:send] user=%s %s apns ENV MISMATCH (status=%d reason=%q): APNS_ENV=%s but this token was minted for the other environment - a debug/dev build needs APNS_ENV=sandbox, TestFlight/App Store needs production. Subscription kept.", us.userID, target, res.status, res.reason, apnsEnvLabel())
			case res.gone():
				pruneSub(us.userID, us.field)
				log.Printf("[push:send] user=%s %s apns GONE (status=%d reason=%q) - subscription pruned", us.userID, target, res.status, res.reason)
			case res.err != nil:
				log.Printf("[push:send] user=%s %s apns ERROR: %v", us.userID, target, res.err)
			default:
				log.Printf("[push:send] user=%s %s apns REJECTED (status=%d reason=%q)", us.userID, target, res.status, res.reason)
			}
		default:
			if !webCfg {
				log.Printf("[push:send] user=%s %s SKIPPED: VAPID not configured (VAPID_PUBLIC_KEY/VAPID_PRIVATE_KEY)", us.userID, target)
				continue
			}
			res := sendWebPush(us.sub, webPayload, 3600)
			switch {
			case res.ok():
				sent++
				log.Printf("[push:send] user=%s %s webpush OK (status=%d)", us.userID, target, res.status)
			case res.gone():
				pruneSub(us.userID, us.field)
				log.Printf("[push:send] user=%s %s webpush GONE (status=%d body=%q) - subscription pruned", us.userID, target, res.status, res.body)
			case res.err != nil:
				log.Printf("[push:send] user=%s %s webpush ERROR: %v", us.userID, target, res.err)
			default:
				log.Printf("[push:send] user=%s %s webpush REJECTED (status=%d body=%q)", us.userID, target, res.status, res.body)
			}
		}
	}
	log.Printf("[push:send] tickle done channel=%s: delivered %d/%d", channelID, sent, len(subs))
	return sent
}

// SendToUsers delivers a tickle now. userIds arrives from WSL as
// []interface{}; count as a string/number. content is an optional map of
// human-readable bits (title/sender/sig/emoji/color/mid/text/value/
// previewBody/avatar/accent/symbol) - "" / absent / an empty object all
// mean "content-light", the pre-enrichment payload.
func (t *sendTransitions) SendToUsers(userIds interface{}, channelId string, sendType string, count interface{}, content interface{}) (r domain.FlowStepResult) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return failResult(http.StatusBadRequest, errRequired("channelId"))
	}
	ids := toStringSlice(userIds)
	sent := sendTickle(ids, channelId, strings.TrimSpace(sendType), cast.ToInt(count), toContentMap(content))

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"recipients": len(ids), "sent": sent}
	return
}

// toContentMap accepts the shapes WSL delivers an object arg as: a real
// map, a nil, or a "" (the ??"" default at a call site). Anything that
// isn't a populated map means "no enrichment".
func toContentMap(v interface{}) map[string]interface{} {
	m, _ := v.(map[string]interface{})
	return m
}

// SendTest delivers an admin-triggered test tickle to every one of userIds'
// registered devices, bypassing prefs/quiet-hours the same way SendToUsers
// does. Unlike SendToUsers, there's deliberately no channelId: a test push
// isn't scoped to any real channel, so requiring one here would just push
// the caller into inventing a fake id. sendTickle gets literal "test" as
// both channelId and sendType - genericBody's "test" case turns that into a
// "Test notification" body instead of prefBucket's signals-bucket default
// ("New signal"), and an empty channelId would otherwise reach the client
// as a "c" the UI can't resolve a title for.
func (t *sendTransitions) SendTest(userIds interface{}, count interface{}) (r domain.FlowStepResult) {
	ids := toStringSlice(userIds)
	sent := sendTickle(ids, "test", "test", cast.ToInt(count), nil)

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"recipients": len(ids), "sent": sent}
	return
}
