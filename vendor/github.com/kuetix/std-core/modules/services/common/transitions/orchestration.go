package transitions

import (
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type orchestrationTransitions struct {
	workflow.BaseServiceTransition
}

func NewOrchestrationTransitions() interfaces.ServiceTransitions {
	return &orchestrationTransitions{}
}

// RunWorkflow executes a workflow with the current WorkerSessionContext using WorkflowRunner
func (ot *orchestrationTransitions) RunWorkflow(p *workflow.WorkerSessionContext, name string) (response map[string]*workflow.WorkerResponse, err error) {
	return workflow.ExecuteWithRunnerAndSharedContext(p, "workflow", name)
}

// RunFeature executes a feature with the current WorkerSessionContext using FeatureRunner
func (ot *orchestrationTransitions) RunFeature(p *workflow.WorkerSessionContext, name string) (response map[string]*workflow.WorkerResponse, err error) {
	return workflow.ExecuteWithRunnerAndSharedContext(p, "feature", name)
}

// RunSolution executes a solution with the current WorkerSessionContext using SolutionRunner
func (ot *orchestrationTransitions) RunSolution(p *workflow.WorkerSessionContext, name string) (response map[string]*workflow.WorkerResponse, err error) {
	return workflow.ExecuteWithRunnerAndSharedContext(p, "solution", name)
}

// RunWorkflowChain executes multiple workflows in sequence with shared context
func (ot *orchestrationTransitions) RunWorkflowChain(p *workflow.WorkerSessionContext, names []string) (responses []map[string]*workflow.WorkerResponse, err error) {
	engine := p.Engine
	app := engine.GetApplication()
	wfConfig := engine.GetWorkflowConfig()

	runner := workflow.NewWorkflowRunner(wfConfig, app)
	responses = make([]map[string]*workflow.WorkerResponse, 0, len(names))

	for _, name := range names {
		response, runErr := runner.RunWithSharedContext(p, name)
		if runErr != nil {
			return responses, runErr
		}
		responses = append(responses, response)

		// Check if there was an error and stop the chain if needed
		for _, resp := range response {
			if resp.IsError() {
				return responses, nil
			}
		}
	}

	return responses, nil
}

// RunFeatureChain executes multiple features in sequence with shared context
func (ot *orchestrationTransitions) RunFeatureChain(p *workflow.WorkerSessionContext, names []string) (responses []map[string]*workflow.WorkerResponse, err error) {
	engine := p.Engine
	app := engine.GetApplication()
	wfConfig := engine.GetWorkflowConfig()

	runner := workflow.NewFeatureRunner(wfConfig, app)
	return runner.RunWorkflowChain(p, names)
}

// RunSolutionChain executes multiple solutions in sequence with shared context
func (ot *orchestrationTransitions) RunSolutionChain(p *workflow.WorkerSessionContext, names []string) (responses []map[string]*workflow.WorkerResponse, err error) {
	responses = make([]map[string]*workflow.WorkerResponse, 0, len(names))

	for _, name := range names {
		response, runErr := workflow.ExecuteWithRunnerAndSharedContext(p, "solution", name)
		if runErr != nil {
			return responses, runErr
		}
		responses = append(responses, response)

		// Check if there was an error and stop the chain if needed
		for _, resp := range response {
			if resp.IsError() {
				return responses, nil
			}
		}
	}

	return responses, nil
}
