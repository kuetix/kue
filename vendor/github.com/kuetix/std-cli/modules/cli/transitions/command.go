package transitions

import (
	"flag"
	"fmt"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/logger"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type commandTransition struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewCommandTransition() interfaces.ServiceTransitions { return &commandTransition{} }

//goland:noinspection GoUnusedParameter
func (h *commandTransition) Echo(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)
	logger.Info("Command options: ", options, " Command: ", command)
	if options["help"].(bool) {
		helpText = GetUsage(h.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, h.Ctx.WorkflowContext.Value("workflowsPath").(string))
	} else {
		if echo, ok := options["echo"].(string); ok {
			helpText += fmt.Sprintf("%s\n", echo)
		}
	}
	r.Success = true
	r.Response = helpText
	return
}
