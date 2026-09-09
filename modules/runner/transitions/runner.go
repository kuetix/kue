// Package transitions implements the `kue run` / `kue modules` / `kue
// transitions` commands — the workflow-first half of the kue CLI.
//
// kue links in the full Kuetix standard library (std-core, std-cli,
// std-auth, std-http, std-ai — see modules/modules.go), so it can execute a
// workflow directly instead of only composing one into a project. RunCommand
// is the executor; ModulesCommand and TransitionsCommand report exactly what
// action set that linked-in library makes runnable.
package transitions

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/engine/engine/workflow"
	"github.com/kuetix/kue"
	projectTransitions "github.com/kuetix/kue/modules/project/transitions"
	"github.com/kuetix/kue/modules/shared"
	. "github.com/kuetix/std-cli/modules/cli/helpers"
)

type runnerTransitions struct {
	workflow.BaseServiceTransition
}

func NewRunnerTransitions() interfaces.ServiceTransitions {
	return &runnerTransitions{}
}

// ---------------------------------------------------------------------------
// kue run — execute a workflow (local file, project dir, or registry name)
// ---------------------------------------------------------------------------

// runResult is the single normalized outcome of a workflow execution,
// modelled on github.com/kuetix/runner's `result`.
type runResult struct {
	Success    bool        `json:"success"`
	StatusCode int         `json:"status_code,omitempty"`
	Response   interface{} `json:"response,omitempty"`
	Error      string      `json:"error,omitempty"`
	DurationMs int64       `json:"duration_ms"`
}

func (rt *runnerTransitions) RunCommand(command map[string]interface{}, config map[string]interface{}, kueConfig shared.KueConfig, flagSet *flag.FlagSet, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)

	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(rt.GetSession(), config, flags)
		return
	}

	target, extraArgs := targetAndArgs(command, config, options)
	if strings.TrimSpace(target) == "" {
		r.Error = fmt.Errorf("nothing to run — usage: kue run <file.wsl | org/name | project-dir> [key=value ...]")
		return
	}

	// When the target is a bare positional the CLI resolves the command via the
	// "*" wildcard and never parses this command's flags — so also read the raw
	// option tokens off requestedCommand.
	raw := rawFlags(command)
	asJSON := boolFlag(options, "json") || raw["json"] == "true"
	checkOnly := boolFlag(options, "check") || raw["check"] == "true"
	force := boolFlag(options, "force") || raw["force"] == "true"
	noCache := boolFlag(options, "no-cache") || raw["no-cache"] == "true"
	owner, _ := options["owner"].(string)
	if owner == "" {
		owner = raw["owner"]
	}
	if h, _ := options["host"].(string); strings.TrimSpace(h) != "" {
		kueConfig.Host = h
	} else if raw["host"] != "" {
		kueConfig.Host = raw["host"]
	}

	// 1. Project directory — keep the historical `kue run .` behavior.
	if projectTransitions.IsProjectDir(target) || (target == "." && projectTransitions.IsProjectDir(mustCwd())) {
		if checkOnly {
			r.Success = true
			r.Response = "project directory — `kue run` will build and run it"
			return
		}
		out, err := projectTransitions.RunProjectDir(target)
		if err != nil {
			r.Error = fmt.Errorf("project run failed: %w\n%s", err, out)
			return
		}
		r.Success = true
		r.Response = out
		return
	}

	// 2/3. Resolve to a WSL/SWSL file on disk (local path or registry download).
	var (
		wfPath string
		meta   *remoteMeta
		err    error
	)
	if isWorkflowFile(target) {
		abs, aerr := filepath.Abs(target)
		if aerr != nil {
			r.Error = aerr
			return
		}
		if _, serr := os.Stat(abs); serr != nil {
			r.Error = fmt.Errorf("workflow file not found: %s", target)
			return
		}
		wfPath = abs
	} else {
		wfPath, meta, err = fetchRegistryWorkflow(kueConfig, target, owner, noCache)
		if err != nil {
			r.Error = err
			return
		}
	}

	// Parse + validate through the engine, and check every action it calls is
	// linked into this kue binary.
	actions, perr := rt.workflowActions(wfPath)
	if perr != nil {
		r.Error = fmt.Errorf("workflow does not validate: %w", perr)
		return
	}
	missing := unresolvedModules(actions)

	if checkOnly {
		// The check itself succeeded; runnability is reported in the body (and
		// the `runnable` field of --json output) rather than as a hard error,
		// so the report always reaches the user.
		r.Success = true
		r.Response = checkReport(target, wfPath, meta, actions, missing, asJSON)
		return
	}

	if len(missing) > 0 && !force {
		r.Error = fmt.Errorf(
			"cannot run %s: %d action module(s) not linked into this kue build: %s\n"+
				"  run `kue modules` to see what is available, or re-run with --force to try anyway",
			target, len(missing), strings.Join(missing, ", "),
		)
		return
	}

	res := rt.execWorkflow(wfPath, extraArgs)
	if asJSON {
		r.Success = res.Success
		b, _ := json.MarshalIndent(res, "", "  ")
		r.Response = string(b)
	} else {
		r.Success = res.Success
		r.Response = humanResult(res)
	}
	if !res.Success && res.Error != "" {
		r.Error = fmt.Errorf("%s", res.Error)
	}
	return
}

