package transitions

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"maps"
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
	runnerTransitions "github.com/kuetix/kue/modules/runner/transitions"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type installTransitions struct {
	workflow.BaseServiceTransition
	ListNames *shared.ListNames
}

func NewInstallTransition() interfaces.ServiceTransitions {
	return &installTransitions{ListNames: shared.NewListNames("workflow", "list")}
}

// InstallCommand downloads a workflow (or package) from the registry, writes
// the workflow file (and any imported workflow files) into ./workflows, and
// adds each Go module dependency to the current project's go.mod. It prints a
// reminder to run `go mod tidy && go mod vendor` afterwards.
//
//goland:noinspection GoUnusedParameter
func (i *installTransitions) InstallCommand(command, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(i.GetSession(), config, flags)
		return
	}

	name := i.firstPositional(command, config, options)
	if name == "" {
		r.Error = fmt.Errorf("name is required (usage: kue install <name>)")
		return
	}

	assetType := strings.ToLower(strings.TrimSpace(i.strOpt(options, "type")))
	output := i.strOpt(options, "output")
	if strings.TrimSpace(output) == "" {
		output = "."
	}
	force := i.boolOpt(options, "force")

	// Outside a Kuetix project there is nothing to compose the workflow into —
	// fetch + verify + cache it into ~/.kue instead so `kue run <name>` can use
	// it, and skip the go.mod wiring entirely.
	if assetType != "package" && !isInProject(output) {
		path, cerr := runnerTransitions.CacheWorkflow(kueConfig, name, i.strOpt(options, "owner"))
		if cerr != nil {
			r.Error = fmt.Errorf("cache workflow %q: %w", name, cerr)
			return
		}
		r.Success = true
		r.Response = fmt.Sprintf("cached %s -> %s\nrun it with:  kue run %s", name, path, name)
		return
	}

	// Workflows are user-scoped on the registry, so the workflow branch needs
	// a login; anonymous installs go straight to the (public) package path.
	if assetType == "" && shared.GetLoginToken(kueConfig) == "" {
		assetType = "package"
	}

	switch assetType {
	case "", "workflow":
		body, statusCode, werr := shared.PerformAuthenticatedRequest(kueConfig, http.MethodGet, "/workflow/"+url.PathEscape(name), nil)
		if werr == nil {
			report, err := i.installWorkflowFromBody(name, body, output, force)
			r.StatusCode = statusCode
			if err != nil {
				r.Error = err
				return
			}
			r.Success = true
			r.Response = report
			return
		}
		if assetType == "workflow" || !i.isNotFound(statusCode) {
			r.StatusCode = statusCode
			r.Error = fmt.Errorf("workflow install failed: %w", werr)
			return
		}
		fallthrough
	case "package":
		// Optional auth: anonymous installs resolve public packages only.
		body, statusCode, perr := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, "/packages/install?name="+url.QueryEscape(name), nil)
		r.StatusCode = statusCode
		if perr != nil {
			r.Error = fmt.Errorf("package install failed: %w", perr)
			return
		}
		report, err := i.installPackageFromBody(name, body, output)
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

