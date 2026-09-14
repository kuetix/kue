package transitions

import (
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type databaseTransitions struct {
	workflow.BaseServiceTransition
}

func NewDatabaseTransitions() interfaces.ServiceTransitions {
	return &databaseTransitions{}
}

// Ping opens (or reuses) the store at path and reports whether it is reachable.
func (t *databaseTransitions) Ping(path string) (r domain.FlowStepResult) {
	_, resolvedPath, err := resolveDB(path)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"path":      resolvedPath,
		"connected": true,
	}
	return
}
