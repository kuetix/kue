package transitions

import (
	"context"
	"net/http"
	"time"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type stringTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewStringTransitions() interfaces.ServiceTransitions {
	return &stringTransitions{client: newClient()}
}

// Set stores value under key, optionally expiring after ttlSeconds (0 = no expiry).
func (t *stringTransitions) Set(key, value string, ttlSeconds int) (r domain.FlowStepResult) {
	var ttl time.Duration
	if ttlSeconds > 0 {
		ttl = time.Duration(ttlSeconds) * time.Second
	}

	if err := t.client.Set(context.Background(), key, value, ttl).Err(); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key}
	return
}

// Get reads the value stored under key.
func (t *stringTransitions) Get(key string) (r domain.FlowStepResult) {
	value, err := t.client.Get(context.Background(), key).Result()
	if err != nil {
		if err == goredis.Nil {
			r.Error = err
			r.StatusCode = http.StatusNotFound
			return
		}
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "value": value}
	return
}

// GetDel atomically reads key and deletes it in the same round trip -
// unlike a separate Get+Del pair, no other caller can observe or consume
// the value in between, which is what makes this suitable for one-time-use
// tokens (e.g. an OAuth state parameter): two concurrent requests racing
// on the same key can never both see it as present.
//
// Unlike Get/Del above, a missing key never sets r.Error here - only a
// genuine connectivity/infrastructure failure does. A caller consuming a
// one-time token (this action's main use case) needs to branch cleanly on
// "was it there or not" via the `found` field (assert.Bool), not treat a
// route-fail as its "already used/expired" case - confirmed empirically
// that this engine treats any r.Error as fatal and skips straight past
// the calling workflow's own `on fail ->` transition, the same
// std-auth ComparePassword quirk kuetix/zmist's user/user.go documents.
func (t *stringTransitions) GetDel(key string) (r domain.FlowStepResult) {
	value, err := t.client.GetDel(context.Background(), key).Result()
	if err != nil && err != goredis.Nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "value": value, "found": err == nil}
	return
}

// Del removes key. Success carries the operation result; "deleted" reports
// whether a key was actually present to remove.
func (t *stringTransitions) Del(key string) (r domain.FlowStepResult) {
	count, err := t.client.Del(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "deleted": count > 0}
	return
}

// Exists reports whether key is present. The operation itself succeeds
// regardless of the answer; "exists" carries the boolean result in the
// response so a false answer isn't treated as a workflow failure.
func (t *stringTransitions) Exists(key string) (r domain.FlowStepResult) {
	count, err := t.client.Exists(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "exists": count > 0}
	return
}

// Incr increments the integer value stored at key by 1 (creating it at 0 first if missing).
func (t *stringTransitions) Incr(key string) (r domain.FlowStepResult) {
	value, err := t.client.Incr(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "value": value}
	return
}

// Decr decrements the integer value stored at key by 1 (creating it at 0 first if missing).
func (t *stringTransitions) Decr(key string) (r domain.FlowStepResult) {
	value, err := t.client.Decr(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "value": value}
	return
}

// Expire sets key to expire after ttlSeconds. "set" reports whether the
// timeout was actually applied (false if key doesn't exist).
func (t *stringTransitions) Expire(key string, ttlSeconds int) (r domain.FlowStepResult) {
	applied, err := t.client.Expire(context.Background(), key, time.Duration(ttlSeconds)*time.Second).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "set": applied}
	return
}

// TTL returns the remaining time to live for key, in seconds. -1 means key
// exists with no expiry, -2 means key doesn't exist.
func (t *stringTransitions) TTL(key string) (r domain.FlowStepResult) {
	ttl, err := t.client.TTL(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "ttlSeconds": int(ttl.Seconds())}
	return
}
