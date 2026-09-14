package domain

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kuetix/engine/engine/defines"
	h "github.com/kuetix/engine/engine/helpers"
	"github.com/kuetix/engine/event"
	"github.com/kuetix/helpers"
	"github.com/kuetix/logger"
	"github.com/spf13/viper"
	"gopkg.in/ini.v1"
)

type Environment struct {
	Name       string `json:"name,omitempty" mapstructure:"NAME"`
	AppEnv     string `json:"appEnv,omitempty" mapstructure:"APP_ENV"`
	ConfigPath string `json:"configPath,omitempty" mapstructure:"CONFIG_PATH"`
	Options    *Options
	Config     *Config
}

func NewEnvironment(env string, options *Options) *Environment {
	var opts Options
	if options == nil {
		opts = Options{
			EngineName:      "engine",
			ConfigName:      "engine",
			Verbose:         false,
			Quiet:           false,
			Amount:          1,
			Retry:           1,
			RetryDelay:      0,
			RestartPolicy:   defines.RestartPolicyStop,
			Workflow:        "",
			Version:         "dev",
			BuildTime:       time.Now().Format(time.RFC3339),
			LogPath:         "stdout",
			Config:          nil,
			Args:            []string{},
			Context:         map[string]interface{}{},
			Settings:        map[string]interface{}{},
			EmbedFS:         nil,
			EmbedFSRootPath: "",
		}
	} else {
		opts = Options{
			EngineName:      options.EngineName,
			ConfigName:      options.ConfigName,
			Verbose:         options.Verbose,
			Quiet:           options.Quiet,
			Amount:          options.Amount,
			Retry:           options.Retry,
			RetryDelay:      options.RetryDelay,
			RestartPolicy:   options.RestartPolicy,
			Workflow:        options.Workflow,
			Version:         options.Version,
			BuildTime:       options.BuildTime,
			LogPath:         options.LogPath,
			Config:          nil,
			Args:            make([]string, len(options.Args)),
			Context:         make(map[string]interface{}),
			Settings:        make(map[string]interface{}),
			EmbedFS:         options.EmbedFS,
			EmbedFSRootPath: options.EmbedFSRootPath,
		}
		if len(options.Args) > 0 {
			copy(opts.Args, options.Args)
		}
		if len(options.Context) > 0 {
			for key, value := range options.Context {
				opts.Context[key] = value
			}
		}
		if len(options.Settings) > 0 {
			for key, value := range options.Settings {
				opts.Settings[key] = value
			}
		}
		if options.Config == nil {
			opts.Config = &Config{
				FilePath: "",
				Application: ApplicationConfig{
					Name:          options.EngineName,
					Env:           env,
					Debug:         false,
					Timezone:      "UTC",
					Locale:        "en",
					Version:       options.Version,
					BuildTime:     options.BuildTime,
					LogFile:       options.LogPath,
					ModulesPath:   "modules",
					WorkflowsPath: "workflows",
					Items:         map[string]interface{}{},
				},
				WorkflowConfig: make([]WorkflowConfigItem, 0),
				Items:          map[string]interface{}{},
			}
		} else {
			opts.Config = &Config{
				FilePath: options.Config.FilePath,
				Application: ApplicationConfig{
					Name:          options.Config.Application.Name,
					Env:           options.Config.Application.Env,
					Debug:         options.Config.Application.Debug,
					Timezone:      options.Config.Application.Timezone,
					Locale:        options.Config.Application.Locale,
					Version:       options.Config.Application.Version,
					BuildTime:     options.Config.Application.BuildTime,
					LogFile:       options.Config.Application.LogFile,
					ModulesPath:   options.Config.Application.ModulesPath,
					WorkflowsPath: options.Config.Application.WorkflowsPath,
					Items:         maps.Clone(options.Config.Application.Items),
				},
				WorkflowConfig: make([]WorkflowConfigItem, len(options.Config.WorkflowConfig)),
				Items:          maps.Clone(options.Config.Items),
			}
			if len(options.Config.WorkflowConfig) > 0 {
				for k, v := range options.Config.WorkflowConfig {
					opts.Config.WorkflowConfig[k] = WorkflowConfigItem{
						Name:          strings.TrimSpace(v.Name),
						Path:          strings.TrimSpace(v.Path),
						Amount:        v.Amount,
						Retry:         v.Retry,
						RetryDelay:    v.RetryDelay,
						RestartPolicy: strings.TrimSpace(v.RestartPolicy),
						Options:       maps.Clone(v.Options),
					}
				}
			}
		}
	}

	newEnv := &Environment{
		Name:       opts.ConfigName,
		Options:    &opts,
		Config:     opts.Config,
		AppEnv:     env,
		ConfigPath: "",
	}

	cfg := newEnv.Config
	if cfg.Application.Debug {
		logger.EnableDebug()
	}

	if opts.Quiet {
		cfg.Application.Debug = false
		logger.EnableQuiet()
	}

	newEnv.Bootstrap(&opts)

	event.Bus.Publish("on:wsl:env:boot", env, &opts)
	return newEnv
}

