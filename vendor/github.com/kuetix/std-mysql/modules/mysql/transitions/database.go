package transitions

import (
	"database/sql"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type databaseTransitions struct {
	workflow.BaseServiceTransition
	db     *sql.DB
	target string
}

func NewDatabaseTransitions() interfaces.ServiceTransitions {
	return &databaseTransitions{db: newDB(), target: redactDSN(resolveDSN())}
}

// Ping reports whether the client's mysql connection is reachable.
func (t *databaseTransitions) Ping() (r domain.FlowStepResult) {
	if err := t.db.Ping(); err != nil {
		r.Error = err
		r.StatusCode = http.StatusServiceUnavailable
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"target":    t.target,
		"connected": true,
	}
	return
}
