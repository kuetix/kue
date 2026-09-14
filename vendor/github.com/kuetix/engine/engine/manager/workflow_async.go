package manager

import (
	"sync"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/services/atomic"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

// WorkflowManager Runner ...
func WorkflowManager(env *domain.Environment) map[string]*workflow.WorkerResponse {
	options := env.Options
	app := domain.NewApplication(env)
	app.EngineName = options.EngineName
	workflowName := options.Workflow
	amount := options.Amount
	args := options.Args
	context := options.Context
	if context == nil {
		context = map[string]interface{}{}
	}
	return executeApplication(app, workflowName, amount, context, args...)
}

func executeApplication(app domain.Application, workflowName string, amount int, context map[string]interface{}, args ...string) map[string]*workflow.WorkerResponse {
	items := app.Env.Config.WorkflowConfig
	var wfConfig = domain.WorkflowConfigItem{
		Name:          workflowName,
		Path:          app.Env.Config.Application.ModulesPath,
		Amount:        amount,
		Retry:         app.Env.Options.Retry,
		RetryDelay:    app.Env.Options.RetryDelay,
		RestartPolicy: app.Env.Options.RestartPolicy,
	}
	for _, item := range items {
		if workflowName == item.Name {
			wfConfig = item
			break
		}
	}

	Quit := atomic.NewBoolChannel()
	context["quit"] = Quit
	if len(args) > 0 {
		arguments := map[string]interface{}{}
		for _, item := range args {
			key, value := helpers.SplitString(item, "=")
			if key == "" {
				continue
			}
			arguments[key] = value
			context[key] = value
		}
		context["args"] = arguments
	}

	logger.Debugf("[engine:workflow] running workflow %s:%s with %d concurrency", wfConfig.Name, wfConfig.Path, wfConfig.Amount)
	wg := &sync.WaitGroup{}
	// Use wfConfig.Amount as the actual number of goroutines that will be spawned
	resultChan := make(chan map[string]*workflow.WorkerResponse, wfConfig.Amount)

	collected := map[string][]*workflow.WorkerResponse{}
	workflow.RunBatch(wfConfig, app, wg, resultChan, context)

	// Close the channel after all goroutines are done in a separate goroutine
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Collect all results until the channel is closed
	for r := range resultChan {
		for name, res := range r {
			collected[name] = append(collected[name], res)
		}
	}

	results := map[string]*workflow.WorkerResponse{}
	for name, responses := range collected {
		results[name] = bestWorkerResponse(responses)
	}
	return results
}

// bestWorkerResponse picks the "best" response from a list of attempts for the
// same workflow name. Successful responses always beat errored ones; ties are
// broken by lowest StatusCode (so 200 wins over 201, 400 wins over 500).
func bestWorkerResponse(responses []*workflow.WorkerResponse) *workflow.WorkerResponse {
	var best *workflow.WorkerResponse
	for _, r := range responses {
		if r == nil {
			continue
		}
		if best == nil {
			best = r
			continue
		}
		if r.IsSuccess() && !best.IsSuccess() {
			best = r
			continue
		}
		if r.IsSuccess() == best.IsSuccess() && r.StatusCode > 0 && (best.StatusCode == 0 || r.StatusCode < best.StatusCode) {
			best = r
		}
	}
	return best
}
