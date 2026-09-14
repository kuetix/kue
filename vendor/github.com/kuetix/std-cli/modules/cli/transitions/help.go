package transitions

import (
	"flag"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/logger"
	"github.com/kuetix/std-cli/modules/cli/helpers"
)

type helpTransition struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewHelpTransition() interfaces.ServiceTransitions { return &helpTransition{} }

//goland:noinspection GoUnusedParameter
func (h *helpTransition) Help(command string, commands, requestedCommand, commandConfig, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := helpers.GetFlags(flags)
	parts := requestedCommand["parts"].([]string)
	cmd := parts[0]
	parts = parts[1:]
	parts = append(parts, cmd)
	lookingFor := strings.Join(parts, ".")
	var path string
	path = h.Ctx.WorkflowContext.Value("workflowsPath").(string)
	if p, ok := options["workflows"].(string); ok {
		path = p
	}
	if c, ok := commands[lookingFor]; ok {
		if usage, ok := c.(map[string]interface{})["config"].(map[string]interface{})["usage"].(string); ok {
			logger.Info("Command options: ", options, " Command: ", lookingFor)
			helpText = helpers.GetUsage(h.Ctx.Engine.GetApplication(), usage, flagSet, path)
		}
	} else {
		logger.Info("Command options: ", options, " Command: ", command)
		if usage, ok := config["usage"].(string); ok {
			helpText = helpers.GetUsage(h.Ctx.Engine.GetApplication(), usage, flagSet, path)
		}
	}

	r.Success = true
	r.Response = helpText
	return
}
