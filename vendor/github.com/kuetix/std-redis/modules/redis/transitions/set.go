package transitions

import (
	"context"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type setTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewSetTransitions() interfaces.ServiceTransitions {
	return &setTransitions{client: newClient()}
}

// SAdd adds member to the set stored at key.
func (t *setTransitions) SAdd(key, member string) (r domain.FlowStepResult) {
	added, err := t.client.SAdd(context.Background(), key, member).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "member": member, "added": added > 0}
	return
}

// SRem removes member from the set stored at key.
func (t *setTransitions) SRem(key, member string) (r domain.FlowStepResult) {
	removed, err := t.client.SRem(context.Background(), key, member).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "member": member, "removed": removed > 0}
	return
}

// SMembers returns every member of the set stored at key.
func (t *setTransitions) SMembers(key string) (r domain.FlowStepResult) {
	members, err := t.client.SMembers(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = members
	return
}

// SIsMember reports whether member is present in the set stored at key. The
// operation itself succeeds regardless of the answer; "isMember" carries the
// boolean result in the response so a false answer isn't treated as a
// workflow failure.
func (t *setTransitions) SIsMember(key, member string) (r domain.FlowStepResult) {
	isMember, err := t.client.SIsMember(context.Background(), key, member).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "member": member, "isMember": isMember}
	return
}

// SCard returns the number of members in the set stored at key (0 for a
// missing key, same as a real Redis SCARD - no separate "exists" case to
// branch on). Suited to counting idempotent membership (e.g. "how many
// distinct accounts like this channel") without a separately-maintained
// counter that could drift from the set itself.
func (t *setTransitions) SCard(key string) (r domain.FlowStepResult) {
	count, err := t.client.SCard(context.Background(), key).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "count": count}
	return
}
