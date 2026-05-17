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

	query := strings.TrimSpace(options["query"].(string))
	if query == "" {
		r.Error = fmt.Errorf("--query is required")
		return
	}

	searchPath := buildPackageSearchPath(query)
	body, statusCode, err := performPackageAuthRequest(kueConfig, http.MethodGet, searchPath, nil)
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

	name := strings.TrimSpace(options["name"].(string))
	if name == "" {
		r.Error = fmt.Errorf("--name is required")
		return
	}

	installPath := buildPackageInstallPath(name)
	body, statusCode, err := performPackageAuthRequest(kueConfig, http.MethodGet, installPath, nil)
	r.StatusCode = statusCode
	if err != nil {
		r.Error = fmt.Errorf("package install failed: %w", err)
		return
	}

	r.Success = true
	r.Response = body
	return
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
func (p *pkgTransitions) PackagePublishCommand(command string, config map[string]interface{}, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	var helpText string
	options := GetFlags(flags)

	if options["help"].(bool) {
		helpText = GetUsage(p.Ctx.Engine.GetApplication(), config["usage"].(string), flagSet, p.Ctx.WorkflowContext.Value("workflowsPath").(string))
		r.Success = true
		r.Response = helpText
		return
	}

	result, err := publishPackage(options)
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

	modulesPath := filepath.Join(pkgDir, "modules.json")
	if modulesData, err := os.ReadFile(modulesPath); err == nil {
		var modules []string
		if err := json.Unmarshal(modulesData, &modules); err == nil {
			payload.Modules = modules
		}
	}

	return payload, nil
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
func publishPackage(options map[string]interface{}) (string, error) {
	name := strings.TrimSpace(options["name"].(string))
	description := strings.TrimSpace(options["description"].(string))
	version := strings.TrimSpace(options["version"].(string))
	publisher := strings.TrimSpace(options["publisher"].(string))
	keywords := strings.TrimSpace(options["keywords"].(string))
	output := strings.TrimSpace(options["output"].(string))

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
	sb.WriteString("\nTo release this package:\n")
	sb.WriteString("  1. Commit and push your changes\n")
	sb.WriteString(fmt.Sprintf("  2. Tag the release: git tag v%s\n", info.Version))
	sb.WriteString("  3. Push the tag: git push origin --tags\n")
	sb.WriteString("  4. Register with the registry: kue package add\n")

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
