package transitions

import (
	"context"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type hashTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewHashTransitions() interfaces.ServiceTransitions {
	return &hashTransitions{client: newClient()}
}

// HSet sets field to value within the hash stored at key.
func (t *hashTransitions) HSet(key, field, value string) (r domain.FlowStepResult) {
	if err := t.client.HSet(context.Background(), key, field, value).Err(); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "field": field}
	return
}

// HGet reads field from the hash stored at key.
func (t *hashTransitions) HGet(key, field string) (r domain.FlowStepResult) {
	value, err := t.client.HGet(context.Background(), key, field).Result()
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
	r.Response = map[string]interface{}{"key": key, "field": field, "value": value}
	return
}

// HGetAll returns every field/value pair in the hash stored at key.
func (t *hashTransitions) HGetAll(key string) (r domain.FlowStepResult) {
	fields, err := t.client.HGetAll(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = fields
	return
}

// HDel removes field from the hash stored at key.
func (t *hashTransitions) HDel(key, field string) (r domain.FlowStepResult) {
	count, err := t.client.HDel(context.Background(), key, field).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "field": field, "deleted": count > 0}
	return
}

// HExists reports whether field is present in the hash stored at key. The
// operation itself succeeds regardless of the answer; "exists" carries the
// boolean result in the response so a false answer isn't treated as a
// workflow failure.
func (t *hashTransitions) HExists(key, field string) (r domain.FlowStepResult) {
	exists, err := t.client.HExists(context.Background(), key, field).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "field": field, "exists": exists}
	return
}
