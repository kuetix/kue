package transitions

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type installTransitions struct {
	workflow.BaseServiceTransition
}

func NewInstallTransition() interfaces.ServiceTransitions { return &installTransitions{} }

// InstallCommand downloads a workflow (or package) from the registry, writes
// the workflow file (and any imported workflow files) into ./workflows, and
// adds each Go module dependency to the current project's go.mod. It prints a
// reminder to run `go mod tidy && go mod vendor` afterwards.
//
//goland:noinspection GoUnusedParameter
func (i *installTransitions) InstallCommand(command, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	name := firstPositional(command, config, options)
	if name == "" {
		r.Error = fmt.Errorf("name is required (usage: kue install <name>)")
		return
	}

	assetType := strings.ToLower(strings.TrimSpace(strOpt(options, "type")))
	output := strOpt(options, "output")
	if strings.TrimSpace(output) == "" {
		output = "."
	}
	force := boolOpt(options, "force")

	switch assetType {
	case "", "workflow":
		body, statusCode, werr := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/workflow/"+url.PathEscape(name), nil)
		if werr == nil {
			report, err := installWorkflowFromBody(name, body, output, force)
			r.StatusCode = statusCode
			if err != nil {
				r.Error = err
				return
			}
			r.Success = true
			r.Response = report
			return
		}
		if assetType == "workflow" || !isNotFound(statusCode) {
			r.StatusCode = statusCode
			r.Error = fmt.Errorf("workflow install failed: %w", werr)
			return
		}
		fallthrough
	case "package":
		body, statusCode, perr := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/packages/install?name="+url.QueryEscape(name), nil)
		r.StatusCode = statusCode
		if perr != nil {
			r.Error = fmt.Errorf("package install failed: %w", perr)
			return
		}
		report, err := installPackageFromBody(name, body, output)
		if err != nil {
			r.Error = err
			return
		}
		r.Success = true
		r.Response = report
		return
	default:
		r.Error = fmt.Errorf("unknown --type %q (expected 'workflow' or 'package')", assetType)
		return
	}
}

// ---------------------------------------------------------------------------
// Workflow install
// ---------------------------------------------------------------------------

func installWorkflowFromBody(name, body, output string, force bool) (string, error) {
	content, imports, deps, err := parseWorkflowResponse(body)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("workflow %q returned no content", name)
	}

	workflowsRoot := filepath.Join(output, "workflows")
	wsl := filepath.Join(workflowsRoot, filepath.FromSlash(name)+".wsl")
	if err = writeWorkflowFile(wsl, content, force); err != nil {
		return "", err
	}

	importedPaths := make([]string, 0, len(imports))
	for impName, impContent := range imports {
		if strings.TrimSpace(impName) == "" || strings.TrimSpace(impContent) == "" {
			continue
		}
		path := filepath.Join(workflowsRoot, filepath.FromSlash(impName)+".wsl")
		if err := writeWorkflowFile(path, impContent, force); err != nil {
			return "", err
		}
		importedPaths = append(importedPaths, path)
	}

	addedMods, skippedMods, modErrs := addGoModuleRequires(output, deps)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Installed workflow %q to %s\n", name, wsl))
	if len(importedPaths) > 0 {
		sb.WriteString("Imports written:\n")
		sort.Strings(importedPaths)
		for _, p := range importedPaths {
			sb.WriteString("  - " + p + "\n")
		}
	}
	writeDependencyReport(&sb, addedMods, skippedMods, modErrs)
	writeFollowupHint(&sb, len(addedMods) > 0)
	return sb.String(), nil
}

func writeWorkflowFile(path, content string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("file already exists: %s (use --force to overwrite)", path)
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", filepath.Dir(path), err)
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

// parseWorkflowResponse pulls the workflow content, imports map, and
// dependency list out of a server response body. The server is permissive
// about envelope shape — the response may be either the workflow payload
// directly, or wrapped under "data" / "workflow".
func parseWorkflowResponse(body string) (string, map[string]string, []string, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", nil, nil, fmt.Errorf("empty response body")
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return body, nil, nil, nil
	}
	payload := obj
	for _, key := range []string{"data", "workflow", "result"} {
		if inner, ok := obj[key].(map[string]interface{}); ok {
			payload = inner
			break
		}
	}

	content, _ := payload["content"].(string)
	imports := map[string]string{}
	if raw, ok := payload["imports"].(map[string]interface{}); ok {
		for k, v := range raw {
			if s, ok := v.(string); ok {
				imports[k] = s
			}
		}
	}
	deps := extractGoModules(payload["dependencies"])
	return content, imports, deps, nil
}

func extractGoModules(raw interface{}) []string {
	items, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(items))
	for _, it := range items {
		entry, ok := it.(map[string]interface{})
		if !ok {
			continue
		}
		mod, _ := entry["go_module"].(string)
		mod = strings.TrimSpace(mod)
		if mod == "" || seen[mod] {
			continue
		}
		seen[mod] = true
		out = append(out, mod)
	}
	return out
}

