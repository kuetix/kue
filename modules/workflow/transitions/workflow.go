package transitions

import (
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/helpers"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
	"github.com/schollz/progressbar/v3"
)

type workflowTransitions struct {
	workflow.BaseServiceTransition
	ListNames *shared.ListNames
}

func NewWorkflowTransition() interfaces.ServiceTransitions {
	return &workflowTransitions{ListNames: shared.NewListNames("workflow", "list")}
}

// ---------------------------------------------------------------------------
// Payload types sent to the server
// ---------------------------------------------------------------------------

type workflowDependency struct {
	GoModule   string `json:"go_module"`
	ModulePath string `json:"module_path"`
	Namespace  string `json:"namespace"`
	Class      string `json:"class"`
	// ModuleInfo is the full entry from the dependency's modules.json
	// (info + methods), so the server has labels/descriptions/signatures
	// without re-parsing source.
	ModuleInfo map[string]interface{} `json:"module_info,omitempty"`
	// ModulesJSONPath is the relative path under the project (or vendor)
	// where the source modules.json was found, for traceability.
	ModulesJSONPath string `json:"modules_json_path,omitempty"`
	// Provenance: whether the module lives in this project (replace directive /
	// own module), and, for external modules, the git source + version.
	IsLocal    bool   `json:"is_local,omitempty"`
	Repository string `json:"repository,omitempty"`
	Version    string `json:"version,omitempty"`
	Commit     string `json:"commit,omitempty"`
}

type workflowActionInfo struct {
	Workflow string                       `json:"workflow"`
	State    string                       `json:"state"`
	Module   string                       `json:"module,omitempty"`
	Name     string                       `json:"name"`
	As       string                       `json:"as,omitempty"`
	Args     []workflow.WorkflowArg       `json:"args,omitempty"`
	Params   []string                     `json:"params,omitempty"`
	Terminal string                       `json:"terminal,omitempty"`
	Metadata *interfaces.FunctionMetadata `json:"metadata,omitempty"`
}

type workflowPayload struct {
	Name         string               `json:"name"`
	Project      string               `json:"project,omitempty"`
	FilePath     string               `json:"file_path,omitempty"`
	Content      string               `json:"content"`
	Imports      map[string]string    `json:"imports,omitempty"`
	Actions      []workflowActionInfo `json:"actions"`
	Dependencies []workflowDependency `json:"dependencies"`
	Public       bool                 `json:"public,omitempty"`
}

type packageUploadPayload struct {
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Description   string                 `json:"description"`
	Version       string                 `json:"version"`
	Engine        string                 `json:"engine"`
	Publisher     string                 `json:"publisher"`
	Keywords      []string               `json:"keywords"`
	Modules       []string               `json:"modules"`
	ModuleCatalog map[string]interface{} `json:"moduleCatalog,omitempty"`
}

// uploadOptions carries CLI-supplied overrides into uploadOne/buildPayload.
type uploadOptions struct {
	ProjectName string
	Public      bool
}

// ===========================================================================
// Transition methods
// ===========================================================================

