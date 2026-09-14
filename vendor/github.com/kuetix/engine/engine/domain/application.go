package domain

import (
	"embed"
	"io"
	"log"
	"os"
	"path/filepath"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/modules"
	"github.com/kuetix/logger"
)

type Application struct {
	EngineName      string
	Env             *Environment
	EmbedFS         embed.FS
	EmbedFSRootPath string
}

type Options struct {
	EngineName      string `json:"engine_name,omitempty" mapstructure:"engine_name"`
	ConfigName      string `json:"config_name,omitempty" mapstructure:"config_name"`
	Verbose         bool   `json:"verbose,omitempty" mapstructure:"verbose"`
	Quiet           bool   `json:"quiet,omitempty" mapstructure:"quiet"`
	Amount          int    `json:"amount,omitempty" mapstructure:"amount"`
	Retry           int    `json:"retry,omitempty" mapstructure:"retry"`
	RetryDelay      int    `json:"retry_delay,omitempty" mapstructure:"retryDelay"`
	RestartPolicy   string `json:"restart_policy,omitempty" mapstructure:"restart_policy"`
	Workflow        string `json:"workflow,omitempty" mapstructure:"workflow"`
	Version         string `json:"version,omitempty" mapstructure:"version"`
	BuildTime       string `json:"build_time,omitempty" mapstructure:"build_time"`
	LogPath         string `json:"log_path,omitempty" mapstructure:"log_path"`
	Config          *Config
	Args            []string
	Context         map[string]interface{}
	Settings        map[string]interface{} `json:"-,omitempty" mapstructure:"-"`
	EmbedFS         *embed.FS
	EmbedFSRootPath string
}

func NewApplication(env *Environment) Application {
	boot.DependencyInjection()

	if env.Options.EmbedFS == nil {
		env.Options.EmbedFS = &embed.FS{}
	}
	fs := env.Options.EmbedFS
	if di.Has("workflows_embed_fs") {
		fs = di.Fetch("workflows_embed_fs").(*embed.FS)
	}
	fsRootPath := env.Options.EmbedFSRootPath
	if di.Has("workflows_embed_fs_root_path") {
		fsRootPath = di.Fetch("workflows_embed_fs_root_path").(string)
	}

	if di.Has("env") {
		env = di.Fetch("env").(*Environment)
	}

	app := Application{
		Env:             env,
		EmbedFS:         *fs,
		EmbedFSRootPath: fsRootPath,
	}

	di.ToFetch("app", app)

	modules.Enable()
	di.DependencyInjectionBoot()

	// Open or create the log file
	var mw io.Writer
	var err error
	var file *os.File
	logFile := env.Config.Application.LogFile
	if logFile == "" {
		logFile = "stdout"
	}
	if filepath.Base(logFile) != "stdout" && filepath.Base(logFile) != "stderr" {
		file, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
		if err != nil {
			logger.Fatalf("Failed to open log file: %v", err)
		}
		defer func(file *os.File) {
			err = file.Close()
			if err != nil {
				logger.Fatalf("Failed to close log file: %v", err)
			}
		}(file)
		// Redirect log output to the file
		mw = io.MultiWriter(os.Stdout, file)
	} else {
		// Redirect log output to the file
		mw = io.MultiWriter(os.Stdout)
	}

	log.SetOutput(mw)

	return app
}

func (app *Application) Close() {
	println("Close")
}
