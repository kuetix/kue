package transitions

import (
	"errors"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type keysTransitions struct {
	workflow.BaseServiceTransition
}

func NewKeysTransitions() interfaces.ServiceTransitions {
	return &keysTransitions{}
}

// Build joins prefix and id into a "prefix:id" Redis key. Workflow
// action-argument expressions only accept a single bare $dotted.path
// reference (no string concatenation), so building a collection key like
// "user:profile:<id>" from its parts has to happen here in Go rather than
// inline in a .wsl file.
func (t *keysTransitions) Build(prefix, id string) (r domain.FlowStepResult) {
	if prefix == "" || id == "" {
		r.Error = errors.New("prefix and id are both required")
		r.StatusCode = http.StatusBadRequest
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"key": prefix + ":" + id}
	return
}
