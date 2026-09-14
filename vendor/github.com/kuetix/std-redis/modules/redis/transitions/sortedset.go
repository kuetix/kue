package transitions

import (
	"context"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type sortedsetTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewSortedsetTransitions() interfaces.ServiceTransitions {
	return &sortedsetTransitions{client: newClient()}
}

// ZAdd adds member with score to the sorted set stored at key.
func (t *sortedsetTransitions) ZAdd(key, member string, score float64) (r domain.FlowStepResult) {
	added, err := t.client.ZAdd(context.Background(), key, goredis.Z{Score: score, Member: member}).Result()
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": key, "member": member, "score": score, "added": added > 0}
	return
}

// ZRem removes member from the sorted set stored at key.
func (t *sortedsetTransitions) ZRem(key, member string) (r domain.FlowStepResult) {
	removed, err := t.client.ZRem(context.Background(), key, member).Result()
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

// ZScore returns member's score in the sorted set stored at key.
func (t *sortedsetTransitions) ZScore(key, member string) (r domain.FlowStepResult) {
	score, err := t.client.ZScore(context.Background(), key, member).Result()
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
	r.Response = map[string]interface{}{"key": key, "member": member, "score": score}
	return
}

// ZRange returns members of the sorted set stored at key between start and
// stop (inclusive, 0-based ranks; negative indexes count from the end).
func (t *sortedsetTransitions) ZRange(key string, start, stop int) (r domain.FlowStepResult) {
	members, err := t.client.ZRange(context.Background(), key, int64(start), int64(stop)).Result()
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

// ZRangeByLex returns members of the sorted set stored at key within the
// given lexicographical range (all members must share score 0 for this to
// be meaningful - see ZAdd). min/max use the standard Redis lex-range
// syntax: "-"/"+" for unbounded, "[value" for inclusive, "(value" for
// exclusive. This is the standard "prefix search without RediSearch"
// primitive: querying with min="[prefix" and max="[prefix\xff" returns
// every member starting with prefix.
func (t *sortedsetTransitions) ZRangeByLex(key, min, max string) (r domain.FlowStepResult) {
	members, err := t.client.ZRangeByLex(context.Background(), key, &goredis.ZRangeBy{Min: min, Max: max}).Result()
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

// LexPrefixBounds returns the inclusive [min, max] bounds that, passed to
// ZRangeByLex, match every member starting with prefix. Exists purely
// because WSL action-argument expressions can't build these bounds inline
// (no string concatenation) - see ZRangeByLex's doc comment for the
// underlying "[prefix" / "[prefix\xff" pattern.
func (t *sortedsetTransitions) LexPrefixBounds(prefix string) (r domain.FlowStepResult) {
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"min": "[" + prefix, "max": "[" + prefix + "\xff"}
	return
}
