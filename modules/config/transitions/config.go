package transitions

import (
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	"github.com/kuetix/logger"
)

type configTransitions struct {
	workflow.BaseServiceTransition
}

func NewConfigTransition() interfaces.ServiceTransitions { return &configTransitions{} }

// Resolve loads config and returns it as a response for next steps to use.
//
// The parameter is intentionally NOT called "options": that name is reserved
// by the engine's WSL evaluator (mergeActionArgsIntoTransition nests any
// action argument named "options" under an extra "options" wrapper to avoid
// clobbering transition schema fields), so an action call written as
// `config/config.Resolve(options: $config.flags)` never actually delivered
// $config.flags here — it arrived double-wrapped as {"options": {...}}, no
// matter the expression. Every workflows/cli/*/*.wsl "Config" state instead
// calls this as `config/config.Resolve(flags: $config.flags)`; flags is
// GetFlags' per-command getter-closure map, the same shape "Command" states
// already pass to their own transitions as `flags: $config.flags`. strOpt
// below resolves either that closure shape or a plain string — the latter
// kept working for direct Go-level callers (tests) that build the map by
// hand.
//
//goland:noinspection GoUnusedParameter
func (w *configTransitions) Resolve(flags map[string]interface{}) (r domain.FlowStepResult) {
	cfgPath := shared.ResolveConfigPath(strOpt(flags, "config"))

	kueConfig, err := shared.LoadKueConfig(cfgPath)
	if err != nil {
		r.Success = false
		r.Error = fmt.Errorf("failed to read config: %w", err)
		return
	}

	apiHost := shared.ResolveAPIHost(strOpt(flags, "host"))
	if strings.TrimSpace(strOpt(flags, "host")) == "" && strings.TrimSpace(kueConfig.Host) != "" {
		apiHost = kueConfig.Host
	}

	token, ok := kueConfig.Login["token"]
	if !ok {
		data, ok := kueConfig.Login["data"]
		if !ok {
			token = ""
		} else {
			token, ok = data.(map[string]interface{})["token"].(string)
			if !ok {
				token = ""
			}
		}
	}

	kueConfig.Host = apiHost

	// Visible under --debug/-vv: which config file and host this invocation
	// actually resolved to, and whether it found a login token there — the
	// single most useful line when a command seems to have hit the wrong
	// server or identity (KUE_CONFIG_PATH/KUE_HOST pointing somewhere
	// unexpected, --host not overriding a stored config, no login, ...).
	loggedIn := kueConfig.Login != nil && len(kueConfig.Login) > 0
	logger.Debugf("[config.Resolve] config=%s host=%s loggedIn=%t", cfgPath, apiHost, loggedIn)

	login := kueConfig.Login
	if login != nil || len(login) > 0 {
		if data, ok := login["data"].(map[string]interface{}); ok {
			login = data
		}
	}
	r.Success = true
	r.Response = map[string]interface{}{
		"host":      apiHost,
		"kueConfig": kueConfig,
		"login":     login,
		"token":     token,
	}
	return
}

// strOpt reads flags[key], accepting either a plain string (direct Go calls,
// e.g. tests) or one of GetFlags' getter closures — a func() *string, the
// shape a real CLI invocation's $config.flags carries.
func strOpt(flags map[string]interface{}, key string) string {
	switch v := flags[key].(type) {
	case string:
		return v
	case func() *string:
		if s := v(); s != nil {
			return *s
		}
	}
	return ""
}