//goland:noinspection GoUnusedParameter
func (w *workflowTransitions) UploadAllCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(w.GetSession(), config, flags)
		return
	}

	method := strings.ToUpper(strings.TrimSpace(options["method"].(string)))
	if method == "" {
		method = http.MethodPost
	}
	if method != http.MethodPost && method != http.MethodPut {
		r.Error = fmt.Errorf("invalid --method %q (expected POST or PUT)", method)
		return
	}

	cwd, err := os.Getwd()
	if err != nil {
		r.Error = fmt.Errorf("failed to get working directory: %w", err)
		return
	}
	output := strings.TrimSpace(options["output"].(string))
	if output == "" || output == "." {
		output = cwd
	}
	workflowsRoot := filepath.Join(output, "workflows")
	if _, err := os.Stat(workflowsRoot); err != nil {
		r.Error = fmt.Errorf("workflows directory not found at %s: %w", workflowsRoot, err)
		return
	}

	names, err := discoverWorkflowNames(workflowsRoot)
	if err != nil {
		r.Error = fmt.Errorf("failed to enumerate workflows: %w", err)
		return
	}
	if len(names) == 0 {
		// A package may still have metadata to register even when it does not
		// contain workflows, so continue through the package upload below.
		sb := strings.Builder{}
		if packageName, packageErr := uploadPackageMetadata(output, kueConfig, method); packageErr != nil {
			sb.WriteString(fmt.Sprintf("- package FAILED: %v\n", packageErr))
		} else if packageName != "" {
			sb.WriteString(fmt.Sprintf("- package %s ok\n", packageName))
		}
		r.Success = true
		r.Response = "No workflows found to upload.\n" + sb.String()
		return
	}

	// Optional positional: a name or glob restricting which local workflows
	// are uploaded (`kue wsl upload api_server/routes`, `kue wsl upload 'api_server/*'`).
	if query := w.firstPositional(config, options); query != "" {
		filtered := matchWorkflowNames(names, query)
		if len(filtered) == 0 {
			r.Error = fmt.Errorf("no local workflows under %s match %q", workflowsRoot, query)
			return
		}
		names = filtered
	}

	uploadOpts := optsFromFlags(options)
	var sb strings.Builder
	var errs []error
	uploaded, failed, exists := 0, 0, 0
	defer func() {
		if r := recover(); r != nil {
			errs = append(errs, fmt.Errorf("panic: %v", r))
			sb.WriteString(fmt.Sprintf("Batch %s failed: %v\n", method, r))
			failed++
		}
	}()
	for _, name := range names {
		var body string
		var statusCode int
		body, statusCode, err = w.uploadOne(name, kueConfig, method, uploadOpts)
		if statusCode != http.StatusConflict {
			if err != nil {
				errs = append(errs, err)
				sb.WriteString(fmt.Sprintf("- %s FAILED: %v\n", name, err))
				failed++
				continue
			}
		} else {
			exists++
		}
		sb.WriteString(fmt.Sprintf("- %s ok\n", name))
		_ = body
		uploaded++
	}

	summary := fmt.Sprintf("Batch %s complete: %d exists %d uploaded, %d failed (of %d)\n", method, exists, uploaded, failed, len(names))
	if packageName, packageErr := uploadPackageMetadata(output, kueConfig, method); packageErr != nil {
		sb.WriteString(fmt.Sprintf("- package FAILED: %v\n", packageErr))
	} else if packageName != "" {
		sb.WriteString(fmt.Sprintf("- package %s ok\n", packageName))
	}
	r.Success = true
	r.Response = summary + sb.String()
	return
}

// uploadPackageMetadata registers the package descriptor next to the
// workflows, when the project has a kuetix.json. A first upload uses POST;
// an existing package is updated automatically after a conflict.
func uploadPackageMetadata(projectRoot string, kueConfig shared.KueConfig, method string) (string, error) {
	manifestPath := filepath.Join(projectRoot, "kuetix.json")
	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("read kuetix.json: %w", err)
	}

	var payload packageUploadPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return "", fmt.Errorf("parse kuetix.json: %w", err)
	}
	if moduleName := packageNameFromGoMod(projectRoot); moduleName != "" {
		payload.Name = moduleName
	}
	if strings.TrimSpace(payload.Name) == "" {
		return "", fmt.Errorf("package name is missing from go.mod and kuetix.json")
	}
	payload.Modules = packageGoModules(projectRoot, payload.Modules)
	payload.ModuleCatalog = packageModuleCatalog(projectRoot)

	body, status, requestErr := shared.PerformAuthenticatedRequest(kueConfig, method, "/package", payload)
	if requestErr != nil && method == http.MethodPost && status == http.StatusConflict {
		body, status, requestErr = shared.PerformAuthenticatedRequest(kueConfig, http.MethodPut, "/package", payload)
	}
	if requestErr != nil {
		return "", fmt.Errorf("upload package %q (HTTP %d): %w", payload.Name, status, requestErr)
	}
	_ = body
	return payload.Name, nil
}

// packageNameFromGoMod converts a Go module path into the registry package
// name. For example, github.com/kuetix/std-ai becomes kuetix-std-ai.
func packageNameFromGoMod(projectRoot string) string {
	data, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "module ") {
			continue
		}
		moduleName := strings.TrimSpace(strings.TrimPrefix(line, "module "))
		moduleName = strings.TrimPrefix(moduleName, "github.com/")
		return strings.ReplaceAll(moduleName, "/", "-")
	}
	return ""
}

func packageGoModules(projectRoot string, modules []string) []string {
	seen := make(map[string]struct{}, len(modules))
	result := make([]string, 0, len(modules))
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, module := range modules {
		add(module)
	}

	data, err := os.ReadFile(filepath.Join(projectRoot, "modules", "modules.json"))
	if err != nil {
		return result
	}
	var catalog map[string]json.RawMessage
	if json.Unmarshal(data, &catalog) != nil {
		return result
	}
	for _, raw := range catalog {
		var entry struct {
			Info struct {
				GoModule string `json:"go_module"`
			} `json:"info"`
		}
		if json.Unmarshal(raw, &entry) == nil {
			add(entry.Info.GoModule)
		}
	}
	return result
}

