package transitions

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type pkgTransitions struct {
	workflow.BaseServiceTransition
	modulesPath   string
	workflowsPath string
	version       string
	buildTime     string
	fs            map[string]*flag.FlagSet
	commands      map[string]interface{}
}

func NewPkgTransition() interfaces.ServiceTransitions { return &pkgTransitions{} }

// ---------------------------------------------------------------------------
// packagePayload is the JSON body sent to the packages API.
// ---------------------------------------------------------------------------

type packagePayload struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Engine      string   `json:"engine"`
	Publisher   string   `json:"publisher"`
	Keywords    []string `json:"keywords"`
	Modules     []string `json:"modules"`
}

// ---------------------------------------------------------------------------
// Bootstrap template
// ---------------------------------------------------------------------------

const bootstrapTmpl = `package {{.PackageName}}

import (
	di "github.com/kuetix/container"
	"{{.ImportPath}}/modules"
)

func init() {
	di.Boot()
}

//goland:noinspection GoUnusedExportedFunction
func Enable() {
	modules.Enable()
}
`

type bootstrapTemplateData struct {
	PackageName string
	ImportPath  string
}

// ===========================================================================
// Transition methods
// ===========================================================================

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageAddCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	return packageUpsertCommand(options, kueConfig, http.MethodPost)
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageUpdateCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	return packageUpsertCommand(options, kueConfig, http.MethodPut)
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageSearchCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	var query string
	if q, ok := options["query"].(string); ok {
		query = strings.TrimSpace(q)
	}
	if query == "" {
		if args, ok := config["args"].([]string); ok && len(args) > 0 {
			query = strings.TrimSpace(args[0])
		}
	}
	if query == "" {
		r.Error = fmt.Errorf("query is required (usage: kue package search <query>)")
		return
	}

	searchPath := buildPackageSearchPath(query)
	// Optional auth: anonymous search returns public packages only.
	body, statusCode, err := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, searchPath, nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("package search failed: %w", err)
		return
	}

	r.Success = true
	r.Response = body
	return
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageInstallCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	var name string
	if n, ok := options["name"].(string); ok {
		name = strings.TrimSpace(n)
	}
	if name == "" {
		if args, ok := config["args"].([]string); ok && len(args) > 0 {
			name = strings.TrimSpace(args[0])
		}
	}
	if name == "" {
		r.Error = fmt.Errorf("package name is required (usage: kue package install <name>)")
		return
	}

	installPath := buildPackageInstallPath(name)
	// Optional auth: anonymous installs resolve public packages only.
	body, statusCode, err := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, installPath, nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("package install failed: %w", err)
		return
	}

	r.Success = true
	r.Response = body
	return
}

// PackageInfoCommand backs `kue package <name>` — look up a single package on
// the registry and print its metadata. Falls back to a name search with
// suggestions when there is no exact match.
//
//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageInfoCommand(command map[string]interface{}, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		return
	}

	name := packageInfoName(command, config, options)
	if name == "" {
		r.Error = fmt.Errorf("package name is required (usage: kue package <name>  |  kue package search <query>)")
		return
	}

	// Exact lookup: the public detail endpoint (package metadata + module
	// catalog + the workflows that depend on it), falling back to the
	// install-metadata endpoint.
	body, statusCode, err := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, "/packages/detail?name="+url.QueryEscape(name), nil)
	if err != nil || statusCode < 200 || statusCode >= 300 || !packageBodyHasData(body) {
		body, statusCode, err = shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, buildPackageInstallPath(name), nil)
	}
	if err == nil && statusCode >= 200 && statusCode < 300 && packageBodyHasData(body) {
		r.StatusCode = statusCode
		r.Success = true
		var pretty bytes.Buffer
		if json.Indent(&pretty, []byte(body), "", "  ") == nil {
			r.Response = pretty.String()
		} else {
			r.Response = body
		}
		return
	}

	// No exact hit — search and suggest.
	sBody, _, sErr := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, buildPackageSearchPath(name), nil)
	names := packageNamesFromSearch(sBody)
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("No package named %q on the registry.\n", name))
	if sErr == nil && len(names) > 0 {
		sb.WriteString("Did you mean:\n")
		for _, n := range names {
			sb.WriteString("  - " + n + "\n")
		}
	} else {
		sb.WriteString("`kue package search " + name + "` found nothing either.\n")
		sb.WriteString("(Registry package names are the Go module path with the host dropped and '/'→'-',\n")
		sb.WriteString(" e.g. github.com/kuetix/std-core -> kuetix-std-core. Deps behind a `replace`\n")
		sb.WriteString(" directive are never published.)\n")
	}
	r.Success = true
	r.Response = sb.String()
	return
}

