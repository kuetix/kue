package workflow

import (
	"context"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/helpers"
)

type HandleWorkflow interface {
	ProcessWorkflow(c context.Context, workflowName string, workflowContext *map[string]interface{}) map[string]*WorkerResponse
	GetEngine() *EngineInterface
	GetEngineName() string
	GetWorker() *Worker
}

type handleWorkflow struct {
	WorkflowConfig domain.WorkflowConfigItem
	Application    domain.Application
	Engine         *EngineInterface
	Worker         *Worker
	workflowName   string
	transitions    map[string]interfaces.ServiceTransitions
	options        map[string]interface{}
}

// NewHandleWorkflow ...
// parameter options ...map[string]interface{} - list of options:
//   - debug bool
//   - preventLogError bool
//   - preventLogInfo bool
func NewHandleWorkflow(wfConfig domain.WorkflowConfigItem, app domain.Application, workflowName string, transitions []string, options ...map[string]interface{}) HandleWorkflow {
	var ts map[string]interfaces.ServiceTransitions
	ts = make(map[string]interfaces.ServiceTransitions)
	for _, t := range transitions {
		ts[t] = di.Resolve(defines.TransitionPrefix + t).(interfaces.ServiceTransitions)
	}
	var opts map[string]interface{}
	defaultOptions := map[string]interface{}{
		"debug":           false,
		"preventLogError": false,
		"preventLogInfo":  false,
	}

	options = append([]map[string]interface{}{defaultOptions}, options...)
	opts = helpers.MergeMapsLevel0(options...)
	return &handleWorkflow{
		WorkflowConfig: wfConfig,
		Application:    app,
		workflowName:   workflowName,
		transitions:    ts,
		options:        opts,
	}
}

func (hw *handleWorkflow) ProcessWorkflow(c context.Context, workflowName string, workflowContext *map[string]interface{}) map[string]*WorkerResponse {
	worker := NewWorkflowWorker(NewWorkflowContext(workflowContext), hw.transitions, hw.options)
	worker.SetDebug(hw.options["debug"].(bool))
	hw.Worker = &worker
	workflowEngine := NewWorkflowEngine(hw.GetEngineName(), hw.Application)
	hw.Engine = &workflowEngine
	workflowEngine.SetPreventLogError(hw.options["preventLogError"].(bool))
	result := workflowEngine.Process(c, workflowName, worker)

	return result
}

func (hw *handleWorkflow) GetEngine() *EngineInterface {
	return hw.Engine
}

func (hw *handleWorkflow) GetEngineName() string {
	return hw.Application.EngineName
}

func (hw *handleWorkflow) GetWorker() *Worker {
	return hw.Worker
}