func (env *Environment) Bootstrap(options *Options) {
	var err error

	// Initialize Config struct
	var config *Config

	logger.Debug("[engine:boot] Bootstrap")
	cwd := env.GetEnvPath()
	logger.Debug(fmt.Sprintf("[engine:boot] Environment path: %s", cwd))
	cwd = fmt.Sprintf("/%s/", strings.Trim(cwd, "/"))
	configFilePath := fmt.Sprintf("%s.env", cwd)
	if _, err = os.Stat(configFilePath); !os.IsNotExist(err) {
		logger.Debug(fmt.Sprintf("[engine:boot] Environment file found: %s", configFilePath))
		viper.SetConfigFile(configFilePath)

		err = viper.ReadInConfig()
		if err != nil {
			logger.Fatal(fmt.Sprintf("[engine:boot] Environment can't be loaded: %s", err))
		}

		err = viper.Unmarshal(env)
		if err != nil {
			logger.Fatal(fmt.Sprintf("[engine:boot] Environment can't be loaded: %s", err))
		}
	} else {
		logger.Debug(fmt.Sprintf("[engine:boot] Environment file not found, using defaults: %s", configFilePath))
		env.AppEnv = "production"
		env.ConfigPath = filepath.Join("etc", env.AppEnv)
	}

	if env.AppEnv == "" {
		env.AppEnv = "production"
	}

	logger.Debug(fmt.Sprintf("[engine] Environment mode: %s", env.AppEnv))

	var iniFileFound = false
	if _, err = os.Stat(filepath.Join(cwd, env.ConfigPath)); !os.IsNotExist(err) {
		logger.Debug(fmt.Sprintf("[engine:boot] Environment config path: CWD: %s, env.ConfigPath: %s", cwd, env.ConfigPath))
		config, err = env.LoadConfig(options.ConfigName, env.ConfigPath, options.Config)
		if err != nil {
			dir := fmt.Sprintf("%s%s", cwd, env.ConfigPath)
			config, err = env.LoadConfig(options.ConfigName, dir, options.Config)
			if err != nil {
				logger.Debugf("Error loading config: %v", err)
				iniFileFound = false
			} else {
				logger.Debug(fmt.Sprintf("[engine:boot] Config loaded from: %s", filepath.Join(dir, options.ConfigName)))
				iniFileFound = true
			}
		} else {
			logger.Debug(fmt.Sprintf("[engine:boot] Config loaded from: %s", filepath.Join(cwd, env.ConfigPath)))
			iniFileFound = true
		}
	}

	if !iniFileFound {
		config = &Config{}
		config.Application.Env = env.AppEnv
		config.Application.Debug = false
		config.Application.Locale = "en"
		config.Application.LogFile = "wsl.log"
		config.Application.ModulesPath = "modules"
		config.Application.WorkflowsPath = "workflows"
		if options.LogPath != "" {
			config.Application.LogFile = options.LogPath
		}
	}

	logger.SetJsonFormat(true)
	if config != nil && (config.Application.LogFile == "" || strings.TrimSpace(strings.ToLower(config.Application.LogFile)) == "stdout") {
		logger.SetOutput(logger.StdOut)
	} else if config != nil && strings.TrimSpace(strings.ToLower(config.Application.LogFile)) == "stderr" {
		logger.SetOutput(logger.StdErr)
	} else {
		if config != nil {
			logger.SetOutputPath(strings.TrimSpace(config.Application.LogFile))
		}
		logger.Info("[engine:boot] Bootstrap")
	}

	if config != nil {
		err = env.EnsureLogs(config, cwd)
		if err != nil {
			logger.Fatalf("Error prepare logs: %v", err)
		}
	}

	env.Config = config
	env.Config.Application.Version = options.Version
	env.Config.Application.BuildTime = options.BuildTime

	return
}

func (env *Environment) GetEnvPath() string {
	cwd, _ := os.Getwd()
	if strings.Contains(cwd, "tests") {
		parts := strings.Split(cwd, "tests")
		cwd = fmt.Sprintf("%stests/", parts[0])
		return cwd
	}

	return cwd
}

