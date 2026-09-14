package transitions

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type jsonTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewJsonTransitions() interfaces.ServiceTransitions {
	return &jsonTransitions{client: newClient()}
}

// JSONSet marshals value (typically an object literal built in the workflow)
// to JSON and stores it at key via RedisJSON's JSON.SET, replacing the whole
// document at path "$".
func (t *jsonTransitions) JSONSet(key string, value any) (r domain.FlowStepResult) {
	data, err := json.Marshal(value)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := t.client.Do(context.Background(), "JSON.SET", key, "$", string(data)).Err(); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key}
	return
}

// JSONGet reads the JSON document stored at key via RedisJSON's JSON.GET.
// The decoded document is returned as "value" (so downstream workflow steps
// can dot-path into its fields) alongside the raw JSON text as "raw".
func (t *jsonTransitions) JSONGet(key string) (r domain.FlowStepResult) {
	raw, err := t.client.Do(context.Background(), "JSON.GET", key).Text()
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

	var value interface{}
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "value": value, "raw": raw}
	return
}

// JSONSetPath marshals value and stores it at the given RedisJSON path within
// the document at key (e.g. path "$.password" to patch a single field)
// without replacing the rest of the document.
func (t *jsonTransitions) JSONSetPath(key, path string, value any) (r domain.FlowStepResult) {
	data, err := json.Marshal(value)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := t.client.Do(context.Background(), "JSON.SET", key, path, string(data)).Err(); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "path": path}
	return
}

// JSONDel removes the JSON document stored at key via RedisJSON's JSON.DEL.
func (t *jsonTransitions) JSONDel(key string) (r domain.FlowStepResult) {
	count, err := t.client.Do(context.Background(), "JSON.DEL", key).Int64()
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
