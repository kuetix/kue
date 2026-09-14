package transitions

import (
	"database/sql"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type execTransitions struct {
	workflow.BaseServiceTransition
	db *sql.DB
}

func NewExecTransitions() interfaces.ServiceTransitions {
	return &execTransitions{db: newDB()}
}

// Exec runs a parameterized INSERT/UPDATE/DELETE (or DDL) statement.
func (t *execTransitions) Exec(sqlText string, args []interface{}) (r domain.FlowStepResult) {
	result, err := t.db.Exec(sqlText, args...)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	lastInsertID, _ := result.LastInsertId()
	rowsAffected, _ := result.RowsAffected()

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"lastInsertId": lastInsertID,
		"rowsAffected": rowsAffected,
	}
	return
}
