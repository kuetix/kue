package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
)

// FeatureRunner handles execution of feature type flows
// Features can run multiple workflows in a chain with shared context
type FeatureRunner struct {
	config      domain.WorkflowConfigItem
	application domain.Application
}

// NewFeatureRunner creates a new feature runner
func NewFeatureRunner(config domain.WorkflowConfigItem, app domain.Application) *FeatureRunner {
	return &FeatureRunner{
		config:      config,
		application: app,
	}
}

// Run executes a feature with its own context
//
//goland:noinspection GoUnusedParameter
func (fr *FeatureRunner) Run(ctx context.Context, name string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) map[string]*WorkerResponse {
	return ExecuteWorkflowRoutine(fr.config, fr.application, name, workflowContext, transitions, options...)
}

// RunWithSharedContext executes a feature with a shared WorkerSessionContext
// Features orchestrate workflows with shared context by loading and executing the feature WSL file
func (fr *FeatureRunner) RunWithSharedContext(workerSessionContext *WorkerSessionContext, name string) (map[string]*WorkerResponse, error) {
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
	(*workflowContext)["_feature_runner"] = true

	// Check if the name contains a specific flow path (namespace/path.flow_name)
	if strings.Contains(name, ".") {
		// Execute specific feature from WSL file
		responses, err := ExecuteSpecificFlow(wfConfig, app, name, "feature", workflowContext, engine.GetResolvers())
		if err != nil {
			return nil, err
		}
		return responses, nil
	}

	// Execute the feature by loading and running the WSL file (original behavior)
	// The feature WSL file will contain workflow orchestration logic
	responses, _ := ExecuteWorkflow(wfConfig, app, name, workflowContext, engine.GetResolvers())

	return responses, nil
}

// RunWorkflowChain executes multiple workflows in sequence with shared context
func (fr *FeatureRunner) RunWorkflowChain(workerSessionContext *WorkerSessionContext, workflowNames []string) ([]map[string]*WorkerResponse, error) {
	responses := make([]map[string]*WorkerResponse, 0, len(workflowNames))

	workflowRunner := NewWorkflowRunner(fr.config, fr.application)

	for _, name := range workflowNames {
		response, err := workflowRunner.RunWithSharedContext(workerSessionContext, name)
		if err != nil {
			return responses, fmt.Errorf("feature runner: error running workflow %s: %w", name, err)
		}
		responses = append(responses, response)

		// Check if there was an error and stop the chain if needed
		for _, resp := range response {
			if resp.IsError() {
				return responses, fmt.Errorf("feature runner: workflow %s returned error", name)
			}
		}
	}

	return responses, nil
}

// GetType returns the type this runner handles
func (fr *FeatureRunner) GetType() string {
	return "feature"
}

// Validate checks if the feature configuration is valid
func (fr *FeatureRunner) Validate() error {
	if fr.config.Name == "" {
		return fmt.Errorf("feature runner: config name cannot be empty")
	}
	return nil
}
