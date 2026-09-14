package transitions

import (
	"bytes"
	"flag"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/kuetix/engine"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/logger"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type cliTransitions struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	buf           bytes.Buffer
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewCliTransition() interfaces.ServiceTransitions { return &cliTransitions{} }

//goland:noinspection GoUnusedParameter
func (c *cliTransitions) InitCommand(command string, flags map[string]interface{}) (r domain.FlowStepResult) {
	context := c.Ctx.WorkflowContext
	parent := context.Value("parent").(*cliTransitions)
	var ok bool
	if len(flags) > 0 {
		opts := GetFlags(flags)
		if len(opts) > 0 {
			var v string
			if v, ok = opts["modules"].(string); ok {
				parent.modulesPath = v
			}
			if v, ok = opts["workflows"].(string); ok {
				parent.workflowsPath = v
			}
		}
	}

	r.Success = true
	return
}

func (c *cliTransitions) RegisterCommands(
	modulesPath string,
	workflowsPath string,
	version string,
	buildTime string,
	defaultCommand string,
	groups map[string]interface{},
) (r domain.FlowStepResult) {
	c.modulesPath = modulesPath
	c.workflowsPath = workflowsPath
	c.version = version
	c.buildTime = buildTime

	app := c.Ctx.Engine.GetApplication()

	// Every flag's schema (including which ones are boolean) is already
	// fully known statically in groups (commands.wsl's const), regardless of
	// which command ends up requested — so collect it before GetArgs splits
	// os.Args, letting it tell a bare boolean flag from one that still needs
	// a following value token.
	mainCommand, requestedCommand, args, options := GetArgs(collectBoolFlagNames(groups))
	if requestedCommand == "" {
		requestedCommand = defaultCommand
	}

	parts := strings.Split(requestedCommand, ".")
	// requestedCommandCandidates lists every flagSet key this invocation could
	// resolve to, in priority order: the exact dotted command GetArgs()
	// produced, then "<group>.*", "<group>.*.*", ... wildcard fallbacks
	// (registered via "*": {"$extends": ""} entries in commands.wsl).
	// GetArgs() always treats a second bare token as a subcommand keyword, so
	// a command whose only positional argument is a plain value rather than a
	// real subcommand (e.g. `install <name>`, `run <target>`) only ever
	// matches one of the wildcard forms below, never the exact dotted
	// command. The registration gate further down must accept those
	// candidates too — matching only the literal requestedCommand silently
	// skipped flag registration (and fs.Parse) for such commands, leaving
	// every option after the name (--output, --owner, --host, --check, ...)
	// unparsed and stuck at its JSON-declared default.
	requestedCommandCandidates := []string{requestedCommand}
	candidateBase := parts[0]
	for i := 1; i < len(parts); i++ {
		requestedCommandCandidates = append(requestedCommandCandidates, candidateBase+strings.Repeat(".*", i))
	}
	candidateCmds := make(map[string]bool, len(requestedCommandCandidates))
	for _, v := range requestedCommandCandidates {
		candidateCmds[v] = true
	}

	c.fs = make(map[string]*flag.FlagSet)
	c.commands = make(map[string]interface{})
	var each []interface{}
	global := make(map[string]interface{})
	eachNames := make(map[string]string)
	if gs, ok := groups["*"].([]interface{}); ok {
		for _, g := range gs {
			if e, ok := g.(map[string]interface{}); ok {
				if l, ok := e["*"].(map[string]interface{})["options"].([]interface{}); ok {
					each = append(each, l...)
				}
				if workflowInit, ok := e["*"].(map[string]interface{})["init"].(string); ok {
					initFs := flag.NewFlagSet(workflowInit, flag.ContinueOnError)
					flags := c.getOptions(each, initFs)
					app.Env.Options.Context["parent"] = c
					commandBase := filepath.Base(workflowInit)
					app.Env.Options.Context["command"] = commandBase
					app.Env.Options.Context["flags"] = flags
					largs := slices.Clone(args)
					appOpts := c.Ctx.Engine.GetApplication().Env.Options
					config := map[string]interface{}{
						"flags": flags,
					}
					_, responses := c.runWorkflow(commandBase, workflowInit, appOpts, config, largs, false, false, false)
					for _, response := range responses {
						if response.Error != nil {
							logger.Errorf("Error initializing command: %s", response.Error.Error())
						}
					}
				}
			}
		}

		if len(each) > 0 {
			for _, v := range each {
				if o, ok := v.(map[string]interface{}); ok {
					// {"long": "modules", "short": "mp", "value": "modules", "usage": "Path to modules directory"},
					eachNames[o["long"].(string)] = o["short"].(string)
				}
			}
		}
	}
	delete(groups, "*")
	for command, commandsList := range groups {
		if commandsConfigs, ok := commandsList.([]interface{}); ok {
			for _, commands := range commandsConfigs {
				if commandsMap, ok := commands.(map[string]interface{}); ok {
					flags := make(map[string]interface{})
					for subCommand, commandConfig := range commandsMap {
						var cmd string
						command = strings.TrimSpace(command)
						subCommand = strings.TrimSpace(subCommand)
						if subCommand != "" {
							cmd = command + "." + subCommand
						} else {
							cmd = command
						}
						c.fs[cmd] = flag.NewFlagSet(command, flag.ContinueOnError)
						c.fs[cmd].SetOutput(&c.buf)
						commandConfig = c.resolveCommandConfig(commandConfig.(map[string]interface{}), commandsMap)
						// Expose the command's FlagSet on the inner config too —
						// workflows pass `config: $config.config` to their
						// transitions, and several of those render `--help` via
						// config["flagSet"]. Without this it is nil and the
						// transition panics on the type assertion.
						commandConfig.(map[string]interface{})["flagSet"] = c.fs[cmd]
						c.commands[cmd] = map[string]interface{}{
							"workflow": commandConfig.(map[string]interface{})["workflow"],
							"config":   commandConfig,
							"flags":    flags,
							"flagSet":  c.fs[cmd],
						}
						if requestedCommand != defaultCommand {
							if !candidateCmds[cmd] {
								continue
							}
						}
						if optionsSlice, ok := commandConfig.(map[string]interface{})["options"].([]interface{}); ok {
							if len(each) > 0 {
								for _, v := range each {
									optionsSlice = append(optionsSlice, v)
								}
							}
							flags = c.getOptions(optionsSlice, c.fs[cmd])
							c.commands[cmd] = map[string]interface{}{
								"main_command": mainCommand,
								"workflow":     commandConfig.(map[string]interface{})["workflow"],
								"config":       commandConfig,
								"flags":        flags,
								"flagSet":      c.fs[cmd],
							}
							_ = c.fs[cmd].Parse(options)
							// Expose positional args on the inner command
							// config — transitions read config["args"].
							commandConfig.(map[string]interface{})["args"] = args
							for l := range eachNames {
								if _, ok := flags[l]; !ok {
									continue
								}
								option := flags[l]
								global[l] = GetFlag(option)
							}
						}
					}
				}
			}
		}
	}

	app.Env.Options.Context["commands"] = c.commands
	app.Env.Options.Context["requestedCommand"] = map[string]interface{}{
		"main_command": mainCommand,
		"command":      requestedCommand,
		"parts":        parts,
		"args":         args,
		"options":      options,
		"global":       global,
	}

	var ok bool
	for _, v := range requestedCommandCandidates {
		_, ok = c.fs[v]
		if ok {
			requestedCommand = v
			ok = true
			break
		}
	}

	if !ok {
		r.Success = false
		r.Error = fmt.Errorf("Command %s not found", requestedCommand)
		r.Response = map[string]interface{}{
			"commands":         c.commands,
			"workflow":         "",
			"requestedCommand": requestedCommand,
			"config":           nil,
			"args":             args,
			"options":          options,
			"global":           global,
		}
		return
	}

	r.Success = true
	c.commands[requestedCommand].(map[string]interface{})["flagSet"] = c.fs[requestedCommand]
	// Transitions read positional args from their command config
	// (config["args"]), so expose them there as well.
	c.commands[requestedCommand].(map[string]interface{})["args"] = args
	commandWorkflow := c.commands[requestedCommand].(map[string]interface{})["workflow"]
	r.Response = map[string]interface{}{
		"commands": c.commands,
		"workflow": commandWorkflow,
		"command":  requestedCommand,
		"config":   c.commands[requestedCommand],
		"args":     args,
		"options":  options,
		"global":   global,
	}

	return
}

// collectBoolFlagNames walks the whole commands.wsl-derived tree — the
// global "*" block plus every group/subcommand — and returns the long and
// short spellings (dashes-stripped) of every flag declared "type": "bool".
// It reads groups only; RegisterCommands still does its own (destructive)
// walk afterward. An entry that only carries "$extends" has no "options" of
// its own to contribute here, but the sibling it extends does, and that
// sibling is walked too — so its bool flags are still collected.
func collectBoolFlagNames(groups map[string]interface{}) map[string]bool {
	names := map[string]bool{}
	addOptionsSlice := func(optionsSlice []interface{}) {
		for _, opts := range optionsSlice {
			o, ok := opts.(map[string]interface{})
			if !ok {
				continue
			}
			if t, _ := o["type"].(string); t != "bool" {
				continue
			}
			if long, ok := o["long"].(string); ok && long != "" {
				names[long] = true
			}
			if short, ok := o["short"].(string); ok && short != "" {
				names[short] = true
			}
		}
	}
	for _, commandsList := range groups {
		commandsConfigs, ok := commandsList.([]interface{})
		if !ok {
			continue
		}
		for _, commands := range commandsConfigs {
			commandsMap, ok := commands.(map[string]interface{})
			if !ok {
				continue
			}
			for _, commandConfig := range commandsMap {
				cc, ok := commandConfig.(map[string]interface{})
				if !ok {
					continue
				}
				if optionsSlice, ok := cc["options"].([]interface{}); ok {
					addOptionsSlice(optionsSlice)
				}
			}
		}
	}
	return names
}

func (c *cliTransitions) getOptions(optionsSlice []interface{}, fs *flag.FlagSet) (flags map[string]interface{}) {
	flags = make(map[string]interface{})
	for _, opts := range optionsSlice {
		if o, ok := opts.(map[string]interface{}); ok {
			// {"long": "modules", "short": "mp", "value": "modules", "usage": "Path to modules directory"},
			long, ok := o["long"].(string)
			if !ok {
				long = ""
			}
			short, ok := o["short"].(string)
			if !ok {
				short = ""
			}
			value, ok := o["value"].(string)
			if !ok {
				value = ""
			}
			valueBool, ok := o["value"].(bool)
			if !ok {
				valueBool = false
			}
			valueInt, ok := o["value"].(int)
			if !ok {
				valueInt = 0
				// Try to convert float to int if it's a float
			}
			if valueFloat, ok := o["value"].(float64); ok {
				valueInt = int(valueFloat)
			}
			usage, ok := o["usage"].(string)
			if !ok {
				usage = ""
			}
			optionType, ok := o["type"].(string)
			if !ok {
				optionType = "string"
			}
			switch optionType {
			case "bool":
				flags[long] = BoolArg(valueBool, FlagBool(fs, long, short, usage, valueBool)...)
			case "int":
				flags[long] = IntArg(valueInt, FlagInt(fs, long, short, usage, valueInt)...)
			case "string":
				flags[long] = StringArg(value, FlagString(fs, long, short, usage, value)...)
			default:
				flags[long] = StringArg(value, FlagString(fs, long, short, usage, value)...)
			}
		}
	}

	return flags
}

func (c *cliTransitions) UnregisteredCommand(value any) (r domain.FlowStepResult) {
	r.Success = true

	if v, ok := value.(*workflow.WorkerResponse); ok {
		if res, ok := v.Response.(map[string]interface{}); ok {
			if cmd, ok := res["requestedCommand"].(string); ok {
				r.Response = map[string]interface{}{
					"message":          fmt.Sprintf("Command '%s' not found. Use 'help' command to see available commands.", cmd),
					"requestedCommand": cmd,
					"availableCommands": func() []string {
						cmds := make([]string, 0, len(c.commands))
						for cmd := range c.commands {
							cmds = append(cmds, cmd)
						}
						return cmds
					}(),
				}
				return
			}
		}
	}

	if v, ok := value.(map[string]interface{}); ok {
		if res, ok := v["Result"].(map[string]interface{}); ok {
			if cmd, ok := res["requestedCommand"].(string); ok {
				r.Response = map[string]interface{}{
					"message":          fmt.Sprintf("Command '%s' not found. Use 'help' command to see available commands.", cmd),
					"requestedCommand": cmd,
					"availableCommands": func() []string {
						cmds := make([]string, 0, len(c.commands))
						for cmd := range c.commands {
							cmds = append(cmds, cmd)
						}
						return cmds
					}(),
				}
				return
			}
		}
	}

	return
}

// WorkflowExecutor executes a WSL workflow for an HTTP config
func (c *cliTransitions) WorkflowExecutor(command, workflowPath string, config map[string]interface{}, args []string, verbose bool, debug bool, quiet bool) (result domain.FlowStepResult) {
	options := c.Ctx.Engine.GetApplication().Env.Options
	workflowPath, responses := c.runWorkflow(command, workflowPath, options, config, args, verbose, debug, quiet)

	var response *workflow.WorkerResponse
	responseRef, ok := responses[workflowPath]
	if ok {
		response = responseRef
	}
	base := filepath.Base(workflowPath)
	responseRef, ok = responses[base]
	if ok {
		response = responseRef
	}
	if response == nil {
		ok = false
		for _, resp := range responses {
			responseRef = resp
			ok = true
			break
		}
	}
	if ok {
		response = responseRef
	}

	result.Success = true
	if response != nil {
		result.Response = response.Response
		result.StatusCode = response.StatusCode
		result.Error = response.Error
	}
	return
}

func (c *cliTransitions) runWorkflow(command string, workflowPath string, options *domain.Options, config map[string]interface{}, args []string, verbose bool, debug bool, quiet bool) (string, map[string]*workflow.WorkerResponse) {
	// Parse request into workflow arguments
	context := maps.Clone(options.Context)
	settings := maps.Clone(options.Settings)
	options = &domain.Options{
		EngineName:      options.EngineName,
		ConfigName:      options.ConfigName,
		Verbose:         options.Verbose,
		Quiet:           options.Quiet,
		Amount:          options.Amount,
		Retry:           options.Retry,
		RetryDelay:      options.RetryDelay,
		RestartPolicy:   options.RestartPolicy,
		Workflow:        workflowPath,
		Version:         options.Version,
		BuildTime:       options.BuildTime,
		LogPath:         options.LogPath,
		Config:          options.Config,
		Args:            options.Args,
		Context:         context,
		Settings:        settings,
		EmbedFS:         options.EmbedFS,
		EmbedFSRootPath: options.EmbedFSRootPath,
	}
	context["Workflow"] = workflowPath
	context["args"] = args
	context["config"] = config
	context["command"] = command
	context["flags"] = config["flags"]

	if verbose {
		logger.EnableInfo()
	}

	if debug {
		logger.EnableDebug()
	}

	// Execute the workflow
	workflowPath = filepath.Join(c.workflowsPath, workflowPath)
	args = []string{
		// Add configuration to args
		fmt.Sprintf("command=%s", command),
		fmt.Sprintf("modulesPath=%s", c.modulesPath),
		fmt.Sprintf("workflowsPath=%s", c.workflowsPath),
		fmt.Sprintf("version=%s", c.version),
		fmt.Sprintf("buildTime=%s", c.buildTime),
	}
	responses := engine.RunWorkflow("production", &domain.Options{
		EngineName:      "kuetix-cli",
		ConfigName:      "cli",
		Verbose:         verbose || debug,
		Quiet:           quiet,
		Amount:          1,
		Retry:           1,
		RetryDelay:      0,
		RestartPolicy:   options.RestartPolicy,
		Workflow:        workflowPath,
		Version:         options.Version,
		BuildTime:       options.BuildTime,
		LogPath:         options.LogPath,
		Config:          options.Config,
		Args:            args,
		Settings:        options.Settings,
		Context:         context,
		EmbedFS:         options.EmbedFS,
		EmbedFSRootPath: options.EmbedFSRootPath,
	})

	return workflowPath, responses
}

func (c *cliTransitions) resolveCommandConfig(commandConfig, commandsMap map[string]interface{}) map[string]interface{} {
	if extends, ok := commandConfig["$extends"]; ok {
		if parent, ok := commandsMap[extends.(string)]; ok {
			parentMap := parent.(map[string]interface{})
			mergedConfig := make(map[string]interface{})
			for k, v := range parentMap {
				if k == "$extends" {
					v = c.resolveCommandConfig(parentMap, commandsMap)
					if v == nil {
						v = parentMap
					} else {
						for vk, vv := range v.(map[string]interface{}) {
							mergedConfig[vk] = vv
						}
					}
					continue
				}
				mergedConfig[k] = v
			}
			for k, v := range commandConfig {
				if k == "$extends" {
					continue
				}
				mergedConfig[k] = v
			}
			return mergedConfig
		}
	}

	return commandConfig
}
