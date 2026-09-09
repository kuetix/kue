package transitions

import (
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
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(p.GetSession(), config, flags)
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

// IsProjectDir reports whether dir contains an application.json manifest.
// Exported for the `kue run` dispatcher in modules/runner, which auto-detects
// project directories vs. workflow files vs. registry names.
func IsProjectDir(dir string) bool { return isProjectDir(dir) }

// RunProjectDir builds and runs the project rooted at dir (cwd when dir is
// ""), mirroring `kue run`'s project behavior. Returns the combined program
// output. Exported for the modules/runner dispatcher.
func RunProjectDir(dir string) (string, error) {
	projectDir, err := resolveProjectDir(dir)
	if err != nil {
		return "", err
	}
	appType, err := readAppType(projectDir)
	if err != nil {
		return "", err
	}
	switch appType {
	case "package":
		return runPackage(projectDir)
	case "cli", "api", "consumer", "service":
		return runApp(projectDir, appType, "")
	default:
		return "", fmt.Errorf("unsupported application type: %s", appType)
	}
}

// ---------------------------------------------------------------------------
// UpdateCommand — reinitialise the engine environment and regenerate caches
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (p *projectTransitions) UpdateCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(p.GetSession(), config, flags)
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
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(p.GetSession(), config, flags)
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
func (p *projectTransitions) WorkflowCommand(command map[string]interface{}, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(p.GetSession(), config, flags)
		return
	}

	var workflowName string
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		workflowName = args[0]
	}
	if n, ok := options["name"].(string); ok && strings.TrimSpace(n) != "" {
		workflowName = n
	}
	// `kue inspect <name>` — <name> is parsed by the CLI as the command
	// "inspect.<name>" with empty args; recover it from the requested command.
	if strings.TrimSpace(workflowName) == "" {
		full, _ := command["command"].(string)
		main, _ := command["main_command"].(string)
		if main != "" && strings.HasPrefix(full, main+".") {
			if t := strings.TrimSpace(strings.TrimPrefix(full, main+".")); t != "" && t != "*" {
				workflowName = t
			}
		}
	}
	if strings.TrimSpace(workflowName) == "" {
		r.Error = fmt.Errorf("workflow name is required (usage: kue inspect <name>)")
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

	// Resolve the workflow to on-disk source: an explicit file path, then the
	// local project (./workflows), then the ~/.kue install cache — so
	// `kue inspect <name>` works on anything `kue get` has fetched, offline.
	srcPath, srcKind, cw := resolveWorkflowSource(workflowName, cwd)
	if srcPath == "" {
		r.Error = fmt.Errorf(
			"workflow %q not found — not in ./workflows and not in the ~/.kue cache; fetch it first with `kue get %s`",
			workflowName, workflowName)
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

	// GetWorkflowActions takes a project-relative name (WorkflowPath=workflows)
	// or an absolute path — feed it the resolved path unless it's a project
	// workflow addressed by name.
	lookup := workflowName
	if srcKind != "project" {
		lookup = srcPath
	}
	actions, err := eng.GetWorkflowActions(lookup)
	if err != nil {
		r.Error = fmt.Errorf("failed to read workflow %q: %w", workflowName, err)
		return
	}

	content, _ := os.ReadFile(srcPath)
	deps := cachedDependencies(cw)
	showSource, _ := options["source"].(bool)

	pretty, _ := options["pretty"].(bool)
	asJSON := false
	if b, _ := options["json"].(bool); b || pretty {
		asJSON = true
	}
	if asJSON {
		out := map[string]interface{}{
			"name":        workflowName,
			"source":      displayPath(srcPath),
			"source_kind": srcKind,
			"actions":     actions,
		}
		if showSource {
			out["content"] = string(content)
		}
		if srcKind == "cache" {
			out["registry"] = map[string]interface{}{
				"owner":      cw.Owner,
				"version":    cw.Version,
				"hash":       metaString(cw.Meta, "hash"),
				"fetched_at": metaString(cw.Meta, "fetched_at"),
			}
		}
		if len(deps) > 0 {
			out["dependencies"] = deps
		}
		var data []byte
		var jerr error
		if pretty {
			data, jerr = json.MarshalIndent(out, "", "  ")
		} else {
			data, jerr = json.Marshal(out)
		}
		if jerr != nil {
			r.Error = fmt.Errorf("failed to marshal: %w", jerr)
			return
		}
		r.Success = true
		r.Response = string(data)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workflow: %s\n", workflowName))
	sb.WriteString(fmt.Sprintf("  source:   %s  (%s)\n", displayPath(srcPath), sourceLabel(srcKind)))
	if srcKind == "cache" {
		line := fmt.Sprintf("  registry: owner=%s version=%d", cw.Owner, cw.Version)
		if h := metaString(cw.Meta, "hash"); h != "" {
			line += "  hash=" + shortHash(h)
		}
		if f := metaString(cw.Meta, "fetched_at"); f != "" {
			line += "  fetched " + f
		}
		sb.WriteString(line + "\n")
	}

	if len(deps) > 0 {
		sb.WriteString(fmt.Sprintf("\n  dependencies (%d):\n", len(deps)))
		for _, d := range deps {
			ref := d.Namespace
			if d.Class != "" {
				ref += "/" + d.Class
			}
			line := "    - " + ref
			if d.GoModule != "" {
				line += "  " + d.GoModule
			}
			if d.Version != "" {
				line += " " + d.Version
			}
			sb.WriteString(line + "\n")
			if d.IsLocal {
				sb.WriteString("        local (replace directive) — not a published package\n")
			} else if pkg := packageNameFromModule(d.GoModule); pkg != "" {
				sb.WriteString("        registry package: " + pkg + "   (kue package get " + pkg + ")\n")
			}
		}
	}

	if len(actions) == 0 {
		sb.WriteString("\n  (no actions found)\n")
	} else {
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
	}

	if showSource && len(content) > 0 {
		sb.WriteString("\n  --- source (" + filepath.Base(srcPath) + ") ---\n")
		for _, ln := range strings.Split(strings.TrimRight(string(content), "\n"), "\n") {
			sb.WriteString("  " + ln + "\n")
		}
	} else if len(content) > 0 {
		sb.WriteString("\n  (pass --source / -S to print the WSL text)\n")
	}

	r.Success = true
	r.Response = sb.String()
	return
}

// packageNameFromModule mirrors the registry's harvested-package naming: the Go
// module path with the VCS host segment dropped and "/" replaced by "-"
// (github.com/kuetix/std-core -> kuetix-std-core). Returns "" for a
// non-module-path input.
func packageNameFromModule(mod string) string {
	mod = strings.TrimSpace(mod)
	if mod == "" {
		return ""
	}
	parts := strings.Split(mod, "/")
	if len(parts) > 1 && strings.Contains(parts[0], ".") {
		parts = parts[1:] // drop the host (github.com, gitlab.com, …)
	}
	return strings.Join(parts, "-")
}

// depInfo is one dependency entry from a cached workflow's .meta.json.
type depInfo struct {
	GoModule  string `json:"go_module"`
	Namespace string `json:"namespace"`
	Class     string `json:"class"`
	Version   string `json:"version"`
	IsLocal   bool   `json:"is_local"`
}

// resolveWorkflowSource locates a workflow's source file. kind is "file"
// (explicit path), "project" (./workflows/<name>), or "cache" (~/.kue); the
// CachedWorkflow is populated only for "cache".
func resolveWorkflowSource(name, cwd string) (path, kind string, cw shared.CachedWorkflow) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".wsl" || ext == ".swsl" {
		if abs, err := filepath.Abs(name); err == nil {
			if _, e := os.Stat(abs); e == nil {
				return abs, "file", shared.CachedWorkflow{}
			}
		}
	}
	for _, e := range []string{".wsl", ".swsl"} {
		cand := filepath.Join(cwd, "workflows", filepath.FromSlash(strings.TrimSuffix(name, ext))+e)
		if _, err := os.Stat(cand); err == nil {
			return cand, "project", shared.CachedWorkflow{}
		}
	}
	if c, ok := shared.FindCachedWorkflow(name); ok {
		return c.Path, "cache", c
	}
	return "", "", shared.CachedWorkflow{}
}

func cachedDependencies(cw shared.CachedWorkflow) []depInfo {
	if cw.Meta == nil {
		return nil
	}
	raw, ok := cw.Meta["dependencies"].([]interface{})
	if !ok {
		return nil
	}
	out := make([]depInfo, 0, len(raw))
	for _, it := range raw {
		m, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		d := depInfo{
			GoModule:  strFromAny(m["go_module"]),
			Namespace: strFromAny(m["namespace"]),
			Class:     strFromAny(m["class"]),
			Version:   strFromAny(m["version"]),
		}
		if b, ok := m["is_local"].(bool); ok {
			d.IsLocal = b
		}
		if d.GoModule == "" && d.Namespace == "" {
			continue
		}
		out = append(out, d)
	}
	return out
}

func strFromAny(v interface{}) string {
	if s, ok := v.(string); ok {
		return strings.TrimSpace(s)
	}
	return ""
}

func metaString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	return strFromAny(m[key])
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func sourceLabel(kind string) string {
	switch kind {
	case "project":
		return "local project workflow"
	case "cache":
		return "from the ~/.kue registry cache"
	default:
		return "local file"
	}
}

// displayPath shortens an absolute path under the user's home to ~/… .
func displayPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
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
