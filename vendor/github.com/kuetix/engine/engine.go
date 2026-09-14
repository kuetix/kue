package engine

import (
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/manager"
	"github.com/kuetix/engine/engine/workflow"
)

//goland:noinspection GoUnusedExportedFunction
func RunWorkflow(env string, options *domain.Options) map[string]*workflow.WorkerResponse {
	environment := domain.NewEnvironment(env, options)

	defer environment.ShutdownEnvironment()

	return manager.WorkflowManager(environment)
}
