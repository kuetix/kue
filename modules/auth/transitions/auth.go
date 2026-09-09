package transitions

import (
	"encoding/json"
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
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
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
func (a *authTransitions) LogoutCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
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
func (a *authTransitions) RegisterCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
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

	apiHost := shared.ResolveAPIHost(shared.FirstNonEmpty(options["host"].(string), kueConfig.Host))
	if err := shared.PostJSON(apiHost, "/auth/register", payload, nil); err != nil {
		r.Error = fmt.Errorf("register failed: %w", err)
		return
	}

	r.Success = true
	r.Response = "Registration successful.\n"
	return
}

//goland:noinspection GoUnusedParameter
func (a *authTransitions) ProfileGetCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
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
func (a *authTransitions) ProfileUpdateCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
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

//goland:noinspection GoUnusedParameter
func (a *authTransitions) WhoamiCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, kueConfig shared.KueConfig, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if flagBool(options, "help") {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	host := shared.ResolveAPIHost(shared.FirstNonEmpty(flagString(options, "host"), kueConfig.Host))
	cfgPath := shared.ResolveConfigPath(flagString(options, "config"))

	// Not logged in: answer locally, no network round-trip.
	if shared.GetLoginToken(kueConfig) == "" {
		if flagBool(options, "json") {
			r.Success = true
			r.Response = fmt.Sprintf(`{"loggedIn":false,"host":%q,"config":%q}`+"\n", host, cfgPath)
			return
		}
		r.Success = true
		r.StatusCode = http.StatusOK
		r.Response = fmt.Sprintf("Not logged in.\nhost:   %s\nconfig: %s\nRun 'kue login' to authenticate.\n", host, cfgPath)
		return
	}

	respBody, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/profile", nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("whoami failed: %w", err)
		return
	}

	if flagBool(options, "json") {
		r.Success = true
		r.Response = respBody
		return
	}

	username, email, id := profileIdentity(respBody)
	who := shared.FirstNonEmpty(username, email, id, "(unknown)")
	line := "Logged in as " + who
	if email != "" && email != who {
		line += " <" + email + ">"
	}
	out := line + "\n"
	if id != "" {
		out += "id:     " + id + "\n"
	}
	out += fmt.Sprintf("host:   %s\nconfig: %s\n", host, cfgPath)
	r.Success = true
	r.Response = out
	return
}

// profileIdentity pulls a username, email and id out of a GET /profile
// response, tolerating either a bare object or a {"data":{...}} envelope
// and a few common field spellings.
func profileIdentity(body string) (username, email, id string) {
	var raw map[string]interface{}
	if json.Unmarshal([]byte(strings.TrimSpace(body)), &raw) != nil {
		return "", "", ""
	}
	obj := raw
	if data, ok := raw["data"].(map[string]interface{}); ok {
		obj = data
	}
	pick := func(keys ...string) string {
		for _, k := range keys {
			if s, ok := obj[k].(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
		return ""
	}
	return pick("username", "userName", "handle", "nickname", "name"),
		pick("email", "emailAddress"),
		pick("userId", "id", "userID", "uid")
}

func flagBool(options map[string]interface{}, key string) bool {
	v, _ := options[key].(bool)
	return v
}

func flagString(options map[string]interface{}, key string) string {
	v, _ := options[key].(string)
	return v
}
