package transitions

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/services/caches"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type projectTransitions struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewProjectTransition() interfaces.ServiceTransitions { return &projectTransitions{} }

// ---------------------------------------------------------------------------
// RunCommand — build and/or run the current project
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (p *projectTransitions) RunCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	cfg := config
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet := config["flagSet"].(*flag.FlagSet)
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText += buf.String()
		r.Success = true
		r.Response = helpText
		return
	}

	name := options["name"].(string)
	appType := options["app-type"].(string)
	output := options["output"].(string)

	projectDir, err := resolveProjectDir(name)
	if err != nil {
		r.Error = fmt.Errorf("failed to resolve project directory: %w", err)
		return
	}

	if appType == "" {
		detected, err := readAppType(projectDir)
		if err != nil {
			r.Error = fmt.Errorf("failed to read application type: %w", err)
			return
		}
		appType = detected
	}

	switch appType {
	case "package":
		out, err := runPackage(projectDir)
		if err != nil {
			r.Error = fmt.Errorf("package build failed: %w\n%s", err, out)
			return
		}
		r.Success = true
		r.Response = out
	case "cli", "api", "consumer", "service":
		out, err := runApp(projectDir, appType, output)
		if err != nil {
			r.Error = fmt.Errorf("application run failed: %w\n%s", err, out)
			return
		}
		r.Success = true
		r.Response = out
	default:
		r.Error = fmt.Errorf("unsupported application type: %s", appType)
	}
	return
}

// ---------------------------------------------------------------------------
// UpdateCommand — reinitialise the engine environment and regenerate caches
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (p *projectTransitions) UpdateCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	helpText := config["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet := config["flagSet"].(*flag.FlagSet)
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText += buf.String()
		r.Success = true
		r.Response = helpText
		return
	}

	quiet := options["quiet"].(bool)
	opts := p.Ctx.Engine.GetApplication().Env.Options
	opts.Verbose = options["verbose"].(bool)
	opts.Quiet = quiet

	env := domain.NewEnvironment("production", opts)

	if !quiet {
		fmt.Println("Updating module cache (di.go, meta.go, modules.json)...")
	}

	caches.GenerateMetaCache(env)

	var sb strings.Builder
	if !quiet {
		sb.WriteString("Module cache update completed successfully!\n")
	}

	r.Success = true
	r.Response = sb.String()
	return
}

// ---------------------------------------------------------------------------
// ShowCommand — display project metadata (apps, workflows, features, etc.)
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (p *projectTransitions) ShowCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	cfg := config
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet := config["flagSet"].(*flag.FlagSet)
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText += buf.String()
		r.Success = true
		r.Response = helpText
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		r.Error = fmt.Errorf("failed to get working directory: %w", err)
		return
	}

	var sb strings.Builder
	showApplications(&sb, cwd)
	showWorkflows(&sb, cwd)
	showFeatures(&sb, cwd)
	showSolutions(&sb, cwd)
	showModules(&sb, cwd)

	if sb.Len() == 0 {
		sb.WriteString("No project metadata found in the current directory.\n")
	}

	r.Success = true
	r.Response = sb.String()
	return
}

// ---------------------------------------------------------------------------
// WorkflowCommand — inspect a workflow file and list its actions / params
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (p *projectTransitions) WorkflowCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	cfg := config
	helpText := cfg["usage"].(string) + "\n"
	options := GetFlags(flags)

	if options["help"].(bool) {
		var buf bytes.Buffer
		flagSet := config["flagSet"].(*flag.FlagSet)
		flagSet.SetOutput(&buf)
		flagSet.Usage()
		helpText += buf.String()
		r.Success = true
		r.Response = helpText
		return
	}

	var workflowName string
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		workflowName = args[0]
	}
	if n, ok := options["name"].(string); ok && strings.TrimSpace(n) != "" {
		workflowName = n
	}
	if strings.TrimSpace(workflowName) == "" {
		r.Error = fmt.Errorf("workflow name is required (usage: kue workflow <name>)")
		return
	}

	eng, ok := p.Ctx.Engine.(*workflow.Engine)
	if !ok {
		r.Error = fmt.Errorf("unexpected engine type: %T", p.Ctx.Engine)
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		r.Error = fmt.Errorf("failed to get working directory: %w", err)
		return
	}
	prevWD := eng.WorkingDir
	prevWP := eng.WorkflowPath
	eng.WorkingDir = cwd
	eng.WorkflowPath = "workflows"
	defer func() {
		eng.WorkingDir = prevWD
		eng.WorkflowPath = prevWP
	}()

	actions, err := eng.GetWorkflowActions(workflowName)
	if err != nil {
		r.Error = fmt.Errorf("failed to read workflow %q: %w", workflowName, err)
		return
	}

	pretty := options["pretty"].(bool)
	asJSON := options["json"].(bool) || pretty
	if asJSON {
		var data []byte
		var jerr error
		if pretty {
			data, jerr = json.MarshalIndent(actions, "", "  ")
		} else {
			data, jerr = json.Marshal(actions)
		}
		if jerr != nil {
			r.Error = fmt.Errorf("failed to marshal actions: %w", jerr)
			return
		}
		r.Success = true
		r.Response = string(data)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workflow: %s\n", workflowName))
	if len(actions) == 0 {
		sb.WriteString("  (no actions found)\n")
		r.Success = true
		r.Response = sb.String()
		return
	}

	currentWF := ""
	for _, a := range actions {
		if a.Workflow != currentWF {
			currentWF = a.Workflow
			sb.WriteString(fmt.Sprintf("\n  workflow %s:\n", currentWF))
		}
		action := a.Name
		if a.Module != "" {
			action = a.Module + "." + a.Name
		}
		sb.WriteString(fmt.Sprintf("    [%s] %s", a.State, action))
		if a.As != "" {
			sb.WriteString(" as " + a.As)
		}
		if a.Terminal != "" {
			sb.WriteString(fmt.Sprintf(" (end %s)", a.Terminal))
		}
		sb.WriteString("\n")
		for _, arg := range a.Args {
			if arg.Name != "" {
				sb.WriteString(fmt.Sprintf("      - %s: %s\n", arg.Name, arg.Value))
			} else {
				sb.WriteString(fmt.Sprintf("      - %s\n", arg.Value))
			}
		}
		if len(a.Params) > 0 {
			sb.WriteString(fmt.Sprintf("      params: %s\n", strings.Join(a.Params, ", ")))
		}
	}

	r.Success = true
	r.Response = sb.String()
	return
}

