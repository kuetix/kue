package workflow

import "github.com/kuetix/engine/engine/domain/issues"

type BaseServiceTransition struct {
	Ctx *WorkerSessionContext
}

// SetSession sets the worker session context for the transition.
func (b *BaseServiceTransition) SetSession(ctx *WorkerSessionContext) {
	b.Ctx = ctx
}

// GetSession retrieves the worker session context for the transition.
func (b *BaseServiceTransition) GetSession() *WorkerSessionContext {
	return b.Ctx
}

// GetContext retrieves the workflow context map.
func (b *BaseServiceTransition) GetContext() *map[string]interface{} {
	return b.Ctx.WorkflowContext.Context()
}

// S is shorthand for RefreshContext. Name is similar from Starter function.
func (b *BaseServiceTransition) S() *WorkerSessionContext {
	b.Ctx.UpdateContext()
	return b.Ctx
}

// SetResponse sets the response for the worker.
func (b *BaseServiceTransition) SetResponse(response any, statusCode ...int) {
	b.Ctx.Worker.SetResponse(response, statusCode...)
}

// SetStatusCode sets the status code for the worker.
func (b *BaseServiceTransition) SetStatusCode(statusCode int) {
	b.Ctx.Worker.SetStatusCode(statusCode)
}

// SetValue sets a value in the worker session context.
func (b *BaseServiceTransition) SetValue(name string, value any) *WorkerSessionContext {
	return b.Ctx.SetValue(name, value)
}

// GetValue retrieves a value from the worker session context.
func (b *BaseServiceTransition) GetValue(name string) any {
	context := b.Ctx.WorkflowContext.Context()
	if context == nil {
		return nil
	}
	if _, exists := (*context)[name]; !exists {
		if _, exists = (*context)["values"]; exists {
			if _, exists = (*context)["values"].(map[string]interface{})[name]; exists {
				return (*context)["values"].(map[string]interface{})[name]
			} else {
				return nil
			}
		}
	}
	value := (*context)[name]

	return value
}

// Property retrieves a property from the worker session context.
func (b *BaseServiceTransition) Property(key string) interface{} {
	return b.Ctx.Property(key)
}

// SetError sets a single error issue for the worker.
func (b *BaseServiceTransition) SetError(error *issues.Issue, statusCode int) bool {
	return b.Ctx.Worker.SetError(error, statusCode)
}

// SetErrors sets multiple error issues for the worker.
func (b *BaseServiceTransition) SetErrors(errors *issues.Issues, statusCode int) bool {
	return b.Ctx.Worker.SetErrors(errors, statusCode)
}

// HandleError processes an error and sets it in the worker if needed.
func (b *BaseServiceTransition) HandleError(err interface{}, statusCode int) (success bool) {
	return b.Ctx.Worker.HandleError(err, statusCode)
}

func (b *BaseServiceTransition) NewIssue(msg string, statusCode int) bool {
	return b.Ctx.Worker.MergeIssues(issues.NewIssues(issues.NewIssue(msg, nil)), statusCode)
}
