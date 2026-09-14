package transitions

// push/queue - the producer side of the cue:missed stage. Called from
// zmist's broadcast_relay.wsl (with the fan-out's delivered-user list) and
// from signal/create.wsl's REST fallback path (delivered list empty - the
// decide workflow re-checks live membership anyway).
//
// Every parameter is prefixed ("missedXxx", not "channelId"/"type"/...) -
// the same vendored-engine argument-binding precaution documented in
// zmist/modules/protocol/transitions/storage.go's EnqueuePersist: a bare
// parameter name can be silently shadowed by a same-named value elsewhere
// in the workflow's scope.

import (
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

// notifiableType reports whether a message type should ever produce a push.
// Round-control markers and empty relays never do.
func notifiableType(t string) bool {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "round", "":
		return false
	default:
		return true
	}
}

// splitRoomKey pulls channelId + groupNo out of "cue:room:<channelId>:<groupNo>".
// Channel ids are slugs (no colons); groupNo is the trailing segment.
func splitRoomKey(roomKey string) (channelID, groupNo string) {
	rest := strings.TrimPrefix(strings.TrimSpace(roomKey), "cue:room:")
	if i := strings.LastIndex(rest, ":"); i >= 0 {
		return rest[:i], rest[i+1:]
	}
	return rest, "1"
}

const missedStream = "cue:missed"

type queueTransitions struct {
	workflow.BaseServiceTransition
}

// NewQueueTransitions is the DI constructor for the `push/queue` class.
func NewQueueTransitions() interfaces.ServiceTransitions {
	return &queueTransitions{}
}

// EnqueueMissed hands one channel event to the decision->push stage.
// missedDeliveredIds is the set the live fan-out already reached (WSL
// passes it as []interface{} or a joined string); "" / empty is fine.
func (t *queueTransitions) EnqueueMissed(missedChannelId string, missedMessageId string, missedType string, missedSenderId string, missedDeliveredIds interface{}, missedGroupNo string) (r domain.FlowStepResult) {
	missedChannelId = strings.TrimSpace(missedChannelId)
	if missedChannelId == "" {
		return failResult(http.StatusBadRequest, errRequired("missedChannelId"))
	}
	if missedGroupNo == "" {
		missedGroupNo = "1"
	}
	if !notifiableType(missedType) {
		r.Success = true
		r.StatusCode = http.StatusOK
		r.Response = map[string]interface{}{"enqueued": false, "reason": "type not notifiable"}
		return
	}

	delivered := strings.Join(toStringSlice(missedDeliveredIds), ",")

	err := streamXAdd(bg(), missedStream, map[string]interface{}{
		"channelId":        missedChannelId,
		"messageId":        strings.TrimSpace(missedMessageId),
		"type":             strings.TrimSpace(missedType),
		"senderId":         strings.TrimSpace(missedSenderId),
		"deliveredUserIds": delivered,
		"forcedUserIds":    "",
		"groupNo":          missedGroupNo,
	})
	if err != nil {
		return failResult(http.StatusInternalServerError, err)
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"enqueued": true}
	return
}

// EnqueueTest hands a synthetic entry to the same cue:missed -> decide ->
// scheduler pipeline a real channel event goes through, but with an
// explicit audience (testUserId) instead of a resolved channel member
// set. Used by POST /push/test and cmd/push-test. Delivery still honours
// the target's notification preferences and the bulk-coalescing window -
// that is the point: it tests the real path, not a shortcut around it.
// testChannelId is cosmetic (the notification title the client renders);
// defaults to "zmist". testType defaults to "message".
func (t *queueTransitions) EnqueueTest(testUserId string, testChannelId string, testType string, testMessageId string) (r domain.FlowStepResult) {
	testUserId = strings.TrimSpace(testUserId)
	if testUserId == "" {
		return failResult(http.StatusBadRequest, errRequired("testUserId"))
	}
	testChannelId = strings.TrimSpace(testChannelId)
	if testChannelId == "" {
		testChannelId = "zmist"
	}
	testType = strings.TrimSpace(testType)
	if !notifiableType(testType) {
		testType = "message"
	}
	testMessageId = strings.TrimSpace(testMessageId)
	if testMessageId == "" {
		testMessageId = "push-test"
	}

	err := streamXAdd(bg(), missedStream, map[string]interface{}{
		"channelId":        testChannelId,
		"messageId":        testMessageId,
		"type":             testType,
		"senderId":         "",
		"deliveredUserIds": "",
		"forcedUserIds":    testUserId,
		"groupNo":          "1",
	})
	if err != nil {
		return failResult(http.StatusInternalServerError, err)
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"enqueued": true, "userId": testUserId, "channelId": testChannelId, "type": testType}
	return
}

// EnqueueMissedFromRoom is the broadcast-stage entry point: it takes the
// Cue room key (which encodes channelId + groupNo) and the fan-out's
// delivered-user list, and is a no-op for non-notifiable types (round
// markers) so broadcast_relay.wsl can call it unconditionally.
func (t *queueTransitions) EnqueueMissedFromRoom(missedRoomKey string, missedMessageId string, missedType string, missedSenderId string, missedDeliveredIds interface{}) (r domain.FlowStepResult) {
	channelID, groupNo := splitRoomKey(missedRoomKey)
	if channelID == "" {
		return failResult(http.StatusBadRequest, errRequired("missedRoomKey"))
	}
	if !notifiableType(missedType) {
		r.Success = true
		r.StatusCode = http.StatusOK
		r.Response = map[string]interface{}{"enqueued": false, "reason": "type not notifiable"}
		return
	}

	delivered := strings.Join(toStringSlice(missedDeliveredIds), ",")
	err := streamXAdd(bg(), missedStream, map[string]interface{}{
		"channelId":        channelID,
		"messageId":        strings.TrimSpace(missedMessageId),
		"type":             strings.TrimSpace(missedType),
		"senderId":         strings.TrimSpace(missedSenderId),
		"deliveredUserIds": delivered,
		"forcedUserIds":    "",
		"groupNo":          groupNo,
	})
	if err != nil {
		return failResult(http.StatusInternalServerError, err)
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"enqueued": true, "channelId": channelID}
	return
}
