package transitions

import (
	"fmt"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

type speakTransitions struct {
	workflow.BaseServiceTransition
}

func NewSpeakTransitions() interfaces.ServiceTransitions { return &speakTransitions{} }

// Say returns a classic greeting. It does not depend on any input.
func (t *speakTransitions) Say(on string, v ...any) (r domain.FlowStepResult) {
	var value interface{} = v
	if len(v) == 1 {
		value = v[0]
	}
	msg := map[string]interface{}{on: value}
	// logger.Info("["+on+"] ", msg[on])

	fmt.Printf("[%s] %v\n", on, value)

	r.Success = true
	r.Response = msg
	return
}
