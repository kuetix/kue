package transitions

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type addTransitions struct {
	workflow.BaseServiceTransition
	fs       map[string]*flag.FlagSet
	commands map[string]interface{}
}

func NewAddTransition() interfaces.ServiceTransitions { return &addTransitions{} }

// targetName resolves a command's target from the leading positional arg,
// falling back to --name. A command may register neither (the value is then
// absent from the map), so every read is comma-ok — a bare type assertion
// panics the transition.
func targetName(config, options map[string]interface{}) string {
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		if s := strings.TrimSpace(args[0]); s != "" {
			return s
		}
	}
	if n, ok := options["name"].(string); ok {
		return strings.TrimSpace(n)
	}
	return ""
}

// outputDir returns --output, defaulting to the current directory.
func outputDir(options map[string]interface{}) string {
	if o, ok := options["output"].(string); ok && strings.TrimSpace(o) != "" {
		return o
	}
	return "."
}

// ---------------------------------------------------------------------------
// AddModuleCommand — add a module to an existing project
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (a *addTransitions) AddModuleCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	moduleName := targetName(config, options)
	if moduleName == "" {
		r.Error = fmt.Errorf("module name is required (usage: kue add module <name>)")
		return
	}

	output := outputDir(options)
	force, _ := options["force"].(bool)

	camelName := shared.ToCamelCase(moduleName)
	pascalName := shared.ToPascalCase(moduleName)

	transitionsDir := filepath.Join(output, "modules", camelName, "transitions")
	transitionFile := filepath.Join(transitionsDir, camelName+".go")

	ok, err := shared.CheckPathExistsAndConfirm(transitionFile, force)
	if err != nil {
		r.Error = fmt.Errorf("path check failed: %w", err)
		return
	}
	if !ok {
		r.Error = fmt.Errorf("operation cancelled")
		return
	}

	data := shared.TemplateData{
		ModuleName:       camelName,
		ModuleNamePascal: pascalName,
	}

	if err := shared.WriteTemplateToFile(
		"templates/modules/transitions/module.go.tmpl",
		transitionFile,
		data, 0644,
	); err != nil {
		r.Error = fmt.Errorf("failed to create module transition file: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Module '%s' created at %s\n", moduleName, transitionsDir)
	return
}

// ---------------------------------------------------------------------------
// AddWorkflowCommand — add a workflow to an existing project
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (a *addTransitions) AddWorkflowCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	nameArg := targetName(config, options)
	if nameArg == "" {
		r.Error = fmt.Errorf("workflow name is required (usage: kue add workflow <name>)")
		return
	}

	output := outputDir(options)
	force, _ := options["force"].(bool)

	subDir, wfName := parseWorkflowArg(nameArg)

	camelName := shared.ToCamelCase(wfName)
	pascalName := shared.ToPascalCase(wfName)

	wfDir := filepath.Join(output, "workflows", subDir)
	wfFile := filepath.Join(wfDir, camelName+".wsl")

	ok, err := shared.CheckPathExistsAndConfirm(wfFile, force)
	if err != nil {
		r.Error = fmt.Errorf("path check failed: %w", err)
		return
	}
	if !ok {
		r.Error = fmt.Errorf("operation cancelled")
		return
	}

	data := shared.TemplateData{
		WorkflowName:       camelName,
		WorkflowNamePascal: pascalName,
	}

	if err := shared.WriteTemplateToFile(
		"templates/workflows/workflow.wsl.tmpl",
		wfFile,
		data, 0644,
	); err != nil {
		r.Error = fmt.Errorf("failed to create workflow file: %w", err)
		return
	}

	if err := shared.CreateWorkflowMetadata(output, subDir, wfName); err != nil {
		r.Error = fmt.Errorf("failed to create workflow metadata: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Workflow '%s' created at %s\n", wfName, wfFile)
	return
}

// ---------------------------------------------------------------------------
// AddFeatureCommand — add a feature workflow to an existing project
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (a *addTransitions) AddFeatureCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	nameArg := targetName(config, options)
	if nameArg == "" {
		r.Error = fmt.Errorf("feature name is required (usage: kue add feature <name>)")
		return
	}

	output := outputDir(options)
	force, _ := options["force"].(bool)

	camelName := shared.ToCamelCase(nameArg)
	pascalName := shared.ToPascalCase(nameArg)

	featureDir := filepath.Join(output, "workflows", "features")
	featureFile := filepath.Join(featureDir, camelName+".wsl")

	ok, err := shared.CheckPathExistsAndConfirm(featureFile, force)
	if err != nil {
		r.Error = fmt.Errorf("path check failed: %w", err)
		return
	}
	if !ok {
		r.Error = fmt.Errorf("operation cancelled")
		return
	}

	data := shared.TemplateData{
		FeatureName:       camelName,
		FeatureNamePascal: pascalName,
	}

	if err := shared.WriteTemplateToFile(
		"templates/workflows/feature.wsl.tmpl",
		featureFile,
		data, 0644,
	); err != nil {
		r.Error = fmt.Errorf("failed to create feature file: %w", err)
		return
	}

	if err := shared.CreateFeatureMetadata(output, nameArg); err != nil {
		r.Error = fmt.Errorf("failed to create feature metadata: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Feature '%s' created at %s\n", nameArg, featureFile)
	return
}