func packageModuleCatalog(projectRoot string) map[string]interface{} {
	data, err := os.ReadFile(filepath.Join(projectRoot, "modules", "modules.json"))
	if err != nil {
		return nil
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(data, &raw) != nil {
		return nil
	}
	catalog := make(map[string]interface{}, len(raw))
	for key, value := range raw {
		var entry interface{}
		if json.Unmarshal(value, &entry) == nil {
			catalog[key] = entry
		}
	}
	if len(catalog) == 0 {
		return nil
	}
	return catalog
}

//goland:noinspection GoUnusedParameter
func (w *workflowTransitions) ListCommand(command, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	if GetFlags(flags)["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(w.GetSession(), config, flags)
		return
	}

	records, err := w.ListNames.ListOfNames("*", kueConfig)
	if err != nil {
		r.Error = fmt.Errorf("failed to list workflows: %w", err)
		return
	}

	r.Success = true
	r.Response = records
	return
}

//goland:noinspection GoUnusedParameter
func (w *workflowTransitions) StatusCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(w.GetSession(), config, flags)
		return
	}

	noCheck, _ := options["no-check"].(bool)

	cwd, err := os.Getwd()
	if err != nil {
		r.Error = fmt.Errorf("failed to get working directory: %w", err)
		return
	}
	workflowsRoot := filepath.Join(cwd, "workflows")
	if _, err := os.Stat(workflowsRoot); err != nil {
		r.Error = fmt.Errorf("workflows directory not found at %s: %w", workflowsRoot, err)
		return
	}

	names, err := discoverWorkflowNames(workflowsRoot)
	if err != nil {
		r.Error = fmt.Errorf("failed to enumerate workflows: %w", err)
		return
	}

	type local struct {
		name    string
		payload workflowPayload
		err     error
	}
	locals := make([]local, 0, len(names))
	for _, name := range names {
		entry := local{name: name}
		payload, perr := w.buildPayload(name, uploadOptions{})
		if perr != nil {
			entry.err = perr
		} else {
			entry.payload = payload
		}
		locals = append(locals, entry)
	}

	var sb strings.Builder

	if noCheck {
		sb.WriteString(fmt.Sprintf("Local workflows (%d):\n\n", len(locals)))
		for _, l := range locals {
			renderEntry(&sb, l.name, "local", l.payload, l.err)
		}
		r.Success = true
		r.Response = sb.String()
		return
	}

	serverNames, err := fetchServerWorkflowNames(kueConfig)
	if err != nil {
		r.Error = fmt.Errorf("failed to fetch server workflows: %w", err)
		return
	}
	serverSet := map[string]bool{}
	for _, n := range serverNames {
		serverSet[n] = true
	}

	host := strings.TrimSpace(kueConfig.Host)
	if host == "" {
		host = shared.DefaultAPIHost
	}
	sb.WriteString(fmt.Sprintf("Workflow status (%s):\n\n", host))

	newCount, modCount, sameCount, errCount := 0, 0, 0, 0
	for _, l := range locals {
		if l.err != nil {
			renderEntry(&sb, l.name, "error: "+l.err.Error(), workflowPayload{}, l.err)
			errCount++
			continue
		}
		if !serverSet[l.name] {
			renderEntry(&sb, l.name, "new", l.payload, nil)
			newCount++
			continue
		}
		remote, rerr := fetchServerWorkflowContent(kueConfig, l.name)
		if rerr != nil {
			renderEntry(&sb, l.name, "error: "+rerr.Error(), l.payload, rerr)
			errCount++
			continue
		}
		if normalizeContent(remote) == normalizeContent(l.payload.Content) {
			renderEntry(&sb, l.name, "unchanged", l.payload, nil)
			sameCount++
		} else {
			renderEntry(&sb, l.name, "modified", l.payload, nil)
			modCount++
		}
	}

	localSet := map[string]bool{}
	for _, l := range locals {
		localSet[l.name] = true
	}
	var serverOnly []string
	for _, n := range serverNames {
		if !localSet[n] {
			serverOnly = append(serverOnly, n)
		}
	}
	if len(serverOnly) > 0 {
		sb.WriteString("\nServer-only workflows (not present locally):\n")
		for _, n := range serverOnly {
			sb.WriteString("  - " + n + "\n")
		}
	}

	sb.WriteString(fmt.Sprintf(
		"\nSummary: %d new, %d modified, %d unchanged, %d server-only, %d errors (of %d local)\n",
		newCount, modCount, sameCount, len(serverOnly), errCount, len(locals),
	))

	r.Success = true
	r.Response = sb.String()
	return
}

