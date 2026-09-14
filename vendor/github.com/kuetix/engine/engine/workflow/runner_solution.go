package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
)

// SolutionRunner handles execution of solution type flows
// Solutions can run multiple features and workflows with shared context
type SolutionRunner struct {
	config      domain.WorkflowConfigItem
	application domain.Application
}

// NewSolutionRunner creates a new solution runner
func NewSolutionRunner(config domain.WorkflowConfigItem, app domain.Application) *SolutionRunner {
	return &SolutionRunner{
		config:      config,
		application: app,
	}
}

// Run executes a solution with its own context
//
//goland:noinspection GoUnusedParameter
func (sr *SolutionRunner) Run(ctx context.Context, name string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) map[string]*WorkerResponse {
	return ExecuteWorkflowRoutine(sr.config, sr.application, name, workflowContext, transitions, options...)
}

// RunWithSharedContext executes a solution with a shared WorkerSessionContext
// Solutions orchestrate features and workflows with shared context by loading and executing the solution WSL file
func (sr *SolutionRunner) RunWithSharedContext(workerSessionContext *WorkerSessionContext, name string) (map[string]*WorkerResponse, error) {
	ctx := workerSessionContext.ServerContext
	if ctx == nil {
		ctx = context.Background()
	}

	engine := workerSessionContext.Engine
	app := engine.GetApplication()
	wfConfig := engine.GetWorkflowConfig()
	workflowContext := workerSessionContext.WorkflowContext.Context()

	// Store parent flow for reference
	(*workflowContext)["Parent"] = workerSessionContext.Flow
	(*workflowContext)["_solution_runner"] = true

	// Check if the name contains a specific flow path (namespace/path.flow_name)
	if strings.Contains(name, ".") {
		// Execute specific solution from WSL file
		responses, err := ExecuteSpecificFlow(wfConfig, app, name, "solution", workflowContext, engine.GetResolvers())
		if err != nil {
			return nil, err
		}
		return responses, nil
	}

	// Execute the solution by loading and running the WSL file (original behavior)
	// The solution WSL file will contain feature and workflow orchestration logic
	responses, _ := ExecuteWorkflow(wfConfig, app, name, workflowContext, engine.GetResolvers())

	return responses, nil
}

// RunFeatureChain executes multiple features in sequence with shared context
func (sr *SolutionRunner) RunFeatureChain(workerSessionContext *WorkerSessionContext, featureNames []string) ([]map[string]*WorkerResponse, error) {
	responses := make([]map[string]*WorkerResponse, 0, len(featureNames))

	featureRunner := NewFeatureRunner(sr.config, sr.application)

	for _, name := range featureNames {
		response, err := featureRunner.RunWithSharedContext(workerSessionContext, name)
		if err != nil {
			return responses, fmt.Errorf("solution runner: error running feature %s: %w", name, err)
		}
		responses = append(responses, response)

		// Check if there was an error and stop the chain if needed
		for _, resp := range response {
			if resp.IsError() {
				return responses, fmt.Errorf("solution runner: feature %s returned error", name)
			}
		}
	}

	return responses, nil
}

// RunWorkflowChain executes multiple workflows in sequence with shared context
func (sr *SolutionRunner) RunWorkflowChain(workerSessionContext *WorkerSessionContext, workflowNames []string) ([]map[string]*WorkerResponse, error) {
	responses := make([]map[string]*WorkerResponse, 0, len(workflowNames))

	workflowRunner := NewWorkflowRunner(sr.config, sr.application)

	for _, name := range workflowNames {
		response, err := workflowRunner.RunWithSharedContext(workerSessionContext, name)
		if err != nil {
			return responses, fmt.Errorf("solution runner: error running workflow %s: %w", name, err)
		}
		responses = append(responses, response)

		// Check if there was an error and stop the chain if needed
		for _, resp := range response {
			if resp.IsError() {
				return responses, fmt.Errorf("solution runner: workflow %s returned error", name)
			}
		}
	}

	return responses, nil
}

// RunMixedChain executes a mix of features and workflows in sequence with shared context
func (sr *SolutionRunner) RunMixedChain(workerSessionContext *WorkerSessionContext, items []struct {
	Type string // "feature" or "workflow"
	Name string
}) ([]map[string]*WorkerResponse, error) {
	responses := make([]map[string]*WorkerResponse, 0, len(items))

	featureRunner := NewFeatureRunner(sr.config, sr.application)
	workflowRunner := NewWorkflowRunner(sr.config, sr.application)

	for _, item := range items {
		var response map[string]*WorkerResponse
		var err error

		switch item.Type {
		case "feature":
			response, err = featureRunner.RunWithSharedContext(workerSessionContext, item.Name)
		case "workflow":
			response, err = workflowRunner.RunWithSharedContext(workerSessionContext, item.Name)
		default:
			return responses, fmt.Errorf("solution runner: unknown type %s for item %s", item.Type, item.Name)
		}

		if err != nil {
			return responses, fmt.Errorf("solution runner: error running %s %s: %w", item.Type, item.Name, err)
		}

		responses = append(responses, response)

		// Check if there was an error and stop the chain if needed
		for _, resp := range response {
			if resp.IsError() {
				return responses, fmt.Errorf("solution runner: %s %s returned error", item.Type, item.Name)
			}
		}
	}

	return responses, nil
}

// GetType returns the type this runner handles
func (sr *SolutionRunner) GetType() string {
	return "solution"
}

// Validate checks if the solution configuration is valid
func (sr *SolutionRunner) Validate() error {
	if sr.config.Name == "" {
		return fmt.Errorf("solution runner: config name cannot be empty")
	}
	return nil
}