// ---------------------------------------------------------------------------
// AddSolutionCommand — add a solution workflow to an existing project
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (a *addTransitions) AddSolutionCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	nameArg := targetName(config, options)
	if nameArg == "" {
		r.Error = fmt.Errorf("solution name is required (usage: kue add solution <name>)")
		return
	}

	output := outputDir(options)
	force, _ := options["force"].(bool)

	camelName := shared.ToCamelCase(nameArg)
	pascalName := shared.ToPascalCase(nameArg)

	solutionDir := filepath.Join(output, "workflows", "solutions")
	solutionFile := filepath.Join(solutionDir, camelName+".wsl")

	ok, err := shared.CheckPathExistsAndConfirm(solutionFile, force)
	if err != nil {
		r.Error = fmt.Errorf("path check failed: %w", err)
		return
	}
	if !ok {
		r.Error = fmt.Errorf("operation cancelled")
		return
	}

	data := shared.TemplateData{
		SolutionName:       camelName,
		SolutionNamePascal: pascalName,
	}

	if err := shared.WriteTemplateToFile(
		"templates/workflows/solution.wsl.tmpl",
		solutionFile,
		data, 0644,
	); err != nil {
		r.Error = fmt.Errorf("failed to create solution file: %w", err)
		return
	}

	if err := shared.CreateSolutionMetadata(output, nameArg); err != nil {
		r.Error = fmt.Errorf("failed to create solution metadata: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Solution '%s' created at %s\n", nameArg, solutionFile)
	return
}

// ---------------------------------------------------------------------------
// AddPackageCommand — create a kuetix.json package descriptor
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (a *addTransitions) AddPackageCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	// `kue add package [name]` — the name is a positional (--name also works)
	// and is optional: with neither, fall back to the output directory's name.
	nameArg := targetName(config, options)
	output := outputDir(options)

	if nameArg == "" {
		abs, err := filepath.Abs(output)
		if err != nil {
			r.Error = fmt.Errorf("failed to resolve output directory: %w", err)
			return
		}
		nameArg = filepath.Base(abs)
	}

	force, _ := options["force"].(bool)

	if err := os.MkdirAll(output, 0o755); err != nil {
		r.Error = fmt.Errorf("failed to create output directory %s: %w", output, err)
		return
	}

	pkgFile := filepath.Join(output, "kuetix.json")

	ok, err := shared.CheckPathExistsAndConfirm(pkgFile, force)
	if err != nil {
		r.Error = fmt.Errorf("path check failed: %w", err)
		return
	}
	if !ok {
		r.Error = fmt.Errorf("operation cancelled")
		return
	}

	data := shared.TemplateData{
		ProjectName:         nameArg,
		KuetixEngineVersion: shared.KuetixEngineVersion,
	}

	tmplErr := shared.WriteTemplateToFile(
		"templates/config/kuetix.json.tmpl",
		pkgFile,
		data, 0644,
	)
	if tmplErr != nil {
		// Fall back to manual JSON creation
		pkgInfo := shared.PackageInfo{
			Name:        nameArg,
			Type:        "package",
			Description: fmt.Sprintf("Kuetix package: %s", nameArg),
			Version:     "0.1.0",
			Engine:      shared.KuetixEngineVersion,
			Publisher:   "",
			Keywords:    []string{},
		}
		jsonData, jsonErr := json.MarshalIndent(pkgInfo, "", "  ")
		if jsonErr != nil {
			r.Error = fmt.Errorf("failed to marshal package info: %w", jsonErr)
			return
		}
		if err := os.MkdirAll(filepath.Dir(pkgFile), 0755); err != nil {
			r.Error = fmt.Errorf("failed to create directory: %w", err)
			return
		}
		if err := os.WriteFile(pkgFile, jsonData, 0644); err != nil {
			r.Error = fmt.Errorf("failed to write kuetix.json: %w", err)
			return
		}
	}

	r.Success = true
	r.Response = fmt.Sprintf("Package descriptor '%s' created at %s\n", nameArg, pkgFile)
	return
}

// ---------------------------------------------------------------------------
// AddTransitionCommand — add a transition method to a module
// ---------------------------------------------------------------------------