// execWorkflow runs one workflow file through a nested engine invocation (the
// same mechanism std-cli's WorkflowExecutor uses) from a scratch working
// directory containing the empty modules/ + workflows/ dirs the engine's
// path helpers expect.
func (rt *runnerTransitions) execWorkflow(wfPath string, args []string) runResult {
	runDir := filepath.Join(kue.HomeDir, kue.CacheDir, "run")
	_ = os.MkdirAll(filepath.Join(runDir, "modules"), 0o755)
	_ = os.MkdirAll(filepath.Join(runDir, "workflows"), 0o755)

	prevWD, _ := os.Getwd()
	if err := os.Chdir(runDir); err != nil {
		return runResult{Error: fmt.Sprintf("failed to enter run directory: %v", err)}
	}
	defer func() { _ = os.Chdir(prevWD) }()

	start := time.Now()
	responses := engine.RunWorkflow("production", &domain.Options{
		EngineName: "kue-run",
		ConfigName: "engine",
		Quiet:      true,
		Amount:     1,
		Retry:      1,
		Workflow:   wfPath,
		Args:       args,
		Context:    map[string]interface{}{},
		Config:     &domain.Config{},
	})
	out := bestResponse(responses)
	out.DurationMs = time.Since(start).Milliseconds()
	return out
}

// workflowActions parses wfPath through the engine's standard WSL/SWSL
// pipeline and returns every action it references. A parse/validation error
// surfaces here.
func (rt *runnerTransitions) workflowActions(wfPath string) ([]workflow.WorkflowAction, error) {
	eng, ok := rt.Ctx.Engine.(*workflow.Engine)
	if !ok {
		return nil, fmt.Errorf("unexpected engine type: %T", rt.Ctx.Engine)
	}
	prevWD, prevWP := eng.WorkingDir, eng.WorkflowPath
	eng.WorkingDir = filepath.Dir(wfPath)
	eng.WorkflowPath = ""
	defer func() { eng.WorkingDir, eng.WorkflowPath = prevWD, prevWP }()

	name := strings.TrimSuffix(filepath.Base(wfPath), filepath.Ext(wfPath))
	return eng.GetWorkflowActions(name)
}

// ---------------------------------------------------------------------------
// kue modules — Go modules (packages) linked into this kue binary
// ---------------------------------------------------------------------------

type moduleGroup struct {
	Module      string   `json:"module"`
	Namespaces  []string `json:"namespaces"`
	Transitions int      `json:"transitions"`
}