// ---------------------------------------------------------------------------
// Package install
// ---------------------------------------------------------------------------

func installPackageFromBody(name, body, output string) (string, error) {
	body = strings.TrimSpace(body)
	deps := extractPackageGoModules(body)
	addedMods, skippedMods, modErrs := addGoModuleRequires(output, deps)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Installed package %q\n", name))
	if len(deps) == 0 {
		sb.WriteString("Package metadata did not advertise any Go modules.\n")
		sb.WriteString("Raw response:\n")
		sb.WriteString(body + "\n")
	}
	writeDependencyReport(&sb, addedMods, skippedMods, modErrs)
	writeFollowupHint(&sb, len(addedMods) > 0)
	return sb.String(), nil
}

func extractPackageGoModules(body string) []string {
	if body == "" {
		return nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal([]byte(body), &obj); err != nil {
		return nil
	}
	payload := obj
	for _, key := range []string{"data", "package", "result"} {
		if inner, ok := obj[key].(map[string]interface{}); ok {
			payload = inner
			break
		}
	}
	seen := map[string]bool{}
	out := []string{}
	add := func(m string) {
		m = strings.TrimSpace(m)
		if m == "" || seen[m] {
			return
		}
		seen[m] = true
		out = append(out, m)
	}
	if mods, ok := payload["modules"].([]interface{}); ok {
		for _, v := range mods {
			switch t := v.(type) {
			case string:
				add(t)
			case map[string]interface{}:
				if m, ok := t["go_module"].(string); ok {
					add(m)
				}
			}
		}
	}
	if deps := extractGoModules(payload["dependencies"]); len(deps) > 0 {
		for _, d := range deps {
			add(d)
		}
	}
	return out
}

// ---------------------------------------------------------------------------
// go.mod editing
// ---------------------------------------------------------------------------

// addGoModuleRequires shells out to `go mod edit -require=<mod>@latest` for
// each module not already present in the project's go.mod, then runs
// `go get <mod>` to resolve a real version. Both calls are best-effort: if
// `go` is missing or a network call fails, the failure is reported back to
// the user without aborting the whole install.
func addGoModuleRequires(projectDir string, modules []string) (added []string, skipped []string, errs map[string]error) {
	errs = map[string]error{}
	if len(modules) == 0 {
		return
	}
	goModPath := filepath.Join(projectDir, "go.mod")
	existing, err := readExistingRequires(goModPath)
	if err != nil {
		for _, m := range modules {
			errs[m] = err
		}
		return
	}
	for _, mod := range modules {
		if existing[mod] {
			skipped = append(skipped, mod)
			continue
		}
		if err := runGoGet(projectDir, mod); err != nil {
			errs[mod] = err
			continue
		}
		added = append(added, mod)
		existing[mod] = true
	}
	return
}

func readExistingRequires(goModPath string) (map[string]bool, error) {
	data, err := os.ReadFile(goModPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", goModPath, err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "require ")
		fields := strings.Fields(trimmed)
		if len(fields) >= 1 && strings.Contains(fields[0], ".") && strings.Contains(fields[0], "/") {
			out[fields[0]] = true
		}
	}
	return out, nil
}

func runGoGet(projectDir, mod string) error {
	cmd := exec.Command("go", "get", mod)
	cmd.Dir = projectDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Reporting helpers
// ---------------------------------------------------------------------------

func writeDependencyReport(sb *strings.Builder, added, skipped []string, errs map[string]error) {
	if len(added) > 0 {
		sb.WriteString("Added go.mod requires:\n")
		sort.Strings(added)
		for _, m := range added {
			sb.WriteString("  + " + m + "\n")
		}
	}
	if len(skipped) > 0 {
		sb.WriteString("Already present in go.mod:\n")
		sort.Strings(skipped)
		for _, m := range skipped {
			sb.WriteString("  = " + m + "\n")
		}
	}
	if len(errs) > 0 {
		keys := make([]string, 0, len(errs))
		for k := range errs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString("Failed to add (run manually):\n")
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("  ! %s — %v\n", k, errs[k]))
		}
	}
}

func writeFollowupHint(sb *strings.Builder, addedAny bool) {
	if !addedAny {
		return
	}
	sb.WriteString("\nNext steps:\n")
	sb.WriteString("  go mod tidy\n")
	sb.WriteString("  go mod vendor\n")
}

// ---------------------------------------------------------------------------
// Flag/argument helpers
// ---------------------------------------------------------------------------

func firstPositional(command, config map[string]interface{}, options map[string]interface{}) string {
	if n, ok := options["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n)
	}
	if args, ok := config["args"].([]string); ok && len(args) > 0 {
		return strings.TrimSpace(args[0])
	}
	if command != nil {
		if len(command["parts"].([]string)) > 0 {
			for _, part := range command["parts"].([]string) {
				if part != "" && part != command["main_command"].(string) {
					return strings.TrimSpace(part)
				}
			}
		}
	}
	return ""
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

func isNotFound(statusCode int) bool {
	return statusCode == http.StatusNotFound
}
