package transitions

import (
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/spf13/cast"
)

type outcomeTransitions struct {
	workflow.BaseServiceTransition
}

// NewOutcomeTransitions is the DI constructor for the `decision/outcome` class.
func NewOutcomeTransitions() interfaces.ServiceTransitions {
	return &outcomeTransitions{}
}

// Resolve maps a decision outcome to the optional caller hint declared in the
// ruleset's `onOutcome` block (e.g. an HTTP status to respond with). It does
// NOT perform any side effect - executing on the decision stays in the
// consuming project's own workflow.
func (t *outcomeTransitions) Resolve(ruleset interface{}, outcome string) (r domain.FlowStepResult) {
	rs, err := loadRuleset(ruleset)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	outcome = strings.TrimSpace(outcome)

	hint := map[string]interface{}{}
	if raw, ok := rs.OnOutcome[outcome]; ok {
		for k, v := range raw {
			hint[k] = v
		}
	}

	response := map[string]interface{}{
		"outcome": outcome,
		"hint":    hint,
		"known":   len(hint) > 0,
	}
	if raw, ok := hint["httpStatus"]; ok {
		if code, convErr := cast.ToIntE(raw); convErr == nil {
			response["httpStatus"] = code
		}
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = response
	return
}
