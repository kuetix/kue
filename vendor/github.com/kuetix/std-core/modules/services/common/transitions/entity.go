package transitions

import (
	"fmt"
	"net/http"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/helpers"

	"github.com/kuetix/engine/engine/workflow"
	"github.com/mitchellh/mapstructure"
)

type entityTransitions struct {
	workflow.BaseServiceTransition
}

func NewEntityTransitions() interfaces.ServiceTransitions {
	return &entityTransitions{}
}

func (et *entityTransitions) Merge(merge map[string]interface{}, exclude map[string]interface{}) (r domain.FlowStepResult) {
	entities := make(map[string]interface{})

	for dstName, srcName := range merge {
		entities[dstName] = srcName
		_, dstEntity, err := et.Ctx.GetProperty(dstName)
		if !et.HandleError(err, http.StatusBadRequest) {
			r.Success = false
			return
		}
		_, srcEntity, err := et.Ctx.GetProperty(srcName.(string))
		if !et.HandleError(err, http.StatusBadRequest) {
			r.Success = false
			return
		}
		dstEntityBackup := di.Resolve(fmt.Sprintf("entity:%s", dstName))
		err = mapstructure.Decode(dstEntity, &dstEntityBackup)
		if err != nil {
			et.SetError(issues.NewIssue(err.Error(), err), http.StatusInternalServerError)
			r.Success = false
			return
		}
		et.SetValue(fmt.Sprintf("%s:before", dstName), dstEntityBackup)
		srcExclude, ok := exclude[dstName]
		if !ok {
			srcExclude = make(map[string]interface{})
		}
		merged := helpers.MergeObjectsToMap(srcExclude.(map[string]interface{}), dstEntity, srcEntity)
		entity := di.Resolve(fmt.Sprintf("entity:%s", dstName))
		err = mapstructure.Decode(merged, &entity)
		if err != nil {
			et.SetError(issues.NewIssue(err.Error(), err), http.StatusInternalServerError)
			r.Success = false
			return
		}
		et.SetValue(dstName, entity)
	}

	r.Success = true
	return
}

func (et *entityTransitions) Entity(name string, returnTo string, value map[string]interface{}) (r domain.FlowStepResult) {
	entity := di.Resolve(fmt.Sprintf("entity:%s", name))
	err := et.Ctx.ProcessPropertiesRecursively(value)
	if err != nil {
		r.Success = false
		return
	}
	err = mapstructure.Decode(value, &entity)
	if err != nil {
		et.SetError(issues.NewIssue(err.Error(), err), http.StatusInternalServerError)
		r.Success = false
		return
	}

	et.SetValue(returnTo, entity)

	r.Success = true
	return
}