// packageInfoName resolves the package name for `kue package <name>`: an
// explicit --name, a positional arg, or — when the CLI parsed <name> as the
// subcommand token (`package.<name>`) — the tail of the requested command.
func packageInfoName(command, config, options map[string]interface{}) string {
	if n, ok := options["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		return strings.TrimSpace(args[0])
	}
	full, _ := command["command"].(string)
	main, _ := command["main_command"].(string)
	if main != "" && strings.HasPrefix(full, main+".") {
		if t := strings.TrimSpace(strings.TrimPrefix(full, main+".")); t != "" && t != "*" {
			return t
		}
	}
	return ""
}

func packageBodyHasData(body string) bool {
	body = strings.TrimSpace(body)
	if body == "" {
		return false
	}
	var obj map[string]interface{}
	if json.Unmarshal([]byte(body), &obj) != nil {
		return body != ""
	}
	if s, ok := obj["success"].(bool); ok && !s {
		return false
	}
	if d, ok := obj["data"].(map[string]interface{}); ok {
		if len(d) == 0 {
			return false
		}
		// A wrapped-but-empty package payload.
		if n, ok := d["name"].(string); ok && strings.TrimSpace(n) == "" {
			return false
		}
		// /packages/detail shape: {package, catalog, workflows} — real only
		// when `package` resolved to an object.
		if pkgVal, present := d["package"]; present {
			if _, isObj := pkgVal.(map[string]interface{}); !isObj {
				return false
			}
		}
	}
	if d, ok := obj["data"].([]interface{}); ok {
		return len(d) > 0
	}
	return true
}

func packageNamesFromSearch(body string) []string {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil
	}
	var obj struct {
		Data struct {
			Packages []struct {
				Name string `json:"name"`
			} `json:"packages"`
		} `json:"data"`
	}
	if json.Unmarshal([]byte(body), &obj) != nil {
		return nil
	}
	out := make([]string, 0, len(obj.Data.Packages))
	for _, p := range obj.Data.Packages {
		if strings.TrimSpace(p.Name) != "" {
			out = append(out, p.Name)
		}
	}
	return out
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageListCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	body, statusCode, err := performPackageAuthRequest(kueConfig, http.MethodGet, "/package", nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("package list failed: %w", err)
		return
	}

	r.Success = true
	r.Response = body
	return
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageListLocalCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	output := strings.TrimSpace(options["output"].(string))
	modulesFile := filepath.Join(output, "modules", "modules.go")
	if output == "" {
		modulesFile = filepath.Join("modules", "modules.go")
	}

	enabled, err := listEnabledModules(modulesFile)
	if err != nil {
		r.Error = fmt.Errorf("failed to list local packages: %w", err)
		return
	}

	if len(enabled) == 0 {
		r.Success = true
		r.Response = "No enabled modules found in " + modulesFile + "\n"
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Enabled modules in %s:\n", modulesFile))
	for _, mod := range enabled {
		sb.WriteString(fmt.Sprintf("  - %s\n", mod))
	}

	r.Success = true
	r.Response = sb.String()
	return
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageEnableCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	name := strings.TrimSpace(options["name"].(string))
	if name == "" {
		r.Error = fmt.Errorf("--name is required")
		return
	}

	output := strings.TrimSpace(options["output"].(string))
	if output == "" {
		output = "."
	}

	result, err := enableModule(name, output)
	if err != nil {
		r.Error = fmt.Errorf("failed to enable module: %w", err)
		return
	}

	r.Success = true
	r.Response = result
	return
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackagePublishCommand(command string, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	result, err := publishPackage(options, kueConfig)
	if err != nil {
		r.Error = fmt.Errorf("publish failed: %w", err)
		return
	}

	r.Success = true
	r.Response = result
	return
}

//goland:noinspection GoUnusedParameter
func (p *pkgTransitions) PackageShowCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	pathArg := strings.TrimSpace(options["path"].(string))
	result, err := showPackageInfo(pathArg)
	if err != nil {
		r.Error = fmt.Errorf("show failed: %w", err)
		return
	}

	r.Success = true
	r.Response = result
	return
}

// ===========================================================================
// Unexported helpers
// ===========================================================================

// findPackageDir locates the directory containing kuetix.json starting from
// the given path and walking up to the filesystem root.
func findPackageDir(startPath string) (string, error) {
	dir := startPath
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(abs, "kuetix.json")
		if _, err := os.Stat(candidate); err == nil {
			return abs, nil
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}
	return "", fmt.Errorf("kuetix.json not found starting from '%s'", startPath)
}