//goland:noinspection GoUnusedParameter
func (w *workflowTransitions) DeleteCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if options["help"].(bool) {
		r.Success = true
		r.Response = RenderHelp(w.GetSession(), config, flags)
		return
	}

	name := w.firstPositional(config, options)
	if name == "" {
		r.Error = fmt.Errorf("workflow name is required (usage: kue wsl delete <name>)")
		return
	}

	body, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodDelete, "/workflow/"+url.PathEscape(name), nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("workflow delete failed: %w", err)
		return
	}
	r.Success = true
	r.Response = body
	return
}

// ===========================================================================
// Internals
// ===========================================================================

func (w *workflowTransitions) firstPositional(config map[string]interface{}, options map[string]interface{}) string {
	if n, ok := options["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		return strings.TrimSpace(args[0])
	}
	return ""
}

func (w *workflowTransitions) resolveAuth(options map[string]interface{}) (string, shared.KueConfig, error) {
	cfgPath := shared.ResolveConfigPath(strOpt(options, "config"))
	kueConfig, err := shared.LoadKueConfig(cfgPath)
	if err != nil {
		return "", shared.KueConfig{}, fmt.Errorf("failed to read config: %w", err)
	}

	apiHost := shared.ResolveAPIHost(strOpt(options, "host"))
	if strings.TrimSpace(strOpt(options, "host")) == "" && strings.TrimSpace(kueConfig.Host) != "" {
		apiHost = kueConfig.Host
	}
	return apiHost, kueConfig, nil
}

func (w *workflowTransitions) uploadOne(name string, kueConfig shared.KueConfig, method string, opts uploadOptions) (string, int, error) {
	payload, err := w.buildPayload(name, opts)
	if err != nil {
		return "", 0, err
	}

	pathWorkflow := "/workflow"
	if method == http.MethodPut {
		pathWorkflow = "/workflow/" + url.PathEscape(name)
	}

	body, statusCode, err := shared.PerformAuthenticatedRequest(kueConfig, method, pathWorkflow, payload)
	if err != nil {
		return "", statusCode, fmt.Errorf("workflow %s %s failed: %w", method, name, err)
	}
	return body, statusCode, nil
}

// buildPayload reads the workflow source, extracts its actions via the
// engine, looks each one up in boot.MetaFunctionCache to gather package
// dependencies, and assembles the payload for the API.
func (w *workflowTransitions) buildPayload(name string, opts uploadOptions) (workflowPayload, error) {
	var payload workflowPayload
	payload.Name = name
	payload.Public = opts.Public

	eng, ok := w.Ctx.Engine.(*workflow.Engine)
	if !ok {
		return payload, fmt.Errorf("unexpected engine type: %T", w.Ctx.Engine)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return payload, fmt.Errorf("failed to get working directory: %w", err)
	}
	prevWD := eng.WorkingDir
	prevWP := eng.WorkflowPath
	eng.WorkingDir = cwd
	eng.WorkflowPath = "workflows"
	defer func() {
		eng.WorkingDir = prevWD
		eng.WorkflowPath = prevWP
	}()

	fp, err := eng.GetWorkflowFilePath(name)
	if err == nil && fp.FilePath != "" {
		payload.FilePath = fp.FilePath
		if data, rerr := os.ReadFile(fp.FilePath); rerr == nil {
			payload.Content = string(data)
		}
	}

	payload.Imports = collectImports(payload.FilePath, payload.Content)

	actions, err := eng.GetWorkflowActions(name)
	if err != nil {
		return payload, fmt.Errorf("failed to parse workflow %q: %w", name, err)
	}

	projectRoot, _ := findProjectRoot(cwd)
	var projectName string
	if override := strings.TrimSpace(opts.ProjectName); override != "" {
		projectName = override
	} else {
		goMod, gerr := helpers.GetModuleFromGoMod()
		if gerr != nil {
			projectName = filepath.Base(projectRoot)
		} else {
			projectName = goMod
		}
	}
	payload.Project = projectName
	jsonCache := map[string]map[string]interface{}{}
	replaces, requires, ownModule := parseGoModProvenance(projectRoot)

	deps := map[string]workflowDependency{}
	infos := make([]workflowActionInfo, 0, len(actions))
	for _, a := range actions {
		info := workflowActionInfo{
			Workflow: a.Workflow,
			State:    a.State,
			Module:   a.Module,
			Name:     a.Name,
			As:       a.As,
			Args:     a.Args,
			Params:   a.Params,
			Terminal: a.Terminal,
		}
		ns, cls := splitModule(a.Module)
		if ns != "" && cls != "" {
			if classes, ok := boot.MetaFunctionCache[ns]; ok {
				if methods, ok := classes[cls]; ok {
					if meta, ok := methods[a.Name]; ok {
						metaCopy := meta
						info.Metadata = &metaCopy
						key := meta.GoModule + "|" + ns + "/" + cls
						if _, seen := deps[key]; !seen && meta.GoModule != "" {
							dep := workflowDependency{
								GoModule:   meta.GoModule,
								ModulePath: meta.ModulePath,
								Namespace:  ns,
								Class:      cls,
							}
							_, replaced := replaces[meta.GoModule]
							dep.IsLocal = replaced || (ownModule != "" && (meta.GoModule == ownModule || strings.HasPrefix(meta.GoModule, ownModule+"/")))
							if !dep.IsLocal {
								dep.Version = requires[meta.GoModule]
								dep.Repository = deriveRepoURL(meta.GoModule)
								dep.Commit = pseudoVersionCommit(dep.Version)
							}
							entry, src := loadModulesJSONEntry(projectRoot, meta.GoModule, meta.ModulePath, ns, cls, jsonCache)
							if entry != nil {
								dep.ModuleInfo = entry
								dep.ModulesJSONPath = src
							} else {
								// No modules.json on disk (bare-dir upload, or the
								// module is only in the go build cache) — synthesize
								// the catalog from the transitions linked into this
								// kue binary so the published package still carries
								// every method's signature.
								dep.ModuleInfo = synthModuleInfoFromCache(meta.GoModule, meta.ModulePath, ns, cls)
							}
							deps[key] = dep
						}
					}
				}
			}
		}
		infos = append(infos, info)
	}
	payload.Actions = infos
	payload.Dependencies = make([]workflowDependency, 0, len(deps))
	for _, d := range deps {
		payload.Dependencies = append(payload.Dependencies, d)
	}
	return payload, nil
}

