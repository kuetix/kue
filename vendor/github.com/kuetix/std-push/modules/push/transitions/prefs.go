package transitions

// push/prefs - per-user notification preferences. Redis Hash
// "push:prefs:<userId>" with string-bool fields: enabled (master),
// messages, signals, reports. Anything unset defaults to true, so a user
// who subscribed but never opened the settings screen still gets pushed.
//
// `email` is the one exception: it defaults to FALSE (opt-in). When set it
// asks the host app to ALSO deliver the same missed-activity notification
// by email, in addition to any device push. It still respects the master
// `enabled` switch and the per-type gate (messages/signals/reports), so it
// is purely an extra delivery channel layered on the existing push
// audience - see wantsEmail / EmailAudience below.

import (
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

const prefsKeyPrefix = "push:prefs:"

type prefsTransitions struct {
	workflow.BaseServiceTransition
}

// NewPrefsTransitions is the DI constructor for the `push/prefs` class.
func NewPrefsTransitions() interfaces.ServiceTransitions {
	return &prefsTransitions{}
}

func prefsKey(userID string) string { return prefsKeyPrefix + strings.TrimSpace(userID) }

// prefBucket maps a message type to the preference field that governs it:
// "report" -> reports, "message" -> messages, everything else (template
// signal names) -> signals.
func prefBucket(sendType string) string {
	switch strings.ToLower(strings.TrimSpace(sendType)) {
	case "report":
		return "reports"
	case "message", "":
		return "messages"
	default:
		return "signals"
	}
}

type prefsView struct {
	Enabled  bool `json:"enabled"`
	Messages bool `json:"messages"`
	Signals  bool `json:"signals"`
	Reports  bool `json:"reports"`
	// Opt-in (defaults false). "Also email me these notifications."
	Email bool `json:"email"`
	// Opt-in (defaults false). "Show the message text / signal value in the
	// notification itself." Purely a CONTENT gate - a preview:false user
	// still receives every notification they're entitled to, just without
	// the raw text; it never enters allows().
	Preview bool `json:"preview"`
}

func loadPrefs(userID string) prefsView {
	p := prefsView{Enabled: true, Messages: true, Signals: true, Reports: true}
	client := rdb()
	defer client.Close()
	entries, err := client.HGetAll(bg(), prefsKey(userID)).Result()
	if err != nil || len(entries) == 0 {
		return p
	}
	set := func(dst *bool, key string) {
		if v, ok := entries[key]; ok {
			*dst = v == "true" || v == "1"
		}
	}
	set(&p.Enabled, "enabled")
	set(&p.Messages, "messages")
	set(&p.Signals, "signals")
	set(&p.Reports, "reports")
	set(&p.Email, "email")     // absent -> stays false (opt-in)
	set(&p.Preview, "preview") // absent -> stays false (opt-in)
	return p
}

func (p prefsView) allows(sendType string) bool {
	if !p.Enabled {
		return false
	}
	switch prefBucket(sendType) {
	case "reports":
		return p.Reports
	case "signals":
		return p.Signals
	default:
		return p.Messages
	}
}

// Get returns the caller's effective preferences (defaults applied).
func (t *prefsTransitions) Get(userId string) (r domain.FlowStepResult) {
	p := loadPrefs(userId)
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"enabled":  p.Enabled,
		"messages": p.Messages,
		"signals":  p.Signals,
		"reports":  p.Reports,
		"email":    p.Email,
		"preview":  p.Preview,
	}
	return
}

// Set writes whichever of enabled/messages/signals/reports/email/preview
// were supplied (each is "true"/"false"/"" - "" means "leave as is").
// Returns the resulting effective view.
func (t *prefsTransitions) Set(userId string, enabled string, messages string, signals string, reports string, email string, preview string) (r domain.FlowStepResult) {
	userId = strings.TrimSpace(userId)
	if userId == "" {
		return failResult(http.StatusBadRequest, errRequired("userId"))
	}

	fields := map[string]interface{}{}
	add := func(name, val string) {
		switch strings.ToLower(strings.TrimSpace(val)) {
		case "true", "1", "yes", "on":
			fields[name] = "true"
		case "false", "0", "no", "off":
			fields[name] = "false"
		}
	}
	add("enabled", enabled)
	add("messages", messages)
	add("signals", signals)
	add("reports", reports)
	add("email", email)
	add("preview", preview)

	if len(fields) > 0 {
		client := rdb()
		defer client.Close()
		if err := client.HSet(bg(), prefsKey(userId), fields).Err(); err != nil {
			return failResult(http.StatusInternalServerError, err)
		}
	}

	p := loadPrefs(userId)
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"enabled":  p.Enabled,
		"messages": p.Messages,
		"signals":  p.Signals,
		"reports":  p.Reports,
		"email":    p.Email,
		"preview":  p.Preview,
		"updated":  len(fields),
	}
	return
}

// AllowsType reports whether userId currently wants a push for a message
// of sendType. Convenience for callers that aren't going through
// audience.ResolveMissed.
func (t *prefsTransitions) AllowsType(userId string, sendType string) (r domain.FlowStepResult) {
	allowed := loadPrefs(userId).allows(sendType)
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"allowed": allowed, "bucket": prefBucket(sendType)}
	return
}
