package transitions

// push/audience - who should actually get a push for one channel event.
//
// Starts from the channel's member set and removes, in order:
//   - the sender (never notify yourself)
//   - everyone the live fan-out already delivered to (the per-recipient
//     "was it broadcast?" check - passed in from broadcast_relay.wsl's
//     FanOutRelay result)
//   - anyone who muted this channel (push:muted:<userId>, a Set of channelIds)
//   - anyone whose notification preferences opt out of this message type
//
// The channel member set key defaults to zmist's "channel:members:<id>"
// convention; override with PUSH_MEMBERS_KEY_PREFIX for another app.

import (
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type audienceTransitions struct {
	workflow.BaseServiceTransition
}

// NewAudienceTransitions is the DI constructor for the `push/audience` class.
func NewAudienceTransitions() interfaces.ServiceTransitions {
	return &audienceTransitions{}
}

func membersKey(channelID string) string {
	return envOr("PUSH_MEMBERS_KEY_PREFIX", "channel:members") + ":" + strings.TrimSpace(channelID)
}

func mutedKey(userID string) string {
	return "push:muted:" + strings.TrimSpace(userID)
}

// allowedForType keeps only the ids that currently want a push for
// sendType (master + per-type preference). Used for the forced-audience
// path (push-test), which skips channel membership and mute checks but
// still honours the user's own notification toggles so the settings
// screen stays testable.
func allowedForType(ids []string, sendType string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range dedupeNonEmpty(ids) {
		if loadPrefs(id).allows(sendType) {
			out = append(out, id)
		}
	}
	return out
}

// resolveMissed is the shared core (used by decide.wsl via ResolveMissed
// and by bulk.go's flush).
func resolveMissed(channelID, senderID string, delivered []string, sendType string) []string {
	client := rdb()
	defer client.Close()

	members, err := client.SMembers(bg(), membersKey(channelID)).Result()
	if err != nil || len(members) == 0 {
		return nil
	}

	skip := make(map[string]struct{}, len(delivered)+1)
	skip[strings.TrimSpace(senderID)] = struct{}{}
	for _, d := range delivered {
		skip[strings.TrimSpace(d)] = struct{}{}
	}

	out := make([]string, 0, len(members))
	for _, m := range members {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, dropped := skip[m]; dropped {
			continue
		}
		if muted, _ := client.SIsMember(bg(), mutedKey(m), channelID).Result(); muted {
			continue
		}
		if !loadPrefs(m).allows(sendType) {
			continue
		}
		out = append(out, m)
	}
	return dedupeNonEmpty(out)
}

// ResolveMissed returns the subset of a channel's members who did NOT get
// this message live and still want a push for it.
//
// forcedUserIds is the push-test escape hatch: when non-empty the channel
// member set and mute list are ignored and the audience is exactly that
// list filtered by each user's notification preferences. cue:missed
// entries from real activity always pass it empty.
func (t *audienceTransitions) ResolveMissed(channelId string, senderId string, deliveredUserIds interface{}, sendType string, forcedUserIds interface{}) (r domain.FlowStepResult) {
	var missed []string
	if forced := toStringSlice(forcedUserIds); len(forced) > 0 {
		missed = allowedForType(forced, sendType)
	} else {
		channelId = strings.TrimSpace(channelId)
		if channelId == "" {
			return failResult(http.StatusBadRequest, errRequired("channelId"))
		}
		missed = resolveMissed(channelId, senderId, toStringSlice(deliveredUserIds), sendType)
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"missedUserIds": toInterfaceSlice(missed),
		"missedCount":   len(missed),
	}
	return
}

func toInterfaceSlice(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}