//goland:noinspection GoUnusedParameter
func (a *addTransitions) AddTransitionCommand(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	nameArg := targetName(config, options)
	if nameArg == "" {
		r.Error = fmt.Errorf("transition name is required (usage: kue add transition <module>/<Method>)")
		return
	}

	moduleArg, _ := options["module"].(string)
	moduleArg = strings.TrimSpace(moduleArg)

	// A "<module>/<Method>" positional carries the module too.
	if moduleArg == "" {
		if slash := strings.LastIndex(nameArg, "/"); slash > 0 {
			moduleArg, nameArg = nameArg[:slash], nameArg[slash+1:]
		}
	}

	description, _ := options["description"].(string)

	output := outputDir(options)

	// Parse dot notation if present (e.g. "module.transition")
	moduleName, transitionName := parseDotNotation(nameArg, moduleArg)
	if strings.TrimSpace(moduleName) == "" {
		r.Error = fmt.Errorf("module name is required (--module, or pass <module>/<Method>)")
		return
	}

	camelModule := shared.ToCamelCase(moduleName)
	pascalModule := shared.ToPascalCase(moduleName)
	methodName := shared.ToMethodName(transitionName)

	transitionsDir := filepath.Join(output, "modules", camelModule, "transitions")
	transitionFile := filepath.Join(transitionsDir, camelModule+".go")

	// Check if the module transition file exists; if not, create the module first
	if _, err := os.Stat(transitionFile); os.IsNotExist(err) {
		data := shared.TemplateData{
			ModuleName:       camelModule,
			ModuleNamePascal: pascalModule,
		}
		if tmplErr := shared.WriteTemplateToFile(
			"templates/modules/transitions/module.go.tmpl",
			transitionFile,
			data, 0644,
		); tmplErr != nil {
			// Fall back: create a minimal module file
			if mkErr := createMinimalModuleFile(transitionFile, camelModule, pascalModule); mkErr != nil {
				r.Error = fmt.Errorf("failed to create module file: %w", mkErr)
				return
			}
		}
	}

	if err := addTransition(transitionFile, camelModule, methodName, description); err != nil {
		r.Error = fmt.Errorf("failed to add transition: %w", err)
		return
	}

	r.Success = true
	r.Response = fmt.Sprintf("Transition '%s' added to module '%s' at %s\n", methodName, moduleName, transitionFile)
	return
}

// ===========================================================================
// Unexported helper functions
// ===========================================================================

// parseWorkflowArg splits a workflow argument of the form "subdir/name" into
// its subdirectory and workflow name parts. If no separator is present, the
// subdirectory defaults to "cli".
func parseWorkflowArg(arg string) (subDir, name string) {
	arg = strings.TrimSpace(arg)
	if idx := strings.LastIndex(arg, "/"); idx >= 0 {
		return arg[:idx], arg[idx+1:]
	}
	return "cli", arg
}

// parseDotNotation handles "module.transition" notation. If the nameArg
// contains a dot, the portion before the dot is treated as the module name
// and the portion after as the transition name. Otherwise moduleArg is used
// as the module name and nameArg as the transition name.
func parseDotNotation(nameArg, moduleArg string) (moduleName, transitionName string) {
	nameArg = strings.TrimSpace(nameArg)
	if idx := strings.Index(nameArg, "."); idx >= 0 {
		return nameArg[:idx], nameArg[idx+1:]
	}
	return moduleArg, nameArg
}

// addTransition appends a new transition method to the given Go source file.
func addTransition(filePath, moduleCamel, methodName, description string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	methodCode := addTransitionCore(moduleCamel, methodName, description)

	// Check if the method already exists
	if strings.Contains(string(content), "func (") && strings.Contains(string(content), methodName+"(") {
		return fmt.Errorf("method %s already exists in %s", methodName, filePath)
	}

	updated := string(content) + "\n" + methodCode
	return os.WriteFile(filePath, []byte(updated), 0644)
}

// addTransitionCore generates the Go source code for a transition method.
func addTransitionCore(moduleCamel, methodName, description string) string {
	receiverChar := string([]rune(moduleCamel)[0])
	structName := moduleCamel + "Transitions"

	var descComment string
	if strings.TrimSpace(description) != "" {
		descComment = fmt.Sprintf("// %s — %s\n", methodName, description)
	}

	return fmt.Sprintf(`%s//goland:noinspection GoUnusedParameter
func (%s *%s) %s(command string, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(a.GetSession(), config, flags)
		return
	}

	// TODO: implement %s logic
	r.Success = true
	r.Response = "%s executed successfully\n"
	return
}
`, descComment, receiverChar, structName, methodName, methodName, methodName)
}

// createMinimalModuleFile creates a bare-bones module transition file when
// the template system is not available.
func createMinimalModuleFile(filePath, camelName, pascalName string) error {
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	receiverChar := string([]rune(camelName)[0])

	content := fmt.Sprintf(`package transitions

import (
	"bytes"
	"flag"
	"fmt"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type %sTransitions struct {
	workflow.BaseServiceTransition
	fs       map[string]*flag.FlagSet
	commands map[string]interface{}
}

func New%sTransition() interfaces.ServiceTransitions { return &%sTransitions{} }

// Ensure the receiver variable is used to satisfy the linter.
var _ = func(%s *%sTransitions) { _ = %s }
`, camelName, pascalName, camelName, receiverChar, camelName, receiverChar)

	// Suppress the unused import warning — fmt is used in addTransitionCore output.
	_ = fmt.Sprintf
	return os.WriteFile(filePath, []byte(content), 0644)
}
