package transitions

import (
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type documentTransitions struct {
	workflow.BaseServiceTransition
}

func NewDocumentTransitions() interfaces.ServiceTransitions {
	return &documentTransitions{}
}

// Set stores value under id in collection and (re)builds its indexes for the
// given fields.
func (t *documentTransitions) Set(path, collection, id string, value map[string]interface{}, indexes []string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := col.Set(id, value, indexes...); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection}
	return
}

// SetRaw stores value under id without touching indexes (wraps filejsondb's DoSet).
func (t *documentTransitions) SetRaw(path, collection, id string, value map[string]interface{}) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := col.DoSet(id, value); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection}
	return
}

// Get reads the value stored under id, checking indexes if id is not a direct
// key (mirrors filejsondb.Collection.Get lookup fallback).
func (t *documentTransitions) Get(path, collection, id string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	var value interface{}
	if err := col.Get(id, &value); err != nil {
		r.Error = err
		r.StatusCode = http.StatusNotFound
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = value
	return
}

// Update overwrites the value stored under id, keeping its existing indexes intact.
func (t *documentTransitions) Update(path, collection, id string, value map[string]interface{}) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := col.Update(id, value); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection}
	return
}

// Delete removes the value (and its indexes, if any) stored under id.
func (t *documentTransitions) Delete(path, collection, id string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	if err := col.Delete(id); err != nil {
		r.Error = err
		r.StatusCode = http.StatusInternalServerError
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection, "deleted": true}
	return
}

// Exists reports whether id is present. The operation itself succeeds
// regardless of the answer; "exists" carries the boolean result in the
// response so a false answer isn't treated as a workflow failure.
func (t *documentTransitions) Exists(path, collection, id string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	exists := col.Exists(id)

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"id": id, "collection": collection, "exists": exists}
	return
}
