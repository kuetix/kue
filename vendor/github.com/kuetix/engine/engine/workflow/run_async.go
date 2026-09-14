package workflow

import (
	"fmt"
	"runtime/debug"
	"sync"
	"time"

	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/engine/event"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

func RunBatch(wfConfig domain.WorkflowConfigItem, app domain.Application, wg *sync.WaitGroup, resultChan chan map[string]*WorkerResponse, contexts ...map[string]interface{}) {
	for i := 0; i < wfConfig.Amount; i++ {
		wg.Add(1)
		event.Bus.Publish("on:wsl:run:batch", wfConfig, contexts)
		go runOneAsync(i+1, wg, wfConfig, app, resultChan, contexts...)
	}
}

//goland:noinspection GoUnusedParameter
func handlePanic(id, amount int, name string, response map[string]*WorkerResponse, context *map[string]interface{}) error {
	if r := recover(); r != nil {
		err := fmt.Errorf("%v", r)
		fmt.Printf("[engine:workflow] [%d/%d] %s panic: %v\n", id, amount, name, err)
		logger.Debug(fmt.Sprintf("[engine:workflow] [%d/%d] %s panic: %v", id, amount, name, err))
		logger.Debug(fmt.Sprintf("[engine:workflow] [%d/%d] %s context: %v", id, amount, name, *context))
		fmt.Printf("[engine:workflow] [%d/%d] %s stack trace: %v\n", id, amount, name, string(debug.Stack()))
		return err
	}
	return nil
}

func initializeContext(env *domain.Environment, amount, id int, contexts ...map[string]interface{}) map[string]interface{} {
	var context map[string]interface{}
	if len(contexts) < 1 {
		context = map[string]interface{}{}
	} else {
		context = helpers.MergeMapsLevel0(contexts...)
	}

	context["env"] = env
	context["amount"] = amount
	context["currentId"] = id
	return context
}

func handleRestartPolicy(responses map[string]*WorkerResponse, wfConfig domain.WorkflowConfigItem) int {
	isError := false
	for _, resp := range responses {
		if resp.IsError() {
			isError = true
			break
		}
	}

	if !isError {
		return wfConfig.Retry + 1 // Exit on success
	}

	logger.Infof("[engine:workflow:handleRestartPolicy] **RESTART POLICY**: %s", wfConfig.RestartPolicy)
	switch wfConfig.RestartPolicy {
	case defines.RestartPolicyAlways:
		return 0
	case defines.RestartPolicyStop:
		return wfConfig.Retry + 1
	case defines.RestartPolicyOnFailure:
		return 0 // Restart on failure
	default:
		return wfConfig.Retry
	}
}

func runWithId(wfConfig domain.WorkflowConfigItem, app domain.Application, amount, id int,
	onResponse func(response map[string]*WorkerResponse, context map[string]interface{}), contexts ...map[string]interface{}) map[string]*WorkerResponse {

	var responses map[string]*WorkerResponse
	context := initializeContext(app.Env, amount, id, contexts...)
	name := wfConfig.Name

	defer func() {
		if err := handlePanic(id, amount, name, responses, &context); err != nil {
			if responses == nil {
				responses = map[string]*WorkerResponse{
					name: {
						Error: issues.NewIssues(issues.NewIssue(err.Error(), err, map[string]interface{}{"panic": true})),
					},
				}
			} else {
				for _, r := range responses {
					r.RiseAnIssueFromString(err.Error(), map[string]interface{}{"panic": true})
					break
				}
			}
			if onResponse != nil {
				onResponse(responses, context)
			}
		}
	}()

	event.Bus.Publish("on:wsl:before:run", wfConfig, contexts)
	responses = ExecuteWorkflowRoutine(wfConfig, app, name, &context, []string{}, map[string]interface{}{"debug": app.Env.Config.Application.Debug})

	if onResponse != nil {
		onResponse(responses, context)
	}

	return responses
}

func runOneAsync(id int, wg *sync.WaitGroup, wfConfig domain.WorkflowConfigItem, app domain.Application, resultChan chan map[string]*WorkerResponse, contexts ...map[string]interface{}) {

	defer wg.Done()

	attempts := 1
	name := wfConfig.Name
	amount := wfConfig.Amount

	logger.Debugf("[engine:workflow:runOneAsync] [%d/%d] %s start", id, amount, name)

	for attempts <= wfConfig.Retry {
		responseHandler := func(responses map[string]*WorkerResponse, context map[string]interface{}) {
			event.Bus.Publish("on:wsl:complete", wfConfig, contexts)
			logger.Debugf("[engine:workflow:runOneAsync:onResponse] currentId %d", context["currentId"])

			if responses != nil {
				resultChan <- responses
				logWorkflowResponseAsync(responses, id, amount, name)

				attempts = handleRestartPolicy(responses, wfConfig)
			}
		}
		runWithId(wfConfig, app, amount, id, responseHandler, contexts...)

		attempts++
		if attempts <= wfConfig.Retry {
			time.Sleep(time.Duration(wfConfig.RetryDelay) * time.Second)
		}
	}

	logger.Debug(fmt.Sprintf("[engine:workflow:runOneAsync] [%d/%d] wg.Done", id, amount))
	event.Bus.Publish("on:wsl:exit", wfConfig)
}

func logWorkflowResponseAsync(responses map[string]*WorkerResponse, id, amount int, name string) {
	if responses != nil {
		jsonStr := JsonResponses(responses)
		logger.Debugf("[engine:workflow:runOneAsync] [%d/%d] %s stop", id, amount, name)
		logger.Debugf("[engine:workflow:runOneAsync] [%d/%d] responses:\n%s", id, amount, jsonStr)
	}
}