// parseGoModProvenance reads <projectRoot>/go.mod and returns: modules with a
// `replace` directive (treated as local), require-line versions keyed by module
// path, and the project's own module path.
func parseGoModProvenance(projectRoot string) (replaces, requires map[string]string, ownModule string) {
	replaces = map[string]string{}
	requires = map[string]string{}
	if projectRoot == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(projectRoot, "go.mod"))
	if err != nil {
		return
	}
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimSuffix(line, "//indirect")
		if i := strings.Index(line, "//"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		switch {
		case strings.HasPrefix(line, "module "):
			ownModule = strings.TrimSpace(strings.TrimPrefix(line, "module "))
		case strings.HasPrefix(line, "replace "):
			body := strings.TrimSpace(strings.TrimPrefix(line, "replace "))
			if lhs, _, ok := strings.Cut(body, "=>"); ok {
				replaces[strings.Fields(strings.TrimSpace(lhs))[0]] = strings.TrimSpace(body)
			}
		case strings.HasPrefix(line, "require "):
			fields := strings.Fields(strings.TrimPrefix(line, "require "))
			if len(fields) >= 2 {
				requires[fields[0]] = fields[1]
			}
		default:
			// inside a require ( ... ) or replace ( ... ) block
			fields := strings.Fields(line)
			if len(fields) == 2 && strings.Contains(fields[0], "/") && strings.HasPrefix(fields[1], "v") {
				requires[fields[0]] = fields[1]
			} else if len(fields) >= 3 && fields[1] == "=>" {
				replaces[fields[0]] = line
			}
		}
	}
	return
}

// deriveRepoURL turns a Go module path into its best-guess https git URL:
// the first three path segments ("github.com/acme/widgets/sub" ->
// "https://github.com/acme/widgets").
func deriveRepoURL(goModule string) string {
	parts := strings.Split(goModule, "/")
	if len(parts) < 2 || !strings.Contains(parts[0], ".") {
		return ""
	}
	n := 3
	if len(parts) < n {
		n = len(parts)
	}
	return "https://" + strings.Join(parts[:n], "/")
}

// pseudoVersionCommit extracts the trailing commit hash from a Go pseudo-version
// (v0.0.0-20260404221628-68d155712870 -> 68d155712870); "" for a tagged version.
func pseudoVersionCommit(version string) string {
	if !strings.Contains(version, "-") {
		return ""
	}
	segs := strings.Split(version, "-")
	last := segs[len(segs)-1]
	if len(last) == 12 || len(last) == 40 {
		return last
	}
	return ""
}

