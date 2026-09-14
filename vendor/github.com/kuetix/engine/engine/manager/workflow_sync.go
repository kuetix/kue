package manager

import (
	"fmt"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/services/atomic"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/engine/event"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
)

// WorkflowManagerOnce Runner ...
//
//goland:noinspection GoUnusedExportedFunction
func WorkflowManagerOnce(env *domain.Environment, options *domain.Options) map[string]*workflow.WorkerResponse {
	app := domain.NewApplication(env)
	app.EngineName = options.EngineName
	workflowName := options.Workflow
	args := options.Args
	context := options.Context
	if context == nil {
		context = map[string]interface{}{}
	}
	return executeOnceApplication(app, workflowName, context, args...)
}

func executeOnceApplication(app domain.Application, workflowName string, context map[string]interface{}, args ...string) map[string]*workflow.WorkerResponse {
	items := app.Env.Config.WorkflowConfig
	var wfConfig = domain.WorkflowConfigItem{
		Name:          workflowName,
		Path:          app.Env.Config.Application.WorkflowsPath,
		Amount:        1,
		Retry:         3,
		RetryDelay:    0,
		RestartPolicy: "stop",
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
		for _, item := range args {
			key, value := helpers.SplitString(item, "=")
			if key == "" {
				continue
			}
			context[key] = value
		}
	}

	logger.Debug(fmt.Sprintf("[engine:workflow] running workflow %s:%s with %d concurrency", wfConfig.Name, wfConfig.Path, wfConfig.Amount))
	return RunOne(wfConfig, app, context)
}

func RunOne(wfConfig domain.WorkflowConfigItem, app domain.Application, contexts ...map[string]interface{}) map[string]*workflow.WorkerResponse {
	event.Bus.Publish("on:wsl:run:batch", wfConfig, contexts)
	return workflow.RunOne(wfConfig, app, contexts...)
}
