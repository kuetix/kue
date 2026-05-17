package transitions

import (
	"fmt"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
)

type configTransitions struct {
	workflow.BaseServiceTransition
}

func NewConfigTransition() interfaces.ServiceTransitions { return &configTransitions{} }

// Resolve load config and return it as a response for next steps to use
//
//goland:noinspection GoUnusedParameter
func (w *configTransitions) Resolve(options map[string]interface{}) (r domain.FlowStepResult) {
	cfgPath := shared.ResolveConfigPath(strOpt(options, "config"))

	kueConfig, err := shared.LoadKueConfig(cfgPath)
	if err != nil {
		r.Success = false
		r.Error = fmt.Errorf("failed to read config: %w", err)
		return
	}

	apiHost := shared.ResolveAPIHost(strOpt(options, "host"))
	if strings.TrimSpace(strOpt(options, "host")) == "" && strings.TrimSpace(kueConfig.Host) != "" {
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

func strOpt(options map[string]interface{}, key string) string {
	if v, ok := options[key].(string); ok {
		return v
	}
	return ""
}
