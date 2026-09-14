package domain

type IniConfig map[string]interface{}

type ApplicationConfig struct {
	Name          string    `json:"name,omitempty" mapstructure:"name"`
	Env           string    `json:"env,omitempty" mapstructure:"env"`
	Debug         bool      `json:"debug,omitempty" mapstructure:"debug"`
	Timezone      string    `json:"timezone,omitempty" json:"timezone"`
	Locale        string    `json:"locale,omitempty" mapstructure:"locale"`
	Version       string    `json:"version,omitempty" mapstructure:"version"`
	BuildTime     string    `json:"build_time,omitempty" mapstructure:"build_time"`
	LogFile       string    `json:"log_file,omitempty" mapstructure:"log_file"`
	ModulesPath   string    `json:"modules_path,omitempty" mapstructure:"modules_path"`
	WorkflowsPath string    `json:"workflows_path,omitempty" mapstructure:"workflows_path"`
	Items         IniConfig `json:"items,omitempty" mapstructure:"items"`
}

type WorkflowConfigItem struct {
	Name          string                 `json:"name,omitempty" mapstructure:"name"`
	Path          string                 `json:"path,omitempty" mapstructure:"path"`
	Amount        int                    `json:"amount,omitempty" mapstructure:"amount"`
	Retry         int                    `json:"retry,omitempty" mapstructure:"retry"`
	RetryDelay    int                    `json:"retry_delay,omitempty" mapstructure:"retryDelay"`
	RestartPolicy string                 `json:"restart_policy,omitempty" mapstructure:"restart_policy"`
	Options       map[string]interface{} `json:"-,omitempty" mapstructure:"options,remain"`
}

type Config struct {
	FilePath       string               `json:"file_path,omitempty" mapstructure:"file_path"`
	Application    ApplicationConfig    `json:"application,omitempty" mapstructure:"application"`
	WorkflowConfig []WorkflowConfigItem `json:"workflow,omitempty" mapstructure:"workflow"`
	Items          IniConfig            `json:"items,omitempty" mapstructure:"items"`
}
