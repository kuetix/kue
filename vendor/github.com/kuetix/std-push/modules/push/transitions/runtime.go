package transitions

// push/runtime - the two blocking loops that make push a live pipeline
// stage, in the same shape as zmist/protocol.RunBroadcast and
// zmist/janitor.RunLoop (a Go loop dispatching a declarative WSL workflow
// per unit of work - the engine has no runtime looping construct).
//
//   RunMissedConsumer  reads the cue:missed stream, runs workflows/push/decide
//                      per entry (decision -> suppress / defer-bulk / send-now)
//   RunScheduler       runs workflows/push/flush every PUSH_SCHEDULE_INTERVAL
//                      to drain coalesced bulk buffers
//
// Both expect the consuming binary's cwd to contain workflows/push/*.wsl
// (zmist vendors decide.wsl / flush.wsl into backend/workflows/push/).

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"time"

	"github.com/kuetix/engine"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
)

const (
	decideWorkflow = "workflows/push/decide"
	flushWorkflow  = "workflows/push/flush"

	missedGroup             = "push-missed"
	defaultScheduleInterval = 30 * time.Second
)

type runtimeTransitions struct {
	workflow.BaseServiceTransition
}

// NewRuntimeTransitions is the DI constructor for the `push/runtime` class.
func NewRuntimeTransitions() interfaces.ServiceTransitions {
	return &runtimeTransitions{}
}

func scheduleInterval() time.Duration {
	return envDuration("PUSH_SCHEDULE_INTERVAL", defaultScheduleInterval)
}

// RunMissedConsumer blocks forever, running decideWorkflow once per
// cue:missed entry under consumer.
func (t *runtimeTransitions) RunMissedConsumer(consumer string) (r domain.FlowStepResult) {
	if consumer == "" {
		consumer = "push-missed-1"
	}
	log.Printf("[push:runtime] missed consumer started (stream=%s group=%s consumer=%s)", missedStream, missedGroup, consumer)

	err := consumeGroup(context.Background(), missedStream, missedGroup, consumer, func(id string, fields map[string]string) error {
		responses := engine.RunWorkflow("production", &domain.Options{
			EngineName:    "push-missed",
			ConfigName:    "engine",
			Amount:        1,
			Retry:         1,
			RestartPolicy: "stop",
			Workflow:      decideWorkflow,
			LogPath:       "stdout",
			Config:        &domain.Config{},
			Context: map[string]interface{}{
				"missedChannelId": fields["channelId"],
				"missedMessageId": fields["messageId"],
				"missedType":      fields["type"],
				"missedSenderId":  fields["senderId"],
				"missedDelivered": fields["deliveredUserIds"],
				"missedForced":    fields["forcedUserIds"],
				"missedGroupNo":   fields["groupNo"],
			},
		})
		if result := lookupWorkflowResponse(responses, decideWorkflow); result != nil && result.Error != nil {
			log.Printf("[push:runtime] decide error for entry %s: %v", id, result.GetError())
		}
		return nil
	})
	if err != nil {
		return failResult(http.StatusInternalServerError, err)
	}
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = "push missed consumer stopped"
	return
}

// RunScheduler blocks forever, running flushWorkflow now and then every
// PUSH_SCHEDULE_INTERVAL.
func (t *runtimeTransitions) RunScheduler() (r domain.FlowStepResult) {
	interval := scheduleInterval()
	log.Printf("[push:runtime] scheduler started (interval=%s workflow=%s)", interval, flushWorkflow)

	runFlush()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		runFlush()
	}

	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = "push scheduler stopped"
	return
}

func runFlush() {
	responses := engine.RunWorkflow("production", &domain.Options{
		EngineName:    "push-scheduler",
		ConfigName:    "engine",
		Amount:        1,
		Retry:         1,
		RestartPolicy: "stop",
		Workflow:      flushWorkflow,
		LogPath:       "stdout",
		Config:        &domain.Config{},
	})
	if result := lookupWorkflowResponse(responses, flushWorkflow); result != nil && result.Error != nil {
		log.Printf("[push:runtime] flush error: %v", result.GetError())
	}
}

func lookupWorkflowResponse(responses map[string]*workflow.WorkerResponse, workflowPath string) *workflow.WorkerResponse {
	if response, ok := responses[workflowPath]; ok {
		return response
	}
	if response, ok := responses[filepath.Base(workflowPath)]; ok {
		return response
	}
	for _, response := range responses {
		return response
	}
	return nil
}
