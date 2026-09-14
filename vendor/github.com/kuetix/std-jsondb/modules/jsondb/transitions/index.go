package transitions

import (
	"fmt"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type indexTransitions struct {
	workflow.BaseServiceTransition
}

func NewIndexTransitions() interfaces.ServiceTransitions {
	return &indexTransitions{}
}

// SetIndex (re)builds the indexes for id/value across the given fields,
// merging with any indexes that already exist for id.
func (t *indexTransitions) SetIndex(path, collection, id string, value map[string]interface{}, fields []string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := col.SetIndex(id, value, fields...); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection, "indexed": fields}
	return
}

// GetIndexes returns the index entries recorded for id.
func (t *indexTransitions) GetIndexes(path, collection, id string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	_, indexes, err := col.GetIndexes(id)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusNotFound
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = indexes
	return
}

// IsExistsIndexes reports whether index entries exist for id. The operation
// itself succeeds regardless of the answer; "exists" carries the boolean
// result in the response so a false answer isn't treated as a workflow failure.
//
// This goes through GetIndexes rather than filejsondb's own IsExistsIndexes:
// AddIndex stores index blobs under a hashed id, and only GetIndexes/LoadIndexes
// implement the raw-then-hashed lookup fallback needed to find them again;
// IsExistsIndexes only tries the raw id and reports false for anything indexed
// via the normal Set/SetIndex path.
func (t *indexTransitions) IsExistsIndexes(path, collection, id string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	_, indexes, err := col.GetIndexes(id)
	exists := err == nil && len(indexes) > 0

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection, "exists": exists}
	return
}

// DeleteIndexes removes the index entries recorded for id.
//
// This doesn't use filejsondb's own DeleteIndexes: like IsExistsIndexes, it
// only deletes the raw "<id>.idx" key, never the hashed key AddIndex actually
// writes, so it silently no-ops for anything indexed via Set/SetIndex. Instead
// this looks the entry up via GetIndexes (which has the correct fallback) and
// deletes the resolved key straight from the underlying collection, then
// clears the cache since that bypasses filejsondb's own cache invalidation.
func (t *indexTransitions) DeleteIndexes(path, collection, id string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	idx, indexes, err := col.GetIndexes(id)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusNotFound
		return
	}
	if len(indexes) == 0 {
		r.Error = fmt.Errorf("no indexes found for id %q", id)
		r.StatusCode = http.StatusNotFound
		return
	}

	if err := col.Collection().Delete(idx); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}
	col.Clear()

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection, "deleted": true}
	return
}