// readGoModuleName reads the module name from go.mod in the given directory.
func readGoModuleName(dir string) (string, error) {
	goModPath := filepath.Join(dir, "go.mod")
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return "", fmt.Errorf("failed to read go.mod: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "module")), nil
		}
	}
	return "", fmt.Errorf("module directive not found in go.mod")
}

// packageUpsertCommand handles both POST (add) and PUT (update) for packages.
func packageUpsertCommand(options map[string]interface{}, kueConfig shared.KueConfig, method string) (r domain.FlowStepResult) {
	pathArg := strings.TrimSpace(options["path"].(string))

	pkgDir, err := findPackageDir(pathArg)
	if err != nil {
		r.Error = fmt.Errorf("failed to find package directory: %w", err)
		return
	}

	payload, err := buildPackagePayload(pkgDir)
	if err != nil {
		r.Error = fmt.Errorf("failed to build package payload: %w", err)
		return
	}

	body, statusCode, err := performPackageAuthJSONRequest(kueConfig, method, "/package", payload)
	r.StatusCode = statusCode
	if err != nil {
		action := "add"
		if method == http.MethodPut {
			action = "update"
		}
		r.Error = fmt.Errorf("package %s failed: %w", action, err)
		return
	}

	r.Success = true
	r.Response = body
	return
}

