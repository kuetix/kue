package transitions

import (
	"bytes"
	"flag"
	"fmt"
	"net/http"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type authTransitions struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewAuthTransition() interfaces.ServiceTransitions { return &authTransitions{} }

//goland:noinspection GoUnusedParameter
func (a *authTransitions) LoginCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)
	if options["help"].(bool) {
		helpText = GetUsage(a.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, options["workflows"].(string))
		r.Success = true
		r.Response = helpText
		return
	}

	apiHost := shared.ResolveAPIHost(options["host"].(string))
	user := shared.FirstNonEmpty(options["username"].(string))
	pass := shared.FirstNonEmpty(options["password"].(string))

	if user == "" || pass == "" {
		stdinUser, stdinPass, err := shared.ReadCredentialsFromStdin()
		if err != nil {
			r.Error = fmt.Errorf("failed to read credentials from stdin: %w", err)
			return
		}
		if user == "" {
			user = stdinUser
		}
		if pass == "" {
			pass = stdinPass
		}
	}

	if pass == "" {
		keyboardPass, err := shared.ReadPasswordFromKeyboard()
		if err != nil {
			r.Error = fmt.Errorf("failed to read password from keyboard: %w", err)
			return
		}
		pass = keyboardPass
	}

	if user == "" || pass == "" {
		r.Error = fmt.Errorf("username and password are required. Provide via --username/--password, stdin, or interactive prompt")
		return
	}

	respData, err := shared.PostLogin(apiHost, user, pass)
	if err != nil {
		r.Error = fmt.Errorf("login failed: %w", err)
		return
	}

	kueConfig.Login = respData
	kueConfig.Host = apiHost
	kueConfig.Secure = shared.IsHostUseSecure(kueConfig.Host)
	cfgPath := shared.ResolveConfigPath(options["config"].(string))
	if err := shared.SaveKueConfig(cfgPath, kueConfig); err != nil {
		r.Error = fmt.Errorf("failed to save config: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Logged in successfully. Config updated: %s\n", cfgPath)
	return
}

//goland:noinspection GoUnusedParameter
func (a *authTransitions) LogoutCommand(command string, cfg map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText = buf.String() + "\n" + helpText
		r.Success = true
		r.Response = helpText
		return
	}

	kueConfig.Login = nil
	cfgPath := shared.ResolveConfigPath(options["config"].(string))
	if err := shared.SaveKueConfig(cfgPath, kueConfig); err != nil {
		r.Error = fmt.Errorf("failed to save config: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Logged out. Removed login data from: %s\n", cfgPath)
	return
}

//goland:noinspection GoUnusedParameter
func (a *authTransitions) RegisterCommand(command string, cfg map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText = buf.String() + "\n" + helpText
		r.Success = true
		r.Response = helpText
		return
	}

	email := options["email"].(string)
	password := options["password"].(string)
	if strings.TrimSpace(email) == "" || strings.TrimSpace(password) == "" {
		r.Error = fmt.Errorf("email and password are required")
		return
	}

	payload := map[string]string{
		"email":    strings.TrimSpace(email),
		"password": strings.TrimSpace(password),
	}
	if name := options["name"].(string); strings.TrimSpace(name) != "" {
		payload["name"] = strings.TrimSpace(name)
	}
	if username := options["username"].(string); strings.TrimSpace(username) != "" {
		payload["username"] = strings.TrimSpace(username)
	}

	if err := shared.PostJSON(kueConfig.Host, "/auth/register", payload, nil); err != nil {
		r.Error = fmt.Errorf("register failed: %w", err)
		return
	}

	r.Success = true
	r.Response = "Registration successful.\n"
	return
}

//goland:noinspection GoUnusedParameter
func (a *authTransitions) ProfileGetCommand(command string, cfg map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText = buf.String() + "\n" + helpText
		r.Success = true
		r.Response = helpText
		return
	}

	respBody, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/profile", nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("profile get failed: %w", err)
		return
	}

	r.Success = true
	r.Response = respBody
	return
}

//goland:noinspection GoUnusedParameter
func (a *authTransitions) ProfileUpdateCommand(command string, cfg map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText = buf.String() + "\n" + helpText
		r.Success = true
		r.Response = helpText
		return
	}

	name := options["name"].(string)
	username := options["username"].(string)
	if strings.TrimSpace(name) == "" && strings.TrimSpace(username) == "" {
		r.Error = fmt.Errorf("at least one field is required: --name or --username")
		return
	}

	payload := map[string]string{}
	if strings.TrimSpace(name) != "" {
		payload["name"] = strings.TrimSpace(name)
	}
	if strings.TrimSpace(username) != "" {
		payload["username"] = strings.TrimSpace(username)
	}

	respBody, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodPut, "/profile/update", payload)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("profile update failed: %w", err)
		return
	}

	r.Success = true
	r.Response = respBody
	return
}
