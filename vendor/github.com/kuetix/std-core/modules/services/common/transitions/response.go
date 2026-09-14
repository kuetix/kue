package transitions

import (
	"errors"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type responseTransitions struct {
	workflow.BaseServiceTransition
}

//goland:noinspection GoUnusedExportedFunction
func NewResponseTransitions() interfaces.ServiceTransitions {
	return &responseTransitions{}
}

func (et *responseTransitions) End() (r domain.FlowStepResult) {
	r.Success = true
	return
}

func (et *responseTransitions) ResponseSuccess(isSuccess bool) (r domain.FlowStepResult) {
	et.SetResponse(isSuccess)

	r.Success = true
	return
}

func (et *responseTransitions) ResponseLastSuccess() (r domain.FlowStepResult) {
	et.SetResponse(et.S().Property("workflow.Last.Success"))

	r.Success = true
	return
}

func (et *responseTransitions) True(statusCode int) (r domain.FlowStepResult) {
	if statusCode == -2 {
		statusCode = http.StatusBadRequest
	}
	if statusCode == -1 {
		statusCode = http.StatusNoContent
	}

	r.Success = true
	r.StatusCode = statusCode
	r.Response = r.Success
	return
}

func (et *responseTransitions) False(statusCode int) (r domain.FlowStepResult) {
	if statusCode == -2 {
		statusCode = http.StatusBadRequest
	}
	if statusCode == -1 {
		statusCode = http.StatusNoContent
	}
	et.SetResponse(false, statusCode)

	r.Success = true
	return
}

func (et *responseTransitions) SetValue(name string, value string) (r domain.FlowStepResult) {
	et.Ctx.SetValue(name, value)

	r.Success = true
	return
}

func (et *responseTransitions) Response(value any, statusCode int) (r domain.FlowStepResult) {
	r.Response = value

	r.StatusCode = statusCode

	if statusCode == -2 {
		r.StatusCode = http.StatusBadRequest
	}
	if statusCode == -1 {
		r.StatusCode = http.StatusOK
	}

	r.Success = true
	return
}

func (et *responseTransitions) ResponseError(message string, code int) (r domain.FlowStepResult) {
	r.Error = errors.New(message)
	r.Response = message

	r.StatusCode = code

	if code == -2 {
		r.StatusCode = http.StatusBadRequest
	}
	if code == -1 {
		r.StatusCode = http.StatusOK
	}

	r.Success = false
	return
}

func (et *responseTransitions) ResponseEntity(property interface{}, statusCode int) (r domain.FlowStepResult) {
	et.SetResponse(property)

	if statusCode == -2 {
		statusCode = http.StatusBadRequest
	}
	if statusCode == -1 {
		statusCode = http.StatusOK
	}
	et.SetStatusCode(statusCode)

	r.Success = true
	return
}

func (et *responseTransitions) ResponseEntities(entities interface{}, statusCode int) (r domain.FlowStepResult) {
	et.SetResponse(entities)

	if statusCode == -2 {
		statusCode = http.StatusBadRequest
	}
	if statusCode == -1 {
		statusCode = http.StatusOK
	}
	et.SetStatusCode(statusCode)

	r.Success = true
	return
}
