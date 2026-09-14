package transitions

import (
	"context"
	"net/http"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	goredis "github.com/redis/go-redis/v9"
)

type databaseTransitions struct {
	workflow.BaseServiceTransition
	client *goredis.Client
}

func NewDatabaseTransitions() interfaces.ServiceTransitions {
	return &databaseTransitions{client: newClient()}
}

// Ping reports whether the client's redis connection is reachable.
func (t *databaseTransitions) Ping() (r domain.FlowStepResult) {
	if err := t.client.Ping(context.Background()).Err(); err != nil {
		r.Error = err
		r.StatusCode = http.StatusServiceUnavailable
		return
	}

	opts := t.client.Options()
	r.Success = true
	r.StatusCode = http.StatusOK
	r.Response = map[string]interface{}{
		"addr":      opts.Addr,
		"db":        opts.DB,
		"connected": true,
	}
	return
}