// ---------------------------------------------------------------------------
// Helpers — project directory resolution
// ---------------------------------------------------------------------------

func resolveProjectDir(name string) (string, error) {
	if name != "" {
		abs, err := filepath.Abs(name)
		if err != nil {
			return "", err
		}
		if !isProjectDir(abs) {
			return "", fmt.Errorf("directory %s is not a valid project (missing application.json)", abs)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if !isProjectDir(cwd) {
		return "", fmt.Errorf("current directory is not a valid project (missing application.json)")
	}
	return cwd, nil
}

func isProjectDir(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, "application.json"))
	return err == nil && !info.IsDir()
}

func readAppType(projectDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(projectDir, "application.json"))
	if err != nil {
		return "", fmt.Errorf("cannot read application.json: %w", err)
	}
	var meta shared.ApplicationMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return "", fmt.Errorf("invalid application.json: %w", err)
	}
	if meta.Type == "" {
		return "", fmt.Errorf("application.json does not specify a type")
	}
	return meta.Type, nil
}

// ---------------------------------------------------------------------------
// Helpers — build / run
// ---------------------------------------------------------------------------

func runPackage(projectDir string) (string, error) {
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = projectDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), err
	}
	result := strings.TrimSpace(string(out))
	if result == "" {
		result = "Package built successfully."
	}
	return result, nil
}

func runApp(projectDir, appType, output string) (string, error) {
	if output == "" {
		output = filepath.Join(projectDir, "bin", appType)
	}

	// Build the binary.
	buildCmd := exec.Command("go", "build", "-o", output, ".")
	buildCmd.Dir = projectDir
	buildOut, err := buildCmd.CombinedOutput()
	if err != nil {
		return string(buildOut), fmt.Errorf("build failed: %w", err)
	}

	// Run the binary.
	runCmd := exec.Command(output)
	runCmd.Dir = projectDir
	runOut, err := runCmd.CombinedOutput()
	if err != nil {
		return string(runOut), fmt.Errorf("run failed: %w", err)
	}

	return strings.TrimSpace(string(runOut)), nil
}

// ---------------------------------------------------------------------------
// Helpers — show metadata sections
// ---------------------------------------------------------------------------

func showApplications(sb *strings.Builder, baseDir string) {
	metaPath := filepath.Join(baseDir, "application.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return
	}
	var meta shared.ApplicationMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return
	}
	sb.WriteString("Applications:\n")
	sb.WriteString(fmt.Sprintf("  - %s (type: %s, version: %s)\n", meta.Name, meta.Type, meta.Version))
	sb.WriteString("\n")
}

func showWorkflows(sb *strings.Builder, baseDir string) {
	dir := filepath.Join(baseDir, "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var items []shared.WorkflowMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var meta shared.WorkflowMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		items = append(items, meta)
	}
	if len(items) == 0 {
		return
	}
	sb.WriteString("Workflows:\n")
	for _, m := range items {
		sb.WriteString(fmt.Sprintf("  - %s (type: %s, version: %s)\n", m.Name, m.Type, m.Version))
	}
	sb.WriteString("\n")
}

func showFeatures(sb *strings.Builder, baseDir string) {
	dir := filepath.Join(baseDir, "workflows", "features")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var items []shared.FeatureMetadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var meta shared.FeatureMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		items = append(items, meta)
	}
	if len(items) == 0 {
		return
	}
	sb.WriteString("Features:\n")
	for _, m := range items {
		sb.WriteString(fmt.Sprintf("  - %s (type: %s, version: %s)\n", m.Name, m.Type, m.Version))
	}
	sb.WriteString("\n")
}

func showSolutions(sb *strings.Builder, baseDir string) {
	dir := filepath.Join(baseDir, "workflows", "solutions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var items []shared.SolutionMetadata
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var meta shared.SolutionMetadata
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		items = append(items, meta)
	}
	if len(items) == 0 {
		return
	}
	sb.WriteString("Solutions:\n")
	for _, m := range items {
		sb.WriteString(fmt.Sprintf("  - %s (type: %s, version: %s)\n", m.Name, m.Type, m.Version))
	}
	sb.WriteString("\n")
}

func showModules(sb *strings.Builder, baseDir string) {
	dir := filepath.Join(baseDir, "modules")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	if len(names) == 0 {
		return
	}
	sb.WriteString("Modules:\n")
	for _, n := range names {
		sb.WriteString(fmt.Sprintf("  - %s\n", n))
	}
	sb.WriteString("\n")
}
