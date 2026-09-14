package transitions

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

// verdictsKey is the shared-context slice each rule appends its verdict to.
// decision/engine.Decide reads it back once every rule has run.
const verdictsKey = "decision.verdicts"

type ruleTransitions struct {
	workflow.BaseServiceTransition
}

// NewRuleTransitions is the DI constructor for the `decision/rule` class.
func NewRuleTransitions() interfaces.ServiceTransitions {
	return &ruleTransitions{}
}

func appendVerdict(p *workflow.WorkerSessionContext, verdict map[string]interface{}) []interface{} {
	var list []interface{}
	if p != nil {
		if existing, ok := p.Value(verdictsKey).([]interface{}); ok {
			list = existing
		}
	}
	list = append(list, verdict)
	if p != nil {
		p.SetValue(verdictsKey, list)
	}
	return list
}

// Fire is the terminal action of a rule's state-group when its condition
// held. `id` is the rule id from the ruleset. `outcome` and `reason` are
// optional overrides for the ruleset's `then` / `because`.
//
//	state Fire {
//	  action decision/rule.Fire(id: "blocked-country", reason: "Country on the block list")
//	  on success -> NextRule
//	}
func (t *ruleTransitions) Fire(p *workflow.WorkerSessionContext, id string, outcome string, reason string) (r domain.FlowStepResult) {
	id = strings.TrimSpace(id)
	if id == "" {
		r.Error = fmt.Errorf("id is required")
		r.StatusCode = http.StatusBadRequest
		return
	}
	verdict := map[string]interface{}{
		"id":      id,
		"fired":   true,
		"outcome": strings.TrimSpace(outcome),
		"reason":  strings.TrimSpace(reason),
	}
	list := appendVerdict(p, verdict)
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"verdict": verdict, "count": len(list)}
	return
}

// Skip is the terminal action of a rule's state-group when its condition did
// not hold. Recording skips (rather than just falling through) keeps the
// evaluation trace complete.
//
//	state Skip {
//	  action decision/rule.Skip(id: "blocked-country")
//	  on success -> NextRule
//	}
func (t *ruleTransitions) Skip(p *workflow.WorkerSessionContext, id string) (r domain.FlowStepResult) {
	id = strings.TrimSpace(id)
	if id == "" {
		r.Error = fmt.Errorf("id is required")
		r.StatusCode = http.StatusBadRequest
		return
	}
	verdict := map[string]interface{}{"id": id, "fired": false}
	list := appendVerdict(p, verdict)
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"verdict": verdict, "count": len(list)}
	return
}
