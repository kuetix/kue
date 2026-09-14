package transitions

// push/bulk - burst detection + the coalescing buffer.
//
//   push:burst:<channelId>    string, EX PUSH_BULK_WINDOW_SEC (default 180s)
//                             present  => a push already went out recently;
//                                         buffer the next one instead of sending
//   push:pending:<channelId>  List of {messageId,type,missedUserIds,ts} JSON
//                             entries waiting to be sent as one bulk push
//   push:pending:channels     Set index of channels with a non-empty buffer
//
// decide.wsl: first missed message in a quiet channel -> send-now + MarkBurst;
// while the marker lives -> AppendPending. flush.wsl (every PUSH_SCHEDULE_INTERVAL)
// -> FlushDue drains any buffer whose oldest entry is >= PUSH_FLUSH_MAX_AGE_SEC
// old, or whose burst marker has expired, into a single notification. The
// hard 3-minute cap holds because the marker's TTL is the window and the
// decision ruleset's burst-cap-hit rule also forces a send on the next inbound.

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

const (
	burstKeyPrefix        = "push:burst:"
	pendingKeyPrefix      = "push:pending:"
	dropKeyPrefix         = "push:pending:drop:"
	pendingIndexKey       = "push:pending:channels"
	defaultBulkWindowSec  = 180
	defaultFlushMaxAgeSec = 170
)

type bulkTransitions struct {
	workflow.BaseServiceTransition
}

// NewBulkTransitions is the DI constructor for the `push/bulk` class.
func NewBulkTransitions() interfaces.ServiceTransitions {
	return &bulkTransitions{}
}

func burstKey(ch string) string   { return burstKeyPrefix + strings.TrimSpace(ch) }
func pendingKey(ch string) string { return pendingKeyPrefix + strings.TrimSpace(ch) }
func dropKey(ch string) string    { return dropKeyPrefix + strings.TrimSpace(ch) }

func bulkWindow() time.Duration {
	return time.Duration(envInt("PUSH_BULK_WINDOW_SEC", defaultBulkWindowSec)) * time.Second
}
func flushMaxAge() int { return envInt("PUSH_FLUSH_MAX_AGE_SEC", defaultFlushMaxAgeSec) }

type pendingEntry struct {
	MessageID     string   `json:"messageId"`
	Type          string   `json:"type"`
	SenderID      string   `json:"senderId"`
	MissedUserIDs []string `json:"missedUserIds"`
	TS            int64    `json:"ts"`
}

// MarkBurst (re)arms the "a push went out recently" marker for a channel.
func (t *bulkTransitions) MarkBurst(channelId string) (r domain.FlowStepResult) {
	client := rdb()
	defer client.Close()
	if err := client.Set(bg(), burstKey(channelId), "1", bulkWindow()).Err(); err != nil {
		return failResult(http.StatusInternalServerError, err)
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"marked": true}
	return
}

// AppendPending buffers one missed message for a later bulk flush.
// missedUserIds arrives from WSL as []interface{}.
func (t *bulkTransitions) AppendPending(channelId string, messageId string, sendType string, senderId string, missedUserIds interface{}) (r domain.FlowStepResult) {
	channelId = strings.TrimSpace(channelId)
	if channelId == "" {
		return failResult(http.StatusBadRequest, errRequired("channelId"))
	}
	entry := pendingEntry{
		MessageID:     strings.TrimSpace(messageId),
		Type:          strings.TrimSpace(sendType),
		SenderID:      strings.TrimSpace(senderId),
		MissedUserIDs: toStringSlice(missedUserIds),
		TS:            time.Now().Unix(),
	}
	blob, _ := json.Marshal(entry)

	client := rdb()
	defer client.Close()
	pipe := client.Pipeline()
	pipe.RPush(bg(), pendingKey(channelId), string(blob))
	pipe.Expire(bg(), pendingKey(channelId), bulkWindow()+time.Minute)
	pipe.SAdd(bg(), pendingIndexKey, channelId)
	if _, err := pipe.Exec(bg()); err != nil {
		return failResult(http.StatusInternalServerError, err)
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"buffered": true}
	return
}

