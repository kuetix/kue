package transitions

import (
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type contextTransitions struct {
	workflow.BaseServiceTransition
}

//goland:noinspection GoUnusedExportedFunction
func NewContextTransitions() interfaces.ServiceTransitions {
	return &contextTransitions{}
}

// SetContextValue sets a value in the workflow context
func (ct *contextTransitions) SetContextValue(name string, value interface{}) (r domain.FlowStepResult) {
	context := ct.Ctx.WorkflowContext.Context()
	(*context)[name] = value
	ct.Ctx.WorkflowContext.SetContext(context)
	r.Success = true
	return
}