func (rt *runnerTransitions) ModulesCommand(command map[string]interface{}, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(rt.GetSession(), config, flags)
		return
	}
	ensureRegistered()

	byModule := map[string]map[string]int{} // goModule -> namespace/class -> method count
	for ns, classes := range boot.MetaFunctionCache {
		for cls, methods := range classes {
			for _, m := range methods {
				gm := m.GoModule
				if gm == "" {
					gm = "(unknown)"
				}
				if byModule[gm] == nil {
					byModule[gm] = map[string]int{}
				}
				byModule[gm][ns+"/"+cls]++
			}
		}
	}

	groups := make([]moduleGroup, 0, len(byModule))
	for gm, nsMap := range byModule {
		nss := make([]string, 0, len(nsMap))
		total := 0
		for ns, c := range nsMap {
			nss = append(nss, ns)
			total += c
		}
		sort.Strings(nss)
		groups = append(groups, moduleGroup{Module: gm, Namespaces: nss, Transitions: total})
	}
	sort.Slice(groups, func(i, j int) bool { return groups[i].Module < groups[j].Module })

	if b, _ := options["json"].(bool); b {
		b, _ := json.MarshalIndent(map[string]interface{}{"count": len(groups), "modules": groups}, "", "  ")
		r.Success = true
		r.Response = string(b)
		return
	}

	var sb strings.Builder
	sb.WriteString("Packages compiled into kue (workflows may call any action below):\n\n")
	for _, g := range groups {
		sb.WriteString(fmt.Sprintf("  %s  (%d transitions)\n", g.Module, g.Transitions))
		for _, ns := range g.Namespaces {
			sb.WriteString(fmt.Sprintf("    - %s\n", ns))
		}
	}
	sb.WriteString("\nRun `kue transitions` for the full method list.\n")
	r.Success = true
	r.Response = sb.String()
	return
}

// ---------------------------------------------------------------------------
// kue transitions — every action method runnable in this kue binary
// ---------------------------------------------------------------------------

type transitionInfo struct {
	Ref       string   `json:"ref"`
	GoModule  string   `json:"go_module,omitempty"`
	Args      []string `json:"args,omitempty"`
	Returns   []string `json:"returns,omitempty"`
	Signature string   `json:"signature"`
}

func (rt *runnerTransitions) TransitionsCommand(command map[string]interface{}, config map[string]interface{}, flags map[string]interface{}) (r domain.FlowStepResult) {
	options := GetFlags(flags)
	if b, _ := options["help"].(bool); b {
		r.Success = true
		r.Response = RenderHelp(rt.GetSession(), config, flags)
		return
	}
	ensureRegistered()

	filterModule, _ := options["module"].(string)
	filterModule = strings.TrimSpace(filterModule)

	list := make([]transitionInfo, 0, 256)
	for ns, classes := range boot.MetaFunctionCache {
		for cls, methods := range classes {
			for _, m := range methods {
				if filterModule != "" && !strings.Contains(m.GoModule, filterModule) && !strings.Contains(ns+"/"+cls, filterModule) {
					continue
				}
				ref := fmt.Sprintf("%s/%s.%s", ns, cls, m.Name)
				args := make([]string, 0, len(m.ArgNames))
				for i, an := range m.ArgNames {
					t := ""
					if i < len(m.ArgTypes) {
						t = m.ArgTypes[i]
					}
					args = append(args, strings.TrimSpace(an+" "+t))
				}
				list = append(list, transitionInfo{
					Ref:       ref,
					GoModule:  m.GoModule,
					Args:      args,
					Returns:   m.ReturnTypes,
					Signature: fmt.Sprintf("%s(%s)", ref, strings.Join(args, ", ")),
				})
			}
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].Ref < list[j].Ref })

	if b, _ := options["json"].(bool); b {
		b, _ := json.MarshalIndent(map[string]interface{}{"count": len(list), "transitions": list}, "", "  ")
		r.Success = true
		r.Response = string(b)
		return
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%d transitions available in this kue build", len(list)))
	if filterModule != "" {
		sb.WriteString(fmt.Sprintf(" (filtered by %q)", filterModule))
	}
	sb.WriteString(":\n\n")
	for _, t := range list {
		sb.WriteString("  " + t.Signature + "\n")
	}
	r.Success = true
	r.Response = sb.String()
	return
}

