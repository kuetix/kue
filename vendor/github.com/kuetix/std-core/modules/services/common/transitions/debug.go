package transitions

import (
	"encoding/json"
	"slices"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/logger"
)

type debugTransitions struct {
	workflow.BaseServiceTransition
}

func NewDebugTransitions() interfaces.ServiceTransitions {
	return &debugTransitions{}
}

func (et *debugTransitions) Query() (r domain.FlowStepResult) {
	key, value, err := et.S().GetProperty("pwd|parse")
	et.SetResponse(map[string]interface{}{
		"key":   key,
		"value": value,
		"err":   err,
	})
	r.Success = true
	return
}

func (et *debugTransitions) Log() (r domain.FlowStepResult) {
	et.S()
	logger.Debug("Debug Log")
	context := et.GetContext()
	for k, v := range *context {
		if slices.Contains([]string{"workflow.Flow", "workflow.EngineInterface", "workflow.Worker"}, k) {
			continue
		}
		j, err := json.Marshal(v)
		if err != nil {
			logger.Debug(err)
		} else {
			logger.Debug(k, string(j[:]))
		}
	}
	logger.Debug(et.Ctx.Flow.GetTrace())
	r.Success = true
	return
}
