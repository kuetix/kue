package transitions

import (
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type workflowTransitions struct {
	workflow.BaseServiceTransition
	engine workflow.EngineInterface
}

func NewWorkflowTransitions() interfaces.ServiceTransitions {
	return &workflowTransitions{}
}

func (wt *workflowTransitions) Prepare() (r domain.FlowStepResult) {
	route, _ := wt.S().Property("route").(*domain.Route)

	wt.SetValue("handleWorkflow", route.HandleWorkflow())

	r.Success = true
	return
}

func (wt *workflowTransitions) Execute() (r domain.FlowStepResult) {
	w, _ := wt.Property("workflow.EngineInterface").(workflow.EngineInterface)
	request, _ := wt.Property("request").(*http.Request)
	handleWorkflow, _ := wt.Property("handleWorkflow").(workflow.HandleWorkflow)
	route, _ := wt.Property("route").(*domain.Route)

	wt.SetValue("request", request)
	result := handleWorkflow.ProcessWorkflow(w.ApplicationContext(), route.Workflow, wt.GetContext())

	wt.SetValue("result", result)

	r.Success = true
	return
}

func (wt *workflowTransitions) Finish() (r domain.FlowStepResult) {
	result, _ := wt.Property("result").(*workflow.WorkerResponse)
	if result.Error != nil {
		wt.Ctx.Worker.MergeIssues(result.Error, result.StatusCode)
	}
	if result.Response != nil {
		wt.SetResponse(result.Response)
	}
	if result.StatusCode != 0 {
		wt.SetStatusCode(result.StatusCode)
	} else {
		wt.SetStatusCode(http.StatusInternalServerError)
	}

	r.Success = result.Error == nil
	return
}

func (wt *workflowTransitions) IsContinue(isQuit bool) (r domain.FlowStepResult) {
	wt.Ctx.RemoveValue("quit")

	if isQuit {
		r.Success = false
		return
	}

	r.Success = true
	return
}

func (wt *workflowTransitions) Quit(value string) (r domain.FlowStepResult) {
	wt.SetValue(value, "quit")

	r.Success = true
	return
}
