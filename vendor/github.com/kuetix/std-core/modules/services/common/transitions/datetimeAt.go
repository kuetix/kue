package transitions

import (
	"errors"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/domain/timeAt"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/helpers"
)

type datetimeAtTransitions struct {
	workflow.BaseServiceTransition
}

func NewDatetimeAtTransitions() interfaces.ServiceTransitions {
	return &datetimeAtTransitions{}
}

func (dtAt *datetimeAtTransitions) Update(entity interface{}) (r domain.FlowStepResult) {
	dateTimeAt, ok := helpers.FieldValue(entity, "DateTimeAt")
	if !ok {
		dtAt.HandleError(errors.New("can't retrieve DateTimeAt object"), http.StatusInternalServerError)
		r.Success = false
		return
	}

	var at timeAt.DateTimeAt
	if dateTimeAt == nil || (at.CreatedAt == nil && at.UpdatedAt == nil) {
		at = timeAt.DateTimeAt{}
		at.JustCreated()
	} else {
		at = dateTimeAt.(timeAt.DateTimeAt)
		at.UpdatedAtFrom(&at)
	}
	helpers.SetFieldValue(entity, "DateTimeAt", at)

	r.Success = true
	return
}