// DropPendingRecipient removes one user from a channel's not-yet-flushed
// bulk buffer - called when that user (re)joins the channel's Cue room
// while a burst is coalescing (see the Cue join path). A reconnecting
// client refetches the channel itself, so the buffered push would only
// re-notify about messages it has already shown. No-op unless a buffer
// actually exists for the channel, so it's cheap to call on every join.
func (t *bulkTransitions) DropPendingRecipient(channelId string, userId string) (r domain.FlowStepResult) {
	channelId = strings.TrimSpace(channelId)
	userId = strings.TrimSpace(userId)
	r.Success = true
	r.StatusCode = http.StatusOK
	if channelId == "" || userId == "" {
		r.Response = map[string]interface{}{"dropped": false}
		return
	}

	client := rdb()
	defer client.Close()
	if n, _ := client.Exists(bg(), pendingKey(channelId)).Result(); n == 0 {
		r.Response = map[string]interface{}{"dropped": false}
		return
	}
	pipe := client.Pipeline()
	pipe.SAdd(bg(), dropKey(channelId), userId)
	pipe.Expire(bg(), dropKey(channelId), bulkWindow()+time.Minute)
	_, _ = pipe.Exec(bg())

	r.Response = map[string]interface{}{"dropped": true}
	return
}

// PendingStats reports the buffer state decide.wsl feeds into the ruleset.
func (t *bulkTransitions) PendingStats(channelId string) (r domain.FlowStepResult) {
	client := rdb()
	defer client.Close()

	hasBurst, _ := client.Exists(bg(), burstKey(channelId)).Result()
	entries, _ := client.LRange(bg(), pendingKey(channelId), 0, -1).Result()

	oldestAge := 0
	if len(entries) > 0 {
		var first pendingEntry
		if json.Unmarshal([]byte(entries[0]), &first) == nil && first.TS > 0 {
			oldestAge = int(time.Now().Unix() - first.TS)
		}
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"hasBurst":            hasBurst > 0,
		"count":               len(entries),
		"oldestPendingAgeSec": oldestAge,
	}
	return
}

