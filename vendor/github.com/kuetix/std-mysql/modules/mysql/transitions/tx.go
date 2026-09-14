package transitions

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type txTransitions struct {
	workflow.BaseServiceTransition
	db *sql.DB
}

func NewTxTransitions() interfaces.ServiceTransitions {
	return &txTransitions{db: newDB()}
}

var (
	txMu    sync.Mutex
	txCache = map[string]*sql.Tx{}
)

// Begin opens a transaction and returns a txId that Exec, Query, QueryRow,
// Commit, and Rollback use to reach it. The underlying *sql.Tx holds a
// connection out of the pool until Commit or Rollback releases it — every
// Begin must be paired with exactly one of them.
func (t *txTransitions) Begin() (r domain.FlowStepResult) {
	tx, err := t.db.Begin()
	if err != nil {
		r.Error = fmt.Errorf("failed to begin transaction: %w", err)
		r.StatusCode = http.StatusServiceUnavailable
		return
	}

	txId := newTxID()
	txMu.Lock()
	txCache[txId] = tx
	txMu.Unlock()

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"txId": txId}
	return
}

// Exec runs a parameterized INSERT/UPDATE/DELETE (or DDL) statement inside
// the transaction identified by txId.
func (t *txTransitions) Exec(txId, sqlText string, args []interface{}) (r domain.FlowStepResult) {
	tx, err := resolveTx(txId)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	result, err := tx.Exec(sqlText, args...)
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

// ExecMany runs sqlText once per row of rows inside the transaction
// identified by txId, with prefixArgs prepended to every row's own values
// (e.g. a parent id generated earlier in the same transaction). It exists
// so a caller-controlled number of records (e.g. an invoice's line items)
// can be inserted without the caller ever supplying SQL text — only
// positional argument values. sqlText must come from the calling workflow
// itself, never from request input.
//
// prefixArgs, not LAST_INSERT_ID(), is how a parent id reaches each row:
// LAST_INSERT_ID() reflects the most recent auto-increment insert on the
// connection, so from the second row onward it would point at the previous
// row's own generated id instead of the parent's.
//
// rows is []interface{} rather than [][]interface{}: the engine's
// reflection-based action-arg binding can convert a WSL array into
// []interface{}, but not into a nested slice type, so each row is asserted
// to []interface{} here instead of being declared in the signature.
func (t *txTransitions) ExecMany(txId, sqlText string, prefixArgs []interface{}, rows []interface{}) (r domain.FlowStepResult) {
	tx, err := resolveTx(txId)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	var totalRowsAffected int64
	for i, rawRow := range rows {
		row, ok := rawRow.([]interface{})
		if !ok {
			r.Error = fmt.Errorf("row %d: expected an array of values, got %T", i, rawRow)
			r.StatusCode = http.StatusBadRequest
			return
		}
		args := append(append([]interface{}{}, prefixArgs...), row...)
		result, err := tx.Exec(sqlText, args...)
		if err != nil {
			r.Error = fmt.Errorf("row %d: %w", i, err)
			r.StatusCode = http.StatusBadRequest
			return
		}
		affected, _ := result.RowsAffected()
		totalRowsAffected += affected
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"rowsAffected": totalRowsAffected,
		"count":        len(rows),
	}
	return
}

// Query runs a parameterized SELECT inside the transaction identified by
// txId and returns every matching row.
func (t *txTransitions) Query(txId, sqlText string, args []interface{}) (r domain.FlowStepResult) {
	tx, err := resolveTx(txId)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	rows, err := tx.Query(sqlText, args...)
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

// QueryRow runs a parameterized SELECT inside the transaction identified by
// txId and returns the first matching row.
func (t *txTransitions) QueryRow(txId, sqlText string, args []interface{}) (r domain.FlowStepResult) {
	tx, err := resolveTx(txId)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	rows, err := tx.Query(sqlText, args...)
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

// Commit commits the transaction identified by txId and releases its pooled
// connection. txId is no longer valid after this call.
func (t *txTransitions) Commit(txId string) (r domain.FlowStepResult) {
	tx, err := takeTx(txId)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := tx.Commit(); err != nil {
		r.Error = fmt.Errorf("failed to commit transaction: %w", err)
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"committed": true}
	return
}

// Rollback aborts the transaction identified by txId and releases its
// pooled connection. txId is no longer valid after this call.
func (t *txTransitions) Rollback(txId string) (r domain.FlowStepResult) {
	tx, err := takeTx(txId)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := tx.Rollback(); err != nil {
		r.Error = fmt.Errorf("failed to roll back transaction: %w", err)
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"rolledBack": true}
	return
}

func resolveTx(txId string) (*sql.Tx, error) {
	txMu.Lock()
	defer txMu.Unlock()
	tx, ok := txCache[txId]
	if !ok {
		return nil, fmt.Errorf("unknown or already-finished transaction %q", txId)
	}
	return tx, nil
}

// takeTx resolves and removes txId from the cache in one step, so a
// concurrent Commit/Rollback race can't act on the same transaction twice.
func takeTx(txId string) (*sql.Tx, error) {
	txMu.Lock()
	defer txMu.Unlock()
	tx, ok := txCache[txId]
	if !ok {
		return nil, fmt.Errorf("unknown or already-finished transaction %q", txId)
	}
	delete(txCache, txId)
	return tx, nil
}

func newTxID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
