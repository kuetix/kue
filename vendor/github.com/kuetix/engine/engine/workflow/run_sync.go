package workflow

import (
	"errors"
	"fmt"
	"runtime/debug"
	"time"

	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/issues"
	"github.com/kuetix/engine/event"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

func RunOne(wfConfig domain.WorkflowConfigItem, app domain.Application, contexts ...map[string]interface{}) map[string]*WorkerResponse {
	event.Bus.Publish("on:wsl:run:batch", wfConfig, contexts)
	return runOneSync(wfConfig, app, contexts...)
}

func run(wfConfig domain.WorkflowConfigItem, app domain.Application, onResponse func(responses map[string]*WorkerResponse, context map[string]interface{}), contexts ...map[string]interface{}) map[string]*WorkerResponse {
	var responses map[string]*WorkerResponse
	var context map[string]interface{}
	name := wfConfig.Name
	defer func() {
		var err error = nil
		if r := recover(); r != nil {
			err = errors.New(fmt.Sprintf("%v", r))
			fmt.Println(fmt.Sprintf("[engine:workflow] %s panic: %v", name, err))
			logger.Debug(fmt.Sprintf("[engine:workflow] %s panic: %v", name, err))
			logger.Debug(fmt.Sprintf("[engine:workflow] %s context: %v", name, context))
			fmt.Println(fmt.Sprintf("[engine:workflow] %s stack trace: %v", name, string(debug.Stack())))
		}
		if err != nil {
			if responses == nil {
				responses = map[string]*WorkerResponse{
					name: {
						Error: issues.NewIssues(issues.NewIssue(err.Error(), err, map[string]interface{}{"panic": true})),
					},
				}
			} else {
				for _, response := range responses {
					response.RiseAnIssueFromString(err.Error(), map[string]interface{}{"panic": true})
					break
				}
			}
		}
		if onResponse != nil {
			onResponse(responses, context)
		}
	}()

	if len(contexts) < 1 {
		context = map[string]interface{}{}
	} else {
		context = helpers.MergeMapsLevel0(contexts...)
	}
	context["env"] = app.Env
	event.Bus.Publish("on:wsl:before:run", wfConfig, contexts)
	responses = ExecuteWorkflowRoutine(wfConfig, app, name, &context, []string{}, map[string]interface{}{"debug": false})

	return responses
}

func runOneSync(wfConfig domain.WorkflowConfigItem, app domain.Application, contexts ...map[string]interface{}) map[string]*WorkerResponse {
	workflowResponses := make(map[string]*WorkerResponse, 0)
	attemptCount := 1

	logger.Debugf("[engine:workflow:runOneAsync] %s start", wfConfig.Name)

	for attemptCount <= wfConfig.Retry {
		run(wfConfig, app, func(responses map[string]*WorkerResponse, context map[string]interface{}) {
			handleWorkflowResponse(responses, context, wfConfig, &attemptCount, workflowResponses, contexts)
		}, contexts...)

		if attemptCount <= wfConfig.Retry {
			time.Sleep(time.Duration(wfConfig.RetryDelay) * time.Second)
		}
		attemptCount++
	}

	logger.Debug(fmt.Sprintf("[engine:workflow:runOneAsync] [%d/%d] wg.Done", 1, wfConfig.Amount))
	event.Bus.Publish("on:wsl:exit", wfConfig)

	return workflowResponses
}

func handleWorkflowResponse(
	inResponses map[string]*WorkerResponse,
	context map[string]interface{},
	wfConfig domain.WorkflowConfigItem,
	attemptCount *int,
	responses map[string]*WorkerResponse,
	contexts []map[string]interface{},
) {
	if inResponses == nil {
		return
	}

	event.Bus.Publish("on:wsl:complete", wfConfig, contexts)
	logger.Debugf("[engine:workflow:runOneAsync:onResponse] currentId %d", context["currentId"])

	isError := false
	for name, inResponse := range inResponses {
		if inResponse.IsError() {
			isError = true
		}
		responses[name] = inResponse
	}
	logWorkflowResponseOneSync(inResponses, wfConfig.Name)

	switch {
	case isError:
		*attemptCount = wfConfig.Retry + 1 // Exit on success
	case wfConfig.RestartPolicy == defines.RestartPolicyAlways:
		*attemptCount = 0
	case wfConfig.RestartPolicy == defines.RestartPolicyOnFailure:
		*attemptCount = 0
	case wfConfig.RestartPolicy == defines.RestartPolicyStop:
		*attemptCount = wfConfig.Retry + 1
	}
}

func logWorkflowResponseOneSync(responses map[string]*WorkerResponse, workflowName string) {
	jsonResponse := JsonResponses(responses)
	logger.Debugf("[engine:workflow:runOneAsync] %s stop", workflowName)
	logger.Debug(jsonResponse)
	logger.Debug("[engine:workflow:runOneAsync] response:")
	logger.Debug(jsonResponse)
}
