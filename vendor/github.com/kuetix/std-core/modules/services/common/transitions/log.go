package transitions

import (
	"encoding/json"
	"fmt"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/logger"
)

// Info-level output is off by default (see logger.LogLevelDefault, which
// only enables Error) - a general-purpose logging action is useless if
// nothing shows up in the console, so this package turns Info on at load
// time rather than leaving every caller to remember to do it themselves.
func init() {
	logger.EnableInfo()
}

type logTransitions struct {
	workflow.BaseServiceTransition
}

func NewLogTransitions() interfaces.ServiceTransitions {
	return &logTransitions{}
}

// Log writes v to the process log at Info level: a single value
// (v: $someAlias), or several at once via an array or object literal (e.g.
// v: {email: $json.email, url: $resetUrl.url}). Meant as a general-purpose
// stand-in for one-off project-local "print this to the console"
// transitions - see kuetix/uuid consumers like web/backend's old
// user/user.LogResetURL, which this replaces.
func (t *logTransitions) Log(v ...any) (r domain.FlowStepResult) {
	var value any = v
	if len(v) == 1 {
		value = v[0]
	}

	if data, err := json.Marshal(value); err == nil {
		logger.Info(string(data))
	} else {
		logger.Info(fmt.Sprintf("%v", value))
	}

	r.Success = true
	r.Response = map[string]interface{}{"logged": value}
	return
}