// buildPackagePayload reads kuetix.json and modules.json from the given
// directory and constructs a packagePayload.
func buildPackagePayload(pkgDir string) (packagePayload, error) {
	var payload packagePayload

	kuetixPath := filepath.Join(pkgDir, "kuetix.json")
	data, err := os.ReadFile(kuetixPath)
	if err != nil {
		return payload, fmt.Errorf("failed to read kuetix.json: %w", err)
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return payload, fmt.Errorf("failed to parse kuetix.json: %w", err)
	}

	// Packages generated by kueinit keep the module catalog at
	// modules/modules.json. Keep supporting the older root-level modules.json
	// format as well. The registry needs the Go module paths, not the complete
	// catalog object.
	for _, modulesPath := range []string{
		filepath.Join(pkgDir, "modules.json"),
		filepath.Join(pkgDir, "modules", "modules.json"),
	} {
		modulesData, readErr := os.ReadFile(modulesPath)
		if readErr != nil {
			continue
		}

		var modules []string
		if err := json.Unmarshal(modulesData, &modules); err == nil {
			payload.Modules = uniqueStrings(modules)
			break
		}

		var catalog map[string]json.RawMessage
		if err := json.Unmarshal(modulesData, &catalog); err != nil {
			continue
		}
		for _, raw := range catalog {
			var entry struct {
				Info struct {
					GoModule string `json:"go_module"`
				} `json:"info"`
			}
			if err := json.Unmarshal(raw, &entry); err == nil && entry.Info.GoModule != "" {
				payload.Modules = append(payload.Modules, entry.Info.GoModule)
			}
		}
		payload.Modules = uniqueStrings(payload.Modules)
		break
	}

	return payload, nil
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

// performPackageAuthRequest performs an authenticated HTTP request and returns
// the response body as a string.
func performPackageAuthRequest(cfg shared.KueConfig, method, path string, payload interface{}) (string, int, error) {
	return shared.PerformAuthenticatedRequest(cfg, method, path, payload)
}

// performPackageAuthJSONRequest marshals payload to JSON and performs an
// authenticated HTTP request, returning the response body.
func performPackageAuthJSONRequest(cfg shared.KueConfig, method, path string, payload interface{}) (string, int, error) {
	return shared.PerformAuthenticatedRequest(cfg, method, path, payload)
}

// buildPackageSearchPath constructs the search endpoint path with query params.
func buildPackageSearchPath(query string) string {
	return "/packages/search?q=" + url.QueryEscape(query)
}

// buildPackageInstallPath constructs the install endpoint path with query params.
func buildPackageInstallPath(name string) string {
	return "/packages/install?name=" + url.QueryEscape(name)
}

// enableModule generates a bootstrap shim file and updates modules/modules.go.
func enableModule(name, outputDir string) (string, error) {
	var sb strings.Builder

	// Generate bootstrap file
	bootstrapPath, err := generateBootstrapFile(name, outputDir)
	if err != nil {
		return "", err
	}
	sb.WriteString(fmt.Sprintf("Generated bootstrap file: %s\n", bootstrapPath))

	// Update modules/modules.go
	modulesFile := filepath.Join(outputDir, "modules", "modules.go")
	if err := addEnableCallToModules(name, modulesFile); err != nil {
		return "", err
	}
	sb.WriteString(fmt.Sprintf("Updated %s with Enable() call for '%s'\n", modulesFile, name))

	return sb.String(), nil
}

// generateBootstrapFile creates a bootstrap shim Go file for the named module.
func generateBootstrapFile(name, outputDir string) (string, error) {
	pkgName := lastPathSegment(name)
	if isMajorVersionSuffix(pkgName) {
		parts := strings.Split(name, "/")
		if len(parts) >= 2 {
			pkgName = parts[len(parts)-2]
		}
	}

	data := bootstrapTemplateData{
		PackageName: pkgName,
		ImportPath:  name,
	}

	tmpl, err := template.New("bootstrap").Parse(bootstrapTmpl)
	if err != nil {
		return "", fmt.Errorf("failed to parse bootstrap template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to execute bootstrap template: %w", err)
	}

	bootstrapDir := filepath.Join(outputDir, "modules", pkgName)
	if err := os.MkdirAll(bootstrapDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create bootstrap directory: %w", err)
	}

	bootstrapFile := filepath.Join(bootstrapDir, "bootstrap.go")
	if err := os.WriteFile(bootstrapFile, buf.Bytes(), 0644); err != nil {
		return "", fmt.Errorf("failed to write bootstrap file: %w", err)
	}

	return bootstrapFile, nil
}

// addEnableCallToModules adds an import and Enable() call for the named module
// into the modules/modules.go file.
func addEnableCallToModules(name, modulesFile string) error {
	data, err := os.ReadFile(modulesFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", modulesFile, err)
	}

	content := string(data)
	pkgName := lastPathSegment(name)
	if isMajorVersionSuffix(pkgName) {
		parts := strings.Split(name, "/")
		if len(parts) >= 2 {
			pkgName = parts[len(parts)-2]
		}
	}

	// Read go.mod to get the module path for constructing the import
	dir := filepath.Dir(filepath.Dir(modulesFile))
	modName, err := readGoModuleName(dir)
	if err != nil {
		return err
	}

	importPath := modName + "/modules/" + pkgName
	enableCall := "\t" + pkgName + ".Enable()"

	// Check if already enabled
	if strings.Contains(content, importPath) {
		return nil
	}

	// Add import
	importLine := fmt.Sprintf("\t\"%s\"", importPath)
	if strings.Contains(content, "import (") {
		content = strings.Replace(content, "import (", "import (\n"+importLine, 1)
	} else {
		// Insert import block before func init
		content = strings.Replace(content, "func init()", fmt.Sprintf("import (\n%s\n)\n\nfunc init()", importLine), 1)
	}

	// Add Enable() call inside the Enable function
	enableFuncRe := regexp.MustCompile(`(?m)(func Enable\(\)\s*\{)`)
	if enableFuncRe.MatchString(content) {
		content = enableFuncRe.ReplaceAllString(content, "${1}\n"+enableCall)
	}

	return os.WriteFile(modulesFile, []byte(content), 0644)
}

// listEnabledModules reads modules/modules.go and returns import paths of
// enabled modules (those whose Enable() is called).
func listEnabledModules(modulesFile string) ([]string, error) {
	f, err := os.Open(modulesFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var imports []string
	scanner := bufio.NewScanner(f)
	inImport := false
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "import (" {
			inImport = true
			continue
		}
		if inImport {
			if line == ")" {
				inImport = false
				continue
			}
			// Extract import path from quoted string
			cleaned := strings.Trim(line, "\" \t")
			if cleaned != "" && !strings.HasPrefix(cleaned, "//") {
				// Remove alias if present
				parts := strings.Fields(line)
				for _, part := range parts {
					part = strings.Trim(part, "\"")
					if strings.Contains(part, "/") {
						imports = append(imports, part)
						break
					}
				}
			}
		}
	}

	return imports, scanner.Err()
}

// lastPathSegment returns the last segment of a slash-separated path.
func lastPathSegment(path string) string {
	path = strings.TrimRight(path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		return path[idx+1:]
	}
	return path
}

// isMajorVersionSuffix returns true if the segment looks like a Go major
// version suffix (e.g. "v2", "v3").
func isMajorVersionSuffix(segment string) bool {
	if len(segment) < 2 {
		return false
	}
	if segment[0] != 'v' {
		return false
	}
	for _, c := range segment[1:] {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

// resolvePublishDir determines the directory from which to publish. If name is
// provided and maps to a subdirectory, use that; otherwise use current dir.
func resolvePublishDir(name string) string {
	if strings.TrimSpace(name) == "" {
		return "."
	}
	if info, err := os.Stat(name); err == nil && info.IsDir() {
		return name
	}
	return "."
}

// publishPackage updates kuetix.json with metadata from flags and returns
// instructions for releasing.
func publishPackage(options map[string]interface{}, kueConfig shared.KueConfig) (string, error) {
	strOpt := func(key string) string {
		if v, ok := options[key].(string); ok {
			return strings.TrimSpace(v)
		}
		return ""
	}
	name := strOpt("name")
	description := strOpt("description")
	version := strOpt("version")
	publisher := strOpt("publisher")
	keywords := strOpt("keywords")
	output := strOpt("output")

	pkgDir := resolvePublishDir(output)

	kuetixPath := filepath.Join(pkgDir, "kuetix.json")
	var info shared.PackageInfo

	if data, err := os.ReadFile(kuetixPath); err == nil {
		_ = json.Unmarshal(data, &info)
	}

	if name != "" {
		info.Name = name
	}
	if description != "" {
		info.Description = description
	}
	if version != "" {
		info.Version = version
	}
	if publisher != "" {
		info.Publisher = publisher
	}
	if keywords != "" {
		kw := strings.Split(keywords, ",")
		trimmed := make([]string, 0, len(kw))
		for _, k := range kw {
			k = strings.TrimSpace(k)
			if k != "" {
				trimmed = append(trimmed, k)
			}
		}
		info.Keywords = trimmed
	}

	if info.Name == "" {
		return "", fmt.Errorf("package name is required (set via --name or in kuetix.json)")
	}

	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal kuetix.json: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(filepath.Dir(kuetixPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}
	if err := os.WriteFile(kuetixPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write kuetix.json: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Updated %s\n", kuetixPath))
	sb.WriteString(fmt.Sprintf("  Name:        %s\n", info.Name))
	sb.WriteString(fmt.Sprintf("  Version:     %s\n", info.Version))
	sb.WriteString(fmt.Sprintf("  Description: %s\n", info.Description))
	sb.WriteString(fmt.Sprintf("  Publisher:   %s\n", info.Publisher))
	if len(info.Keywords) > 0 {
		sb.WriteString(fmt.Sprintf("  Keywords:    %s\n", strings.Join(info.Keywords, ", ")))
	}
	// When logged in, also flip the published flag on the registry so the
	// package becomes publicly visible (search / pkg.kuetix.com).
	if shared.GetLoginToken(kueConfig) != "" && info.Version != "" {
		payload := map[string]string{"name": info.Name, "version": info.Version}
		body, _, err := shared.PerformAuthenticatedRequest(kueConfig, http.MethodPost, "/package/publish", payload)
		if err != nil {
			sb.WriteString(fmt.Sprintf("\nRegistry publish failed: %v\n", err))
			sb.WriteString("If the package is not registered yet, run: kue package add\n")
		} else {
			sb.WriteString(fmt.Sprintf("\nPublished %s@%s on the registry (now publicly visible).\n", info.Name, info.Version))
			_ = body
		}
	} else {
		sb.WriteString("\nTo make this package publicly visible on the registry:\n")
		sb.WriteString("  1. kue login\n")
		sb.WriteString("  2. kue package add   (first time only)\n")
		sb.WriteString("  3. kue package publish\n")
	}

	return sb.String(), nil
}

// showPackageInfo reads kuetix.json and returns a formatted summary.
func showPackageInfo(pathArg string) (string, error) {
	target := strings.TrimSpace(pathArg)
	if target == "" {
		target = "."
	}

	kuetixPath, err := shared.ResolvePackageJSONPath(target)
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(kuetixPath)
	if err != nil {
		return "", fmt.Errorf("failed to read %s: %w", kuetixPath, err)
	}

	var info shared.PackageInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return "", fmt.Errorf("failed to parse %s: %w", kuetixPath, err)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Package: %s\n", info.Name))
	if info.Type != "" {
		sb.WriteString(fmt.Sprintf("  Type:        %s\n", info.Type))
	}
	if info.Description != "" {
		sb.WriteString(fmt.Sprintf("  Description: %s\n", info.Description))
	}
	if info.Version != "" {
		sb.WriteString(fmt.Sprintf("  Version:     %s\n", info.Version))
	}
	if info.Engine != "" {
		sb.WriteString(fmt.Sprintf("  Engine:      %s\n", info.Engine))
	}
	if info.Publisher != "" {
		sb.WriteString(fmt.Sprintf("  Publisher:   %s\n", info.Publisher))
	}
	if len(info.Keywords) > 0 {
		sb.WriteString(fmt.Sprintf("  Keywords:    %s\n", strings.Join(info.Keywords, ", ")))
	}
	sb.WriteString(fmt.Sprintf("  Path:        %s\n", kuetixPath))

	return sb.String(), nil
}
