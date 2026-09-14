package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
)

// WorkflowRunner handles execution of workflow type flows
type WorkflowRunner struct {
	config      domain.WorkflowConfigItem
	application domain.Application
}

// NewWorkflowRunner creates a new workflow runner
func NewWorkflowRunner(config domain.WorkflowConfigItem, app domain.Application) *WorkflowRunner {
	return &WorkflowRunner{
		config:      config,
		application: app,
	}
}

// Run executes a workflow with its own context
//
//goland:noinspection GoUnusedParameter
func (wr *WorkflowRunner) Run(ctx context.Context, name string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) map[string]*WorkerResponse {
	return ExecuteWorkflowRoutine(wr.config, wr.application, name, workflowContext, transitions, options...)
}

// RunWithSharedContext executes a workflow with a shared WorkerSessionContext
func (wr *WorkflowRunner) RunWithSharedContext(workerSessionContext *WorkerSessionContext, name string) (map[string]*WorkerResponse, error) {
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

	// Check if the name contains a specific flow path (namespace/path.flow_name)
	if strings.Contains(name, ".") {
		// Execute specific flow from WSL file
		responses, err := ExecuteSpecificFlow(wfConfig, app, name, "workflow", workflowContext, engine.GetResolvers())
		if err != nil {
			return nil, err
		}
		return responses, nil
	}

	// Execute the workflow by loading and running the WSL file (original behavior)
	responses, _ := ExecuteWorkflow(wfConfig, app, name, workflowContext, engine.GetResolvers())

	return responses, nil
}

// GetType returns the type this runner handles
func (wr *WorkflowRunner) GetType() string {
	return "workflow"
}

// Validate checks if the workflow configuration is valid
func (wr *WorkflowRunner) Validate() error {
	if wr.config.Name == "" {
		return fmt.Errorf("workflow runner: config name cannot be empty")
	}
	return nil
}