// findProjectRoot walks up from a starting directory looking for go.mod.
func findProjectRoot(start string) (string, bool) {
	dir := start
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// loadModulesJSONEntry resolves the dependency's modules.json file (vendored
// for third-party packages, local for the current project), parses it once
// per process via jsonCache, and returns the entry keyed by "<namespace>/<class>".
// Returns the entry and the modules.json path (relative to projectRoot when
// possible) on success.
func loadModulesJSONEntry(projectRoot, goModule, modulePath, namespace, class string, jsonCache map[string]map[string]interface{}) (map[string]interface{}, string) {
	if projectRoot == "" {
		return nil, ""
	}
	if modulePath == "" {
		modulePath = "modules"
	}

	candidates := []string{
		// vendored third-party package
		filepath.Join(projectRoot, "vendor", goModule, modulePath, "modules.json"),
		// local project (current go module)
		filepath.Join(projectRoot, modulePath, "modules.json"),
		// project-root manifest fallback (e.g. kue/modules.json)
		filepath.Join(projectRoot, "modules.json"),
	}

	key := namespace + "/" + class
	for _, path := range candidates {
		data, ok := jsonCache[path]
		if !ok {
			raw, err := os.ReadFile(path)
			if err != nil {
				jsonCache[path] = nil
				continue
			}
			parsed := map[string]interface{}{}
			if err := json.Unmarshal(raw, &parsed); err != nil {
				jsonCache[path] = nil
				continue
			}
			jsonCache[path] = parsed
			data = parsed
		}
		if data == nil {
			continue
		}
		if entry, ok := data[key].(map[string]interface{}); ok {
			rel, err := filepath.Rel(projectRoot, path)
			if err != nil {
				rel = path
			}
			return entry, filepath.ToSlash(rel)
		}
	}
	return nil, ""
}

// synthModuleInfoFromCache builds a module_info catalog for one
// namespace/class purely from boot.MetaFunctionCache — every transition method
// linked into this kue binary, with its full signature (arg/return names +
// types, file path). It has no prose descriptions (those live only in a
// generated modules.json) but is otherwise the same shape loadModulesJSONEntry
// returns, so the server's harvestPackages stores it in Package.ModuleCatalog
// and the registry package page renders a real Transitions section.
func synthModuleInfoFromCache(goModule, modulePath, namespace, class string) map[string]interface{} {
	classes, ok := boot.MetaFunctionCache[namespace]
	if !ok {
		return nil
	}
	methodsMeta, ok := classes[class]
	if !ok || len(methodsMeta) == 0 {
		return nil
	}

	names := make([]string, 0, len(methodsMeta))
	for name := range methodsMeta {
		names = append(names, name)
	}
	sort.Strings(names)

	methods := make([]map[string]interface{}, 0, len(names))
	for _, name := range names {
		m := methodsMeta[name]
		metaMap := map[string]interface{}{}
		if b, err := json.Marshal(m); err == nil {
			_ = json.Unmarshal(b, &metaMap)
		}
		methods = append(methods, map[string]interface{}{
			"value":  name,
			"label":  name,
			"names":  metaSignature(m),
			"method": metaMap,
		})
	}

	return map[string]interface{}{
		"info": map[string]interface{}{
			"go_module":   goModule,
			"module_path": modulePath,
			"namespace":   namespace,
			"class":       class,
			"label":       namespace + "/" + class,
		},
		"methods": methods,
	}
}

// metaSignature renders "Name(arg: type, ...) → ret: type, ..." for a method.
func metaSignature(m interfaces.FunctionMetadata) string {
	args := make([]string, 0, len(m.ArgNames))
	for i, n := range m.ArgNames {
		typ := "?"
		if i < len(m.ArgTypes) {
			typ = m.ArgTypes[i]
		}
		args = append(args, n+": "+typ)
	}
	head := m.Name + "(" + strings.Join(args, ", ") + ")"
	if len(m.ReturnNames) == 0 {
		return head
	}
	rets := make([]string, 0, len(m.ReturnNames))
	for i, n := range m.ReturnNames {
		typ := "?"
		if i < len(m.ReturnTypes) {
			typ = m.ReturnTypes[i]
		}
		rets = append(rets, n+": "+typ)
	}
	return head + " → " + strings.Join(rets, ", ")
}

// splitModule separates a WSL action module into (namespace, class). The
// namespace can contain slashes (e.g. "services/common/response" splits into
// namespace "services/common", class "response"), so we split on the LAST
// slash, not the first.
func splitModule(module string) (string, string) {
	idx := strings.LastIndex(module, "/")
	if idx < 0 {
		return "", ""
	}
	return module[:idx], module[idx+1:]
}

// collectImports scans a workflow source for `import X` and `extends Y`
// directives and returns a name->content map for each referenced WSL file
// reachable from the same workflows root. Best-effort: missing files are
// silently ignored so the upload still succeeds.
func collectImports(filePath, content string) map[string]string {
	if filePath == "" || content == "" {
		return nil
	}
	root := findWorkflowsRoot(filePath)
	if root == "" {
		return nil
	}
	out := map[string]string{}
	visited := map[string]bool{}
	visit := func(path, body string) {}
	visit = func(path, body string) {
		for _, line := range strings.Split(body, "\n") {
			line = strings.TrimSpace(line)
			var ref string
			switch {
			case strings.HasPrefix(line, "import "):
				ref = strings.TrimSpace(strings.TrimPrefix(line, "import "))
			case strings.HasPrefix(line, "extends "):
				ref = strings.TrimSpace(strings.TrimPrefix(line, "extends "))
			default:
				continue
			}
			ref = strings.Trim(ref, "\"")
			if ref == "" || visited[ref] {
				continue
			}
			visited[ref] = true

			// extends is relative to the file's directory; import is
			// relative to the workflows root.
			candidates := []string{
				filepath.Join(root, ref+".wsl"),
				filepath.Join(root, ref+".swsl"),
				filepath.Join(filepath.Dir(path), ref+".wsl"),
				filepath.Join(filepath.Dir(path), ref+".swsl"),
			}
			for _, c := range candidates {
				data, err := os.ReadFile(c)
				if err != nil {
					continue
				}
				out[ref] = string(data)
				visit(c, string(data))
				break
			}
		}
	}
	visit(filePath, content)
	if len(out) == 0 {
		return nil
	}
	return out
}

// findWorkflowsRoot walks up from a file path to the nearest "workflows"
// directory, returning its absolute path or "" if none is found.
func findWorkflowsRoot(filePath string) string {
	abs, err := filepath.Abs(filePath)
	if err != nil {
		return ""
	}
	dir := filepath.Dir(abs)
	for dir != "/" && dir != "." {
		if filepath.Base(dir) == "workflows" {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

// discoverWorkflowNames walks the workflows directory and returns the
// canonical name (relative path without extension) of every .wsl/.swsl
// file found.
func discoverWorkflowNames(workflowsRoot string) ([]string, error) {
	var names []string
	err := filepath.WalkDir(workflowsRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".wsl" && ext != ".swsl" {
			return nil
		}
		rel, err := filepath.Rel(workflowsRoot, path)
		if err != nil {
			return nil
		}
		name := strings.TrimSuffix(filepath.ToSlash(rel), ext)
		names = append(names, name)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return names, nil
}

// matchWorkflowNames filters workflow names by a CLI pattern:
//   - "" or "*"                 -> every name
//   - no glob metacharacter     -> exact match only
//   - "prefix/*"                -> that prefix and anything below it
//   - otherwise                 -> path.Match glob ("*" does not cross "/")
func matchWorkflowNames(names []string, pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return names
	}
	hasGlob := strings.ContainsAny(pattern, "*?[")
	prefix := ""
	if strings.HasSuffix(pattern, "/*") {
		prefix = strings.TrimSuffix(pattern, "*") // keep the trailing slash
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		switch {
		case !hasGlob:
			if n == pattern {
				out = append(out, n)
			}
		case prefix != "" && strings.HasPrefix(n, prefix):
			out = append(out, n)
		default:
			if ok, _ := path.Match(pattern, n); ok {
				out = append(out, n)
			}
		}
	}
	return out
}

// renderEntry writes a single workflow status block to sb.
func renderEntry(sb *strings.Builder, name, status string, payload workflowPayload, perr error) {
	sb.WriteString(fmt.Sprintf("- %s [%s]\n", name, status))
	if perr != nil {
		return
	}
	if len(payload.Actions) == 0 {
		sb.WriteString("    (no actions — constants/imports only)\n")
		return
	}
	if len(payload.Dependencies) == 0 {
		sb.WriteString("    deps: (none)\n")
		return
	}
	sb.WriteString("    deps:\n")
	for _, d := range payload.Dependencies {
		line := fmt.Sprintf("      - %s/%s", d.Namespace, d.Class)
		if d.GoModule != "" {
			line += "  (" + d.GoModule
			if d.ModulePath != "" {
				line += "/" + d.ModulePath
			}
			line += ")"
		}
		sb.WriteString(line + "\n")
	}
}

// fetchServerWorkflowNames calls GET /workflow and best-effort extracts a
// list of workflow names from the response. Supports several common shapes:
// a JSON array of objects with "name", an object with "workflows"/"items"/"data"
// arrays, or a flat array of strings.
func fetchServerWorkflowNames(kueConfig shared.KueConfig) ([]string, error) {
	var cursor string = ""
	var cursorDetails string = ""
	var count int = 0
	var limit = 100
	var records []string = []string{}
	var urlPath string = ""
	page := 0
	pageMax := 100
	bar := progressbar.Default(int64(pageMax))
	for count >= 0 {
		urlPath = fmt.Sprintf("/workflow?limit=%d&cursor=%s", limit, cursor)
		body, _, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, urlPath, nil)
		if err != nil {
			return nil, err
		}
		body = strings.TrimSpace(body)
		if body == "" {
			return nil, nil
		}

		var arr []interface{}
		if err = json.Unmarshal([]byte(body), &arr); err == nil {
			records = append(records, shared.NamesFromArray(arr)...)
		}

		var obj map[string]interface{}
		if err = json.Unmarshal([]byte(body), &obj); err != nil {
			return nil, fmt.Errorf("invalid workflow list JSON: %w", err)
		}

		data := obj["data"].(map[string]interface{})
		if data != nil {
			if c, ok := data["count"].(float64); ok {
				count = int(c)
			}

			if c, ok := data["cursor"].(string); ok {
				cursor = c
				var bin []byte
				bin, err = base64.StdEncoding.DecodeString(strings.TrimSpace(cursor))
				if err != nil {
					cursorDetails = ""
				} else {
					cursorDetails = string(bin)
					if cursorDetails == "" || cursorDetails == "|" {
						cursorDetails = ""
						cursor = ""
						count = -1
					}
				}
			} else {
				cursor = ""
				count = -1
			}
		}
		if cursor != "" {
			page++
			if page > pageMax {
				pageMax = bar.GetMax() + 1
				bar = progressbar.Default(int64(pageMax))
				_ = bar.Add(0)
			}
			_ = bar.Add(1)
		}

		for _, key := range []string{"workflows", "items", "data", "results"} {
			if v, ok := obj[key]; ok {
				if items, ok := v.([]interface{}); ok {
					records = append(records, shared.NamesFromArray(items)...)
				}
			}
		}
		if d, ok := obj["data"].(map[string]interface{}); ok {
			for _, key := range []string{"workflows", "items", "results"} {
				if v, ok := d[key]; ok {
					if items, ok := v.([]interface{}); ok {
						records = append(records, shared.NamesFromArray(items)...)
					}
				}
			}
		}
	}
	bar.ChangeMax(page)
	_ = bar.Finish()

	return records, nil
}

// fetchServerWorkflowContent fetches a single workflow and extracts its
// stored source content. Tolerates both raw-string and wrapped JSON shapes.
func fetchServerWorkflowContent(kueConfig shared.KueConfig, name string) (string, error) {
	body, _, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/workflow/"+url.PathEscape(name), nil)
	if err != nil {
		return "", err
	}
	body = strings.TrimSpace(body)
	if body == "" {
		return "", nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return body, nil
	}
	if s, ok := obj["content"].(string); ok {
		return s, nil
	}
	for _, key := range []string{"data", "workflow", "result"} {
		if inner, ok := obj[key].(map[string]interface{}); ok {
			if s, ok := inner["content"].(string); ok {
				return s, nil
			}
		}
	}
	return body, nil
}

// normalizeContent trims whitespace and normalizes line endings so that
// trailing-newline differences don't show up as "modified".
func normalizeContent(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimSpace(s)
}

func strOpt(options map[string]interface{}, key string) string {
	if v, ok := options[key].(string); ok {
		return v
	}
	return ""
}

func boolOpt(options map[string]interface{}, key string) bool {
	if v, ok := options[key].(bool); ok {
		return v
	}
	return false
}

func optsFromFlags(options map[string]interface{}) uploadOptions {
	return uploadOptions{
		ProjectName: strings.TrimSpace(strOpt(options, "projectName")),
		Public:      boolOpt(options, "public"),
	}
}

// jsonEncode is a small helper retained for debugging; not currently used
// at the request path because PerformAuthenticatedRequest marshals payload.
//
//goland:noinspection GoUnusedFunction
func jsonEncode(v interface{}) string {
	b, err := json.Marshal(v)
	if err != nil {
		return ""
	}
	return string(b)
}