func (env *Environment) LoadConfig(configName string, dir string, configs ...*Config) (*Config, error) {
	logger.Debug(fmt.Sprintf("[engine:boot] Loading config from: directory: %s, configName: %s", dir, configName))
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %w", err)
	}
	logger.Debug(fmt.Sprintf("[engine:boot] Current working directory: %s and directory: %s", wd, dir))
	if strings.Trim(wd, "/") == strings.Trim(dir, "/") {
		wd = ""
	}
	configFullFilePath := filepath.Join(wd, strings.TrimRight(dir, "/"))
	logger.Debug(fmt.Sprintf("[engine:boot] Full config path: %s and configName: %s", configFullFilePath, configName+".ini"))

	var config Config
	if len(configs) > 0 && configs[0] != nil {
		config = *configs[0]
	} else {
		config = Config{}
	}

	filePath, err := helpers.FindFileDown(configFullFilePath, configName+".ini")
	// runtime/etc/development/engine.ini
	if err == os.ErrNotExist {
		cfg, err := ini.Load(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to load configuration file: %w", err)
		}

		// Initialize Config struct
		config.FilePath = filePath

		// Load API configuration
		if err = env.LoadApplicationConfig(cfg, &config); err != nil {
			logger.Fatalf("Failed to load API configuration: %v", err)
		}

		wfs := cfg.ChildSections("workflow")
		if cfg.HasSection("workflow") || configName == "workflow" || len(wfs) > 0 {
			if err = env.LoadWorkflowConfig(cfg, &config); err != nil {
				logger.Fatalf("Failed to load API configuration: %v", err)
			}
		}
		// Load API configuration

		if _, err = os.Stat(dir); err == nil {
			entries, err := os.ReadDir(dir)
			if err != nil {
				logger.Fatalf("Failed to read directory: %v", err)
			}

			var info os.FileInfo
			for _, filename := range entries {
				if !filename.IsDir() && filepath.Ext(filename.Name()) == ".ini" { // Check if it's a file, not a directory
					info, err = filename.Info()
					if err != nil {
						logger.Fatalf("failed to get file info: %s", err)
					}
					filePath := fmt.Sprintf("%s/%s", dir, info.Name())
					cfg, err = ini.Load(filePath)
					if err != nil {
						logger.Fatalf("failed to load configuration file: %s", err)
					}
				}
			}
		}
	}

	return &config, nil
}

func (env *Environment) EnsureLogs(config *Config, dir string) error {
	logfile := strings.TrimSpace(config.Application.LogFile)
	if logfile == "" {
		return nil
	}
	if logfile == "stdout" || logfile == "stderr" {
		return nil
	}
	logfile = dir + config.Application.LogFile
	err := os.MkdirAll(filepath.Dir(logfile), 0755)
	if err != nil {
		return err
	}

	err = helpers.TouchFile(logfile)
	if err != nil {
		return err
	}

	return nil
}

func (env *Environment) LoadApplicationConfig(cfg *ini.File, config *Config) error {
	// Load [application]
	if cfg.HasSection("application") {
		section := cfg.Section("application")
		config.Application.Env = section.Key("env").MustString("production")
		config.Application.Debug = section.Key("debug").MustBool(false)
		config.Application.Locale = section.Key("locale").MustString("en")
		config.Application.LogFile = section.Key("log_file").MustString("wsl.log")
		config.Application.ModulesPath = section.Key("modules_path").MustString("modules")
		config.Application.WorkflowsPath = section.Key("workflows_path").MustString("workflows")
	}

	config.Items = env.LoadAsMapConfig(cfg, config.Items)

	return nil
}

func (env *Environment) LoadAsMapConfig(cfg *ini.File, config IniConfig) IniConfig {
	if config == nil {
		config = IniConfig{}
	}
	// Load all sections []
	sections := cfg.Sections()
	for _, s := range sections {
		name := s.Name()
		section := cfg.Section(name)
		hashes := section.KeysHash()
		for k, v := range hashes {
			if name == "DEFAULT" {
				config[k] = v
			} else {
				if config[name] == nil {
					config[name] = IniConfig{}
				}
				config[name].(IniConfig)[k] = v
			}
		}
	}

	return config
}

func (env *Environment) LoadWorkflowConfig(cfg *ini.File, config *Config) error {
	if config.WorkflowConfig == nil {
		config.WorkflowConfig = []WorkflowConfigItem{}
	}
	sections := cfg.Sections()
	for _, s := range sections {
		sectionName := s.Name()
		if strings.HasPrefix(sectionName, "workflow.") {
			section := cfg.Section(sectionName)
			key := section.Key("name").String()
			path := section.Key("path").String()
			if key == "" {
				continue
			}
			if path == "" {
				var err error
				path, err = h.ModulesPath("./modules")
				if err != nil {
					logger.Errorf("failed to resolve modules path: %v", err)
					continue
				}
			}
			item := WorkflowConfigItem{
				Name:          key,
				Path:          path,
				Amount:        section.Key("amount").MustInt(1),
				Retry:         section.Key("retry").MustInt(3),
				RetryDelay:    section.Key("retry_delay").MustInt(1),
				RestartPolicy: section.Key("restart_policy").MustString("on-failure"),
			}
			config.WorkflowConfig = append(config.WorkflowConfig, item)
		}
	}

	return nil
}

func (env *Environment) ShutdownEnvironment() {
	logger.CloseLogFile()
}