// InstallBulkCommand downloads a workflow (or package) from the registry, writes
// the workflow file (and any imported workflow files) into ./workflows, and
// adds each Go module dependency to the current project's go.mod. It prints a
// reminder to run `go mod tidy && go mod vendor` afterwards.
//
//goland:noinspection GoUnusedParameter
func (i *installTransitions) InstallBulkCommand(command, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(i.GetSession(), config, flags)
		return
	}

	pattern := i.firstPositional(command, config, options)
	if pattern == "" {
		r.Error = fmt.Errorf("workflow name is required (usage: kue wsl get <name>)")
		return
	}

	// No project here — just fetch + verify + cache the exact workflow into
	// ~/.kue for `kue run <name>`; don't list, don't touch go.mod.
	output := i.strOpt(options, "output")
	if strings.ToLower(strings.TrimSpace(i.strOpt(options, "type"))) != "package" && !isInProject(output) {
		path, cerr := runnerTransitions.CacheWorkflow(kueConfig, pattern, i.strOpt(options, "owner"))
		if cerr != nil {
			r.Error = fmt.Errorf("cache workflow %q: %w", pattern, cerr)
			return
		}
		r.Success = true
		r.Response = fmt.Sprintf("cached %s -> %s\nrun it with:  kue run %s", pattern, path, pattern)
		return
	}

	// Anonymous users cannot list user-scoped workflows — go straight to an
	// exact-name install (public package path).
	var records []string
	if shared.GetLoginToken(kueConfig) == "" {
		records = []string{pattern}
	} else {
		var err error
		records, err = i.ListNames.ListOfNames(pattern, kueConfig)
		if err != nil {
			r.Error = fmt.Errorf("failed to list workflows: %w", err)
			return
		}
		// Nothing in the workflow list matches: treat the pattern as an exact
		// asset name and let InstallCommand resolve it (workflow first, then
		// package fallthrough).
		if len(records) == 0 {
			records = []string{pattern}
		}
	}

	var sb strings.Builder
	var firstErr error
	ok, failed := 0, 0
	for _, record := range records {
		cmd := maps.Clone(command)
		cFlags := maps.Clone(flags)
		cFlags["name"] = []string{record}
		cmd["parts"] = make([]string, 2)
		cmd["parts"].([]string)[0] = command["main_command"].(string)
		cmd["parts"].([]string)[1] = record
		rs := i.InstallCommand(cmd, config, kueConfig, flagSet, cFlags)
		if rs.Error != nil {
			failed++
			if firstErr == nil {
				firstErr = rs.Error
			}
			if len(records) > 1 {
				sb.WriteString(fmt.Sprintf("- %s FAILED: %v\n", record, rs.Error))
			}
			continue
		}
		ok++
		if s, isStr := rs.Response.(string); isStr && strings.TrimSpace(s) != "" {
			sb.WriteString(s)
			if !strings.HasSuffix(s, "\n") {
				sb.WriteString("\n")
			}
		}
	}

	if ok == 0 && firstErr != nil {
		r.Error = firstErr
		return
	}
	r.Success = true
	if len(records) > 1 {
		r.Response = fmt.Sprintf("%d installed, %d failed (of %d)\n%s", ok, failed, len(records), sb.String())
	} else {
		r.Response = sb.String()
	}
	return
}

// ---------------------------------------------------------------------------
// Internals
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Workflow install
// ---------------------------------------------------------------------------

func (i *installTransitions) installWorkflowFromBody(name, body, output string, force bool) (string, error) {
	content, imports, deps, err := i.parseWorkflowResponse(body)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", fmt.Errorf("workflow %q returned no content", name)
	}

	workflowsRoot := filepath.Join(output, "workflows")
	wsl := filepath.Join(workflowsRoot, filepath.FromSlash(name)+".wsl")
	rootStatus, err := i.writeWorkflowFile(wsl, content, force)
	if err != nil {
		return "", err
	}

	type importedFile struct {
		path   string
		status string
	}
	importedPaths := make([]importedFile, 0, len(imports))
	for impName, impContent := range imports {
		if strings.TrimSpace(impName) == "" || strings.TrimSpace(impContent) == "" {
			continue
		}
		path := filepath.Join(workflowsRoot, filepath.FromSlash(impName)+".wsl")
		st, werr := i.writeWorkflowFile(path, impContent, force)
		if werr != nil {
			return "", werr
		}
		importedPaths = append(importedPaths, importedFile{path, st})
	}

	addedMods, skippedMods, modErrs := i.addGoModuleRequires(output, deps)

	var sb strings.Builder
	verb := map[string]string{"created": "Installed", "updated": "Updated", "unchanged": "Already up to date:"}[rootStatus]
	sb.WriteString(fmt.Sprintf("%s workflow %q -> %s\n", verb, name, wsl))
	if len(importedPaths) > 0 {
		sb.WriteString("Imports:\n")
		sort.Slice(importedPaths, func(a, b int) bool { return importedPaths[a].path < importedPaths[b].path })
		for _, p := range importedPaths {
			sb.WriteString(fmt.Sprintf("  - %s (%s)\n", p.path, p.status))
		}
	}
	i.writeDependencyReport(&sb, addedMods, skippedMods, modErrs)
	i.writeFollowupHint(&sb, len(addedMods) > 0)
	return sb.String(), nil
}

