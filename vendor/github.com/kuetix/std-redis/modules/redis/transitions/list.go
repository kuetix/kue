package transitions

import (
	"context"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type listTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewListTransitions() interfaces.ServiceTransitions {
	return &listTransitions{client: newClient()}
}

// LPush prepends value to the list stored at key, returning the new length.
func (t *listTransitions) LPush(key, value string) (r domain.FlowStepResult) {
	length, err := t.client.LPush(context.Background(), key, value).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "length": length}
	return
}

// RPush appends value to the list stored at key, returning the new length.
func (t *listTransitions) RPush(key, value string) (r domain.FlowStepResult) {
	length, err := t.client.RPush(context.Background(), key, value).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "length": length}
	return
}

// LPop removes and returns the first element of the list stored at key.
func (t *listTransitions) LPop(key string) (r domain.FlowStepResult) {
	value, err := t.client.LPop(context.Background(), key).Result()
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

// RPop removes and returns the last element of the list stored at key.
func (t *listTransitions) RPop(key string) (r domain.FlowStepResult) {
	value, err := t.client.RPop(context.Background(), key).Result()
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

// LRange returns elements of the list stored at key between start and stop
// (inclusive, 0-based; negative indexes count from the end, as in redis).
func (t *listTransitions) LRange(key string, start, stop int) (r domain.FlowStepResult) {
	values, err := t.client.LRange(context.Background(), key, int64(start), int64(stop)).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = values
	return
}

// LLen returns the length of the list stored at key.
func (t *listTransitions) LLen(key string) (r domain.FlowStepResult) {
	length, err := t.client.LLen(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "length": length}
	return
}
