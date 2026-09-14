package transitions

// push/facts - assemble the fact set decide.wsl hands to the std-decision
// ruleset. Kept as one Go step because WSL can't turn an assert result
// into a boolean map value, and the ruleset needs isReport as a real bool.

import (
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/spf13/cast"
)

type factsTransitions struct {
	workflow.BaseServiceTransition
}

// NewFactsTransitions is the DI constructor for the `push/facts` class.
func NewFactsTransitions() interfaces.ServiceTransitions {
	return &factsTransitions{}
}

// Build returns { facts: {...} } ready for decision/facts.Prepare.
func (t *factsTransitions) Build(sendType string, missedCount interface{}, hasBurst interface{}, oldestPendingAgeSec interface{}) (r domain.FlowStepResult) {
	facts := map[string]interface{}{
		"isReport":            strings.EqualFold(strings.TrimSpace(sendType), "report"),
		"missedCount":         cast.ToInt(missedCount),
		"hasBurst":            cast.ToBool(hasBurst),
		"oldestPendingAgeSec": cast.ToInt(oldestPendingAgeSec),
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"facts": facts}
	return
}