// ---------------------------------------------------------------------------
// Registry download + integrity
// ---------------------------------------------------------------------------

type remoteMeta struct {
	Name    string
	Owner   string
	Version int
	Hash    string
	Cached  bool
}

// fetchRegistryWorkflow downloads a published workflow, verifies its SHA-256,
// and caches it under ~/.kue/cache/workflows. On a cache hit (hash unchanged)
// the network round-trip for the body is still made — the registry is the
// source of truth for the current version — but the local copy is reused when
// the hashes match. --no-cache forces a fresh write.
func fetchRegistryWorkflow(kueConfig shared.KueConfig, name, owner string, noCache bool) (string, *remoteMeta, error) {
	q := url.Values{}
	q.Set("name", name)
	if owner != "" {
		q.Set("owner", owner)
	}
	body, status, err := shared.PerformOptionalAuthRequest(kueConfig, http.MethodGet, "/workflows/get?"+q.Encode(), nil)
	if err != nil {
		return "", nil, fmt.Errorf("registry fetch failed (%d): %w", status, err)
	}

	var env map[string]interface{}
	if err := json.Unmarshal([]byte(body), &env); err != nil {
		return "", nil, fmt.Errorf("registry returned unparseable response: %w", err)
	}
	data := env
	if d, ok := env["data"].(map[string]interface{}); ok {
		data = d
	}
	content, _ := data["content"].(string)
	if strings.TrimSpace(content) == "" {
		return "", nil, fmt.Errorf("registry returned no content for %q (is it published?)", name)
	}
	hash, _ := data["hash"].(string)
	ownerID, _ := data["owner"].(string)
	version := 0
	if v, ok := data["version"].(float64); ok {
		version = int(v)
	}

	sum := sha256.Sum256([]byte(content))
	got := hex.EncodeToString(sum[:])
	if hash != "" && got != hash {
		return "", nil, fmt.Errorf("integrity check failed for %q: server hash %s, computed %s", name, hash, got)
	}
	if hash == "" {
		hash = got
	}

	base := filepath.Join(kue.HomeDir, kue.CacheDir, "cache", "workflows", safeSeg(ownerID))
	if err := os.MkdirAll(filepath.Join(base, filepath.Dir(name)), 0o755); err != nil {
		return "", nil, err
	}
	cachePath := filepath.Join(base, fmt.Sprintf("%s@v%d%s", name, version, workflowExt(content)))
	meta := &remoteMeta{Name: name, Owner: ownerID, Version: version, Hash: hash}

	if !noCache {
		if existing, rerr := os.ReadFile(cachePath); rerr == nil {
			esum := sha256.Sum256(existing)
			if hex.EncodeToString(esum[:]) == hash {
				meta.Cached = true
				return cachePath, meta, nil
			}
		}
	}
	if err := os.WriteFile(cachePath, []byte(content), 0o644); err != nil {
		return "", nil, err
	}
	metaJSON, _ := json.MarshalIndent(map[string]interface{}{
		"name": name, "owner": ownerID, "version": version, "hash": hash,
		"actions": data["actions"], "dependencies": data["dependencies"],
		"fetched_at": time.Now().UTC().Format(time.RFC3339),
	}, "", "  ")
	_ = os.WriteFile(cachePath+".meta.json", metaJSON, 0o644)
	return cachePath, meta, nil
}