// FlushDue drains every channel buffer that is ready (oldest entry aged
// past PUSH_FLUSH_MAX_AGE_SEC, or the burst marker gone) and sends one
// coalesced tickle per channel. Run on the scheduler tick.
func (t *bulkTransitions) FlushDue() (r domain.FlowStepResult) {
	client := rdb()
	defer client.Close()

	channels, _ := client.SMembers(bg(), pendingIndexKey).Result()
	flushed, pushed := 0, 0
	notReady, emptied, entriesDrained, recipientsNotified := 0, 0, 0, 0
	// Per-channel breakdown of what was actually coalesced and pushed this
	// pass, so a host app can fan the same notification out over another
	// channel (e.g. email) for the recipients who opted in. Push itself is
	// already delivered below; this is purely informational.
	flushedDetail := make([]map[string]interface{}, 0)

	for _, ch := range channels {
		entries, err := client.LRange(bg(), pendingKey(ch), 0, -1).Result()
		if err != nil || len(entries) == 0 {
			client.SRem(bg(), pendingIndexKey, ch)
			continue
		}

		var first pendingEntry
		_ = json.Unmarshal([]byte(entries[0]), &first)
		age := int(time.Now().Unix() - first.TS)
		burstAlive, _ := client.Exists(bg(), burstKey(ch)).Result()

		if age < flushMaxAge() && burstAlive > 0 {
			notReady++
			continue // not ready yet
		}
		entriesDrained += len(entries)

		// Claim the buffer atomically-ish: delete then work from the local copy.
		client.Del(bg(), pendingKey(ch))
		client.SRem(bg(), pendingIndexKey, ch)

		// Anyone who rejoined the channel's Cue room during the wait has
		// already caught up on their own - drop them (see DropPendingRecipient).
		dropped := map[string]struct{}{}
		if members, _ := client.SMembers(bg(), dropKey(ch)).Result(); len(members) > 0 {
			for _, m := range members {
				dropped[m] = struct{}{}
			}
			client.Del(bg(), dropKey(ch))
		}

		recipients := map[string]struct{}{}
		var sendType, newestMessageID string
		var newestTS int64
		senderSet := map[string]struct{}{}
		for _, raw := range entries {
			var e pendingEntry
			if json.Unmarshal([]byte(raw), &e) != nil {
				continue
			}
			if sendType == "" {
				sendType = e.Type
			} else if sendType != e.Type {
				sendType = "bulk"
			}
			if e.MessageID != "" && e.TS >= newestTS {
				newestTS = e.TS
				newestMessageID = e.MessageID
			}
			if s := strings.TrimSpace(e.SenderID); s != "" {
				senderSet[s] = struct{}{}
			}
			for _, u := range e.MissedUserIDs {
				if _, gone := dropped[u]; gone {
					continue
				}
				recipients[u] = struct{}{}
			}
		}
		if len(recipients) == 0 {
			flushed++
			emptied++
			continue
		}

		ids := make([]string, 0, len(recipients))
		for u := range recipients {
			ids = append(ids, u)
		}
		n := len(entries)
		// A coalesced burst has many messages, so no per-message text or
		// signal - just the channel title + a deep link to the newest one.
		title := channelTitle(client, ch)
		bulkContent := map[string]interface{}{}
		if title != "" {
			bulkContent["title"] = title
		}
		if newestMessageID != "" {
			bulkContent["mid"] = newestMessageID
		}
		sent := sendTickle(ids, ch, "bulk", n, bulkContent)
		client.Set(bg(), burstKey(ch), "1", bulkWindow()) // re-arm so a fresh burst still coalesces
		flushed++
		pushed += sent
		recipientsNotified += len(ids)
		senderIDs := make([]string, 0, len(senderSet))
		for s := range senderSet {
			senderIDs = append(senderIDs, s)
		}
		flushedDetail = append(flushedDetail, map[string]interface{}{
			"channelId":       ch,
			"sendType":        sendType,
			"count":           n,
			"recipients":      toInterfaceSlice(ids),
			"title":           title,
			"newestMessageId": newestMessageID,
			"senderIds":       toInterfaceSlice(senderIDs),
		})
	}

	// cue:missed backlog right now - the consumer (cmd/cue-missed) drains
	// this; a growing xlen/lag here means decide.wsl is falling behind.
	missedLen, missedPending, missedLag := streamStats(bg(), client, missedStream, missedGroup)

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"channelsPending":     len(channels),
		"channelsFlushed":     flushed,
		"channelsNotReady":    notReady,
		"channelsEmptied":     emptied,
		"entriesDrained":      entriesDrained,
		"recipientsNotified":  recipientsNotified,
		"notificationsSent":   pushed,
		"flushed":             flushedDetail,
		"missedStreamLen":     missedLen,
		"missedStreamPending": missedPending,
		"missedStreamLag":     missedLag,
	}
	return
}

// channelTitle reads channel:profile:<ch>.title (a RedisJSON doc), so a
// bulk push can say "3 new signals in Revenue" instead of the raw slug.
// Best-effort: "" when there's no profile or no RedisJSON.
func channelTitle(client *redis.Client, ch string) string {
	raw, err := client.Do(bg(), "JSON.GET", "channel:profile:"+strings.TrimSpace(ch)).Text()
	if err != nil || raw == "" {
		return ""
	}
	var doc struct {
		Title string `json:"title"`
	}
	if json.Unmarshal([]byte(raw), &doc) != nil {
		return ""
	}
	return strings.TrimSpace(doc.Title)
}