// writeWorkflowFile writes content to path, refreshing an existing file in
// place (`kue get` is meant to be idempotent). It returns "created",
// "updated", or "unchanged". `force` only matters as a hint for callers/tests;
// a fetch always wins over a stale local copy.
func (i *installTransitions) writeWorkflowFile(path, content string, force bool) (string, error) {
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	status := "created"
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return "unchanged", nil
		}
		status = "updated"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", fmt.Errorf("failed to create directory %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		return "", fmt.Errorf("failed to write %s: %w", path, err)
	}
	return status, nil
}

// parseWorkflowResponse pulls the workflow content, imports map, and
// dependency list out of a server response body. The server is permissive
// about envelope shape — the response may be either the workflow payload
// directly, or wrapped under "data" / "workflow".
func (i *installTransitions) parseWorkflowResponse(body string) (string, map[string]string, []string, error) {
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
	deps := i.extractGoModules(payload["dependencies"])
	return content, imports, deps, nil
}

func (i *installTransitions) extractGoModules(raw interface{}) []string {
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

func (i *installTransitions) installPackageFromBody(name, body, output string) (string, error) {
	body = strings.TrimSpace(body)
	deps := i.extractPackageGoModules(body)
	addedMods, skippedMods, modErrs := i.addGoModuleRequires(output, deps)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Installed package %q\n", name))
	if len(deps) == 0 {
		sb.WriteString("Package metadata did not advertise any Go modules.\n")
		sb.WriteString("Raw response:\n")
		sb.WriteString(body + "\n")
	}
	i.writeDependencyReport(&sb, addedMods, skippedMods, modErrs)
	i.writeFollowupHint(&sb, len(addedMods) > 0)
	return sb.String(), nil
}

func (i *installTransitions) extractPackageGoModules(body string) []string {
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
	if deps := i.extractGoModules(payload["dependencies"]); len(deps) > 0 {
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
func (i *installTransitions) addGoModuleRequires(projectDir string, modules []string) (added []string, skipped []string, errs map[string]error) {
	errs = map[string]error{}
	if len(modules) == 0 {
		return
	}
	goModPath := filepath.Join(projectDir, "go.mod")
	existing, err := i.readExistingRequires(goModPath)
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
		if err = i.runGoGet(projectDir, mod); err != nil {
			errs[mod] = err
			continue
		}
		added = append(added, mod)
		existing[mod] = true
	}
	return
}

func (i *installTransitions) readExistingRequires(goModPath string) (map[string]bool, error) {
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

func (i *installTransitions) runGoGet(projectDir, mod string) error {
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

func (i *installTransitions) writeDependencyReport(sb *strings.Builder, added, skipped []string, errs map[string]error) {
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

func (i *installTransitions) writeFollowupHint(sb *strings.Builder, addedAny bool) {
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

// isInProject reports whether dir (or cwd for ".") is inside a Kuetix project
// worth wiring dependencies into — i.e. it has a go.mod or an application.json.
func isInProject(dir string) bool {
	if strings.TrimSpace(dir) == "" || dir == "." {
		if wd, err := os.Getwd(); err == nil {
			dir = wd
		}
	}
	for _, name := range []string{"go.mod", "application.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func (i *installTransitions) firstPositional(command, config map[string]interface{}, options map[string]interface{}) string {
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

func (i *installTransitions) strOpt(options map[string]interface{}, key string) string {
	if v, ok := options[key].(string); ok {
		return v
	}
	return ""
}

func (i *installTransitions) boolOpt(options map[string]interface{}, key string) bool {
	if v, ok := options[key].(bool); ok {
		return v
	}
	return false
}

func (i *installTransitions) isNotFound(statusCode int) bool {
	return statusCode == http.StatusNotFound
}
