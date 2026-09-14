package workflow

import (
	"github.com/kuetix/engine/engine/domain/interfaces"
)

type ServiceTransitionMapping struct {
	ServiceName string                        `json:"service_name"`
	Name        string                        `json:"name"`
	Impl        interfaces.ServiceTransitions `json:"-"`
}

func (stm *ServiceTransitionMapping) SetWorkerSessionContext(session *WorkerSessionContext) {
	if setter, ok := stm.Impl.(interface{ SetSession(*WorkerSessionContext) }); ok {
		setter.SetSession(session)
	}
}
