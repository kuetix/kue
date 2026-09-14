package workflow

import (
	"context"
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
)

// ParseFlowPath parses a flow path in the format "namespace/path/wsl_name.flow_name"
// Returns the WSL file path and the specific flow name
func ParseFlowPath(path string) (wslPath string, flowName string) {
	// Check if path contains a dot indicating a specific flow name
	if idx := strings.LastIndex(path, "."); idx > 0 {
		wslPath = path[:idx]
		flowName = path[idx+1:]
		return wslPath, flowName
	}

	// No specific flow name, return the whole path
	return path, ""
}

// ExecuteSpecificFlow loads a WSL file and executes only the specified flow
func ExecuteSpecificFlow(wfConfig domain.WorkflowConfigItem, app domain.Application, flowPath string, flowType string, workflowContext *map[string]interface{}, transitions []string, options ...map[string]interface{}) (responses map[string]*WorkerResponse, err error) {
	wslPath, flowName := ParseFlowPath(flowPath)

	// If no specific flow name, use the original behavior
	if flowName == "" {
		responses, _ = ExecuteWorkflow(wfConfig, app, flowPath, workflowContext, transitions, options...)
		return responses, nil
	}

	// Load the WSL file
	handler := NewHandleWorkflow(wfConfig, app, wslPath, transitions, options...)
	rootCtx := context.Background()
	ctx, cancel := context.WithCancel(rootCtx)
	defer cancel()

	engine := handler.GetEngine()
	worker := handler.GetWorker()

	// Load the workflows from the WSL file
	if !(*engine).LoadWorkflow(ctx, wslPath, *worker) {
		response := (*worker).GetWorkerResponse()
		return map[string]*WorkerResponse{flowName: response}, fmt.Errorf("failed to load WSL file: %s", wslPath)
	}

	// Find the specific flow by name and type
	allFlows := (*engine).GetFlow()
	if allFlows == nil {
		return nil, fmt.Errorf("no flows loaded from: %s", wslPath)
	}

	// Get all flows from the engine
	engineImpl := (*engine).(*Engine)
	targetFlow, found := engineImpl.Flows[flowName]

	if !found {
		return nil, fmt.Errorf("flow %s not found in WSL file: %s", flowName, wslPath)
	}

	// Verify the flow type matches if specified
	if flowType != "" && targetFlow.Type != flowType && targetFlow.Type != "" {
		return nil, fmt.Errorf("flow %s in %s is type '%s' but expected '%s'", flowName, wslPath, targetFlow.Type, flowType)
	}

	// Execute only the specific flow
	engineImpl.Flow = targetFlow
	responses = map[string]*WorkerResponse{}

	if (*engine).Start() {
		(*engine).Run()
		response := (*engine).Done()
		responses[flowName] = &WorkerResponse{
			Error:      response.Error,
			StatusCode: response.StatusCode,
			Response:   response.Response,
		}
	} else {
		response := (*worker).GetWorkerResponse()
		responses[flowName] = response
	}

	return responses, nil
}
