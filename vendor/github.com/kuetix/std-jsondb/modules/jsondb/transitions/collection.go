package transitions

import (
	"encoding/json"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type collectionTransitions struct {
	workflow.BaseServiceTransition
}

func NewCollectionTransitions() interfaces.ServiceTransitions {
	return &collectionTransitions{}
}

// GetAll returns every stored value in collection, keyed by id. Note this
// includes ".idx" index entries alongside regular documents, since filejsondb
// keeps both in the same collection namespace.
func (t *collectionTransitions) GetAll(path, collection string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	raw := col.GetAll()
	items := make(map[string]interface{}, len(raw))
	for id, data := range raw {
		var value interface{}
		if err := json.Unmarshal(data, &value); err == nil {
			items[id] = value
		}
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = items
	return
}

// Len returns the number of records stored in collection.
func (t *collectionTransitions) Len(path, collection string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"collection": collection, "count": col.Len()}
	return
}

// Clear purges collection's in-memory cache, forcing the next read to come
// from disk.
func (t *collectionTransitions) Clear(path, collection string) (r domain.FlowStepResult) {
	col, _, err := resolveCollection(path, collection)
	if err != nil {
		r.Error = err
		r.StatusCode = http.StatusBadRequest
		return
	}

	col.Clear()

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{"collection": collection, "cleared": true}
	return
}
