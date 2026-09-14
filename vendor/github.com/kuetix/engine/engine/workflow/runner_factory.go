package workflow

import (
	"context"
	"fmt"

	"github.com/kuetix/engine/engine/domain"
)

// Runner is the interface that all type-specific runners must implement
type Runner interface {
	Run(ctx context.Context, name string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) map[string]*WorkerResponse
	RunWithSharedContext(workerSessionContext *WorkerSessionContext, name string) (map[string]*WorkerResponse, error)
	GetType() string
	Validate() error
}

// RunnerFactory creates the appropriate runner based on flow type
type RunnerFactory struct {
	config      domain.WorkflowConfigItem
	application domain.Application
}

// NewRunnerFactory creates a new runner factory
func NewRunnerFactory(config domain.WorkflowConfigItem, app domain.Application) *RunnerFactory {
	return &RunnerFactory{
		config:      config,
		application: app,
	}
}

// CreateRunner creates a runner based on the flow type
func (rf *RunnerFactory) CreateRunner(flowType string) (Runner, error) {
	switch flowType {
	case "workflow", "":
		return NewWorkflowRunner(rf.config, rf.application), nil
	case "feature":
		return NewFeatureRunner(rf.config, rf.application), nil
	case "solution":
		return NewSolutionRunner(rf.config, rf.application), nil
	default:
		// For custom types, default to workflow runner
		return NewWorkflowRunner(rf.config, rf.application), nil
	}
}

// GetRunnerForFlow gets the appropriate runner for a flow
func (rf *RunnerFactory) GetRunnerForFlow(flow *domain.Flow) (Runner, error) {
	return rf.CreateRunner(flow.Type)
}

// RunnerRegistry maintains a registry of custom runners
type RunnerRegistry struct {
	customRunners map[string]func(domain.WorkflowConfigItem, domain.Application) Runner
}

var globalRunnerRegistry = &RunnerRegistry{
	customRunners: make(map[string]func(domain.WorkflowConfigItem, domain.Application) Runner),
}

// RegisterCustomRunner registers a custom runner for a specific type
func RegisterCustomRunner(typeName string, factory func(domain.WorkflowConfigItem, domain.Application) Runner) {
	globalRunnerRegistry.customRunners[typeName] = factory
}

// GetCustomRunner retrieves a custom runner if registered
func (rr *RunnerRegistry) GetCustomRunner(typeName string, config domain.WorkflowConfigItem, app domain.Application) (Runner, bool) {
	if factory, ok := rr.customRunners[typeName]; ok {
		return factory(config, app), true
	}
	return nil, false
}

// CreateRunnerWithRegistry creates a runner, checking registry first
func (rf *RunnerFactory) CreateRunnerWithRegistry(flowType string) (Runner, error) {
	// Check if there's a custom runner registered
	if runner, ok := globalRunnerRegistry.GetCustomRunner(flowType, rf.config, rf.application); ok {
		return runner, nil
	}

	// Fall back to built-in runners
	return rf.CreateRunner(flowType)
}

// ExecuteWithRunner executes a flow using the appropriate runner
//
//goland:noinspection GoUnusedExportedFunction
func ExecuteWithRunner(wfConfig domain.WorkflowConfigItem, app domain.Application, flowType string, name string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) (map[string]*WorkerResponse, error) {
	factory := NewRunnerFactory(wfConfig, app)
	runner, err := factory.CreateRunnerWithRegistry(flowType)
	if err != nil {
		return nil, fmt.Errorf("failed to create runner for type %s: %w", flowType, err)
	}

	if err := runner.Validate(); err != nil {
		return nil, fmt.Errorf("runner validation failed: %w", err)
	}

	ctx := context.Background()
	responses := runner.Run(ctx, name, workflowContext, transitions, options...)

	return responses, nil
}

// ExecuteWithRunnerAndSharedContext executes a flow with a shared WorkerSessionContext
func ExecuteWithRunnerAndSharedContext(workerSessionContext *WorkerSessionContext, flowType string, name string) (map[string]*WorkerResponse, error) {
	engine := workerSessionContext.Engine
	app := engine.GetApplication()
	wfConfig := engine.GetWorkflowConfig()

	factory := NewRunnerFactory(wfConfig, app)
	runner, err := factory.CreateRunnerWithRegistry(flowType)
	if err != nil {
		return nil, fmt.Errorf("failed to create runner for type %s: %w", flowType, err)
	}

	if err := runner.Validate(); err != nil {
		return nil, fmt.Errorf("runner validation failed: %w", err)
	}

	return runner.RunWithSharedContext(workerSessionContext, name)
}