// CacheWorkflow fetches+verifies+caches a registry workflow without running
// it. Used by `kue install` / `kue i` when there is no project to compose
// into. Returns the local cache path.
func CacheWorkflow(kueConfig shared.KueConfig, name, owner string) (string, error) {
	path, _, err := fetchRegistryWorkflow(kueConfig, name, owner, false)
	return path, err
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

var registeredOnce bool

// ensureRegistered makes sure the DI FactoryContainer is populated so
// CanResolve / the catalog reflect the live linked-in set. modules.Enable()
// already ran in main(); this covers the container boot when a command runs
// before any nested engine invocation.
func ensureRegistered() {
	if registeredOnce {
		return
	}
	boot.DependencyInjection()
	registeredOnce = true
}

func unresolvedModules(actions []workflow.WorkflowAction) []string {
	ensureRegistered()
	seen := map[string]bool{}
	var missing []string
	for _, a := range actions {
		mod := strings.TrimSpace(a.Module)
		if mod == "" || seen[mod] {
			continue
		}
		seen[mod] = true
		if !di.CanResolve(defines.TransitionPrefix + mod) {
			missing = append(missing, mod)
		}
	}
	sort.Strings(missing)
	return missing
}

func checkReport(target, wfPath string, meta *remoteMeta, actions []workflow.WorkflowAction, missing []string, asJSON bool) string {
	mods := map[string]bool{}
	for _, a := range actions {
		if a.Module != "" {
			mods[a.Module] = true
		}
	}
	used := make([]string, 0, len(mods))
	for m := range mods {
		used = append(used, m)
	}
	sort.Strings(used)

	if asJSON {
		out := map[string]interface{}{
			"target":          target,
			"path":            wfPath,
			"runnable":        len(missing) == 0,
			"action_count":    len(actions),
			"modules_used":    used,
			"modules_missing": missing,
		}
		if meta != nil {
			out["registry"] = map[string]interface{}{"owner": meta.Owner, "version": meta.Version, "hash": meta.Hash, "cached": meta.Cached}
		}
		b, _ := json.MarshalIndent(out, "", "  ")
		return string(b)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Workflow: %s\n  file: %s\n", target, wfPath))
	if meta != nil {
		sb.WriteString(fmt.Sprintf("  registry: owner=%s version=%d hash=%s cached=%t\n", meta.Owner, meta.Version, short(meta.Hash), meta.Cached))
	}
	sb.WriteString(fmt.Sprintf("  actions: %d, calling %d module(s)\n", len(actions), len(used)))
	for _, m := range used {
		mark := "ok"
		for _, mm := range missing {
			if mm == m {
				mark = "MISSING"
				break
			}
		}
		sb.WriteString(fmt.Sprintf("    [%s] %s\n", mark, m))
	}
	if len(missing) == 0 {
		sb.WriteString("\nRunnable: yes — every action is linked into this kue build.\n")
	} else {
		sb.WriteString("\nRunnable: NO — missing modules are not compiled into kue. See `kue modules`.\n")
	}
	return sb.String()
}

// bestResponse picks a single result from the (normally one-entry) response
// map, preferring success then the lowest status code — matching the engine
// manager's own tie-break.
func bestResponse(responses map[string]*workflow.WorkerResponse) runResult {
	var chosen *workflow.WorkerResponse
	for _, resp := range responses {
		if resp == nil {
			continue
		}
		if chosen == nil ||
			(resp.IsSuccess() && !chosen.IsSuccess()) ||
			(resp.IsSuccess() == chosen.IsSuccess() && resp.StatusCode > 0 && (chosen.StatusCode == 0 || resp.StatusCode < chosen.StatusCode)) {
			chosen = resp
		}
	}
	if chosen == nil {
		return runResult{Success: false, Error: "workflow produced no response"}
	}
	out := runResult{Success: chosen.IsSuccess(), StatusCode: chosen.StatusCode, Response: chosen.Response}
	if err := chosen.GetError(); err != nil {
		out.Error = err.Error()
	}
	return out
}

func humanResult(res runResult) string {
	var sb strings.Builder
	if res.Success {
		sb.WriteString(fmt.Sprintf("OK (%dms)", res.DurationMs))
	} else {
		sb.WriteString(fmt.Sprintf("FAILED (%dms)", res.DurationMs))
	}
	if res.StatusCode > 0 {
		sb.WriteString(fmt.Sprintf(" [%d]", res.StatusCode))
	}
	sb.WriteString("\n")
	if res.Response != nil {
		if s, ok := res.Response.(string); ok {
			sb.WriteString(s + "\n")
		} else {
			b, _ := json.MarshalIndent(res.Response, "", "  ")
			sb.Write(b)
			sb.WriteString("\n")
		}
	}
	if res.Error != "" {
		sb.WriteString("error: " + res.Error + "\n")
	}
	return sb.String()
}

// targetAndArgs recovers the run target and the trailing key=value workflow
// args. `kue run <target> k=v ...` is parsed by the CLI as command
// "run.<target>" (dots and slashes in <target> survive in the raw string but
// not in the "."-split parts) with k=v landing in config["args"] — so the
// target is reconstructed from the raw requestedCommand string, not the parts.
func targetAndArgs(requested map[string]interface{}, config map[string]interface{}, options map[string]interface{}) (string, []string) {
	extra := asStringSlice(config["args"])

	if n, ok := options["name"].(string); ok && strings.TrimSpace(n) != "" {
		return strings.TrimSpace(n), extra
	}

	if requested != nil {
		full, _ := requested["command"].(string)
		main, _ := requested["main_command"].(string)
		if main != "" && strings.HasPrefix(full, main+".") {
			if t := strings.TrimPrefix(full, main+"."); strings.TrimSpace(t) != "" {
				return t, extra
			}
		}
	}

	// Fallback: a genuine positional (3+ tokens where the CLI kept the first
	// in config["args"]).
	if len(extra) > 0 {
		return extra[0], extra[1:]
	}
	return "", nil
}

// rawFlags scans the raw option tokens the CLI kept on requestedCommand
// ("options") for this command's flags. Needed because a bare positional
// target routes through the "*" wildcard, which skips this command's flag
// parsing. Recognizes both --long and -short forms.
func rawFlags(requested map[string]interface{}) map[string]string {
	out := map[string]string{}
	if requested == nil {
		return out
	}
	toks := asStringSlice(requested["options"])
	bools := map[string]string{"json": "json", "j": "json", "check": "check", "C": "check", "force": "force", "f": "force", "no-cache": "no-cache", "N": "no-cache"}
	strs := map[string]string{"owner": "owner", "O": "owner", "host": "host", "H": "host", "config": "config", "c": "config"}
	for i := 0; i < len(toks); i++ {
		key := strings.TrimLeft(toks[i], "-")
		if canon, ok := bools[key]; ok {
			out[canon] = "true"
			continue
		}
		if canon, ok := strs[key]; ok && i+1 < len(toks) {
			out[canon] = toks[i+1]
			i++
		}
	}
	return out
}

func boolFlag(options map[string]interface{}, key string) bool {
	b, _ := options[key].(bool)
	return b
}

func asStringSlice(v interface{}) []string {
	switch s := v.(type) {
	case []string:
		return append([]string(nil), s...)
	case []interface{}:
		out := make([]string, 0, len(s))
		for _, e := range s {
			if str, ok := e.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

func isWorkflowFile(s string) bool {
	ext := strings.ToLower(filepath.Ext(s))
	return ext == ".wsl" || ext == ".swsl"
}

var wslStateBlock = regexp.MustCompile(`(?m)^\s*state\s+[A-Za-z_]\w*\s*\{`)

// workflowExt guesses the on-disk extension for downloaded workflow content,
// since the registry doesn't record the source format. Every WSL workflow
// declares explicit `state X {` blocks; SWSL never does and instead uses `->`
// chaining / `<-` error binding.
func workflowExt(content string) string {
	if wslStateBlock.MatchString(content) {
		return ".wsl"
	}
	if strings.Contains(content, "->") || strings.Contains(content, "<-") {
		return ".swsl"
	}
	return ".wsl"
}

func safeSeg(s string) string {
	if s == "" {
		return "_"
	}
	return strings.NewReplacer("/", "_", "\\", "_", ":", "_").Replace(s)
}

func short(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func mustCwd() string {
	wd, _ := os.Getwd()
	return wd
}
