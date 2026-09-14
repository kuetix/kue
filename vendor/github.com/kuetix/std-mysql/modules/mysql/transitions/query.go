package transitions

import (
	"database/sql"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type queryTransitions struct {
	workflow.BaseServiceTransition
	db *sql.DB
}

func NewQueryTransitions() interfaces.ServiceTransitions {
	return &queryTransitions{db: newDB()}
}

// normalizeValue converts driver-returned values into plain JSON-friendly
// types; MySQL rows scan most non-numeric columns into []byte.
func normalizeValue(value interface{}) interface{} {
	if b, ok := value.([]byte); ok {
		return string(b)
	}
	return value
}

// scanRows reads every remaining row from rows into a slice of column-name-keyed maps.
func scanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	results := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		scanArgs := make([]interface{}, len(columns))
		for i := range values {
			scanArgs[i] = &values[i]
		}
		if err := rows.Scan(scanArgs...); err != nil {
			return nil, err
		}

		row := make(map[string]interface{}, len(columns))
		for i, col := range columns {
			row[col] = normalizeValue(values[i])
		}
		results = append(results, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

// Query runs a parameterized SELECT and returns every matching row.
func (t *queryTransitions) Query(sqlText string, args []interface{}) (r domain.FlowStepResult) {
	rows, err := t.db.Query(sqlText, args...)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	defer rows.Close()

	results, err := scanRows(rows)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = results
	return
}

// QueryRow runs a parameterized SELECT and returns the first matching row.
func (t *queryTransitions) QueryRow(sqlText string, args []interface{}) (r domain.FlowStepResult) {
	rows, err := t.db.Query(sqlText, args...)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}
	defer rows.Close()

	results, err := scanRows(rows)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}
	if len(results) == 0 {
		r.Error = sql.ErrNoRows
		r.StatusCode = http.StatusNotFound
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = results[0]
	return
}
