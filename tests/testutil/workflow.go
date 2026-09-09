package testutil

import (
	"flag"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuetix/engine"
	"github.com/kuetix/engine/engine/domain"
	"github.com/kuetix/engine/engine/workflow"
	kue "github.com/kuetix/kue"
	"github.com/kuetix/kue/modules"
)

var enableOnce sync.Once

// EnableModules turns on kue's full linked-in module set exactly once per
// process (mirrors what cmd/cli/main.go does before running any workflow).
func EnableModules() { enableOnce.Do(modules.Enable) }

// WorkflowResult is the normalized outcome of a workflow run, matching the
// shape used by the std-package integration tests.
type WorkflowResult struct {
	Name       string
	StatusCode int
	Err        error
	Response   interface{}
	Duration   time.Duration
}

// RunWorkflowFile executes a single .wsl/.swsl file on disk through the real
// engine — the same nested-invocation mechanism `kue run` uses
// (modules/runner/transitions/runner.go:execWorkflow) — from an isolated
// working directory. args are trailing `key=value` workflow arguments.
//
// It narrates the run to the test log: what ran, the outcome, timing, and
// (under `-v` or on failure) the captured engine log.
func RunWorkflowFile(t *testing.T, path string, args ...string) WorkflowResult {
	t.Helper()
	EnableModules()
	ChdirTemp(t)

	dump := captureEngineLog(t)
	invocation := "workflow file " + shortPath(path)
	if len(args) > 0 {
		invocation += " " + strings.Join(args, " ")
	}
	Step(t, "%s", invocation)

	start := time.Now()
	responses := engine.RunWorkflow("production", &domain.Options{
		EngineName: "kue-test",
		ConfigName: "engine",
		Quiet:      !testing.Verbose(),
		Amount:     1,
		Retry:      1,
		Workflow:   path,
		Args:       args,
		Context:    map[string]interface{}{},
		Config:     &domain.Config{},
		LogPath:    "stdout",
	})
	took := time.Since(start)

	res := pickBest(t, path, responses)
	res.Duration = took

	ok := res.Err == nil && (res.StatusCode == 0 || res.StatusCode < 400)
	result(t, ok, took, "%s -> status=%d name=%q", invocation, res.StatusCode, res.Name)
	if res.Err != nil {
		t.Logf("  error: %v", res.Err)
	}
	t.Logf("  response: %s", indent(preview(res.Response)))
	t.Cleanup(func() { dump(t.Failed()) })
	return res
}

func pickBest(t *testing.T, ref string, responses map[string]*workflow.WorkerResponse) WorkflowResult {
	t.Helper()
	var chosen struct {
		set        bool
		statusCode int
		err        error
		response   interface{}
		success    bool
		name       string
	}
	for n, resp := range responses {
		if resp == nil {
			continue
		}
		ok := resp.IsSuccess()
		if !chosen.set ||
			(ok && !chosen.success) ||
			(ok == chosen.success && resp.StatusCode > 0 && (chosen.statusCode == 0 || resp.StatusCode < chosen.statusCode)) {
			chosen.set = true
			chosen.success = ok
			chosen.statusCode = resp.StatusCode
			chosen.err = resp.GetError()
			chosen.response = resp.Response
			chosen.name = n
		}
	}
	if !chosen.set {
		t.Fatalf("workflow %s: engine returned no response", ref)
	}
	return WorkflowResult{Name: chosen.name, StatusCode: chosen.statusCode, Err: chosen.err, Response: chosen.response}
}

// BoolFlag / StringFlag wrap a value the way std-cli's GetFlags expects — as a
// thunk returning a pointer — so it survives the flag-extraction the command
// transitions run on their `flags` argument.
func BoolFlag(v bool) func() *bool { return func() *bool { return &v } }

func StringFlag(v string) func() *string { return func() *string { return &v } }

// CmdContext builds the engine Context that kue's @cli/* command workflows
// expect: a requestedCommand map and a config map holding the parsed flags.
// The CLI framework normally assembles this from argv; here we supply it
// directly so a command's workflow can be driven without a subprocess.
func CmdContext(command string, flags map[string]interface{}) map[string]interface{} {
	if flags == nil {
		flags = map[string]interface{}{}
	}
	// Several command transitions do an unchecked options["help"].(bool), so
	// always supply the thunk unless the caller overrode it.
	if _, ok := flags["help"]; !ok {
		flags["help"] = BoolFlag(false)
	}
	main := command
	if i := strings.IndexByte(command, '.'); i >= 0 {
		main = command[:i]
	}
	return map[string]interface{}{
		"requestedCommand": map[string]interface{}{
			"command":      command,
			"main_command": main,
			"options":      []string{},
		},
		"config": map[string]interface{}{
			"config":  map[string]interface{}{"usage": ""},
			"flags":   flags,
			"flagSet": flag.NewFlagSet("kue-test", flag.ContinueOnError),
		},
	}
}

// RunCLICommand executes an embedded @cli/* workflow (e.g. "@cli/run/modules")
// through the real engine with the given context, returning the normalized
// result. Point registry-touching commands at a MockRegistry via t.Setenv or
// the config map before calling.
func RunCLICommand(t *testing.T, ref string, ctx map[string]interface{}) WorkflowResult {
	t.Helper()
	EnableModules()
	ChdirTemp(t)

	dump := captureEngineLog(t)
	cmd := "?"
	if rc, ok := ctx["requestedCommand"].(map[string]interface{}); ok {
		cmd, _ = rc["command"].(string)
	}
	Step(t, "command workflow %s   (cmd %q)", ref, cmd)

	start := time.Now()
	responses := engine.RunWorkflow("production", &domain.Options{
		EngineName:      "kue-test",
		ConfigName:      "engine",
		Quiet:           !testing.Verbose(),
		Amount:          1,
		Retry:           1,
		Workflow:        ref,
		Config:          &domain.Config{},
		EmbedFS:         &kue.WorkflowsFS,
		EmbedFSRootPath: kue.WorkflowsFSPath,
		Context:         ctx,
		LogPath:         "stdout",
	})
	took := time.Since(start)

	var res WorkflowResult
	for name, resp := range responses {
		if resp == nil {
			continue
		}
		res = WorkflowResult{Name: name, StatusCode: resp.StatusCode, Err: resp.GetError(), Response: resp.Response, Duration: took}
		break
	}
	if res.Name == "" && res.Response == nil && res.Err == nil {
		dump(true)
		t.Fatalf("workflow %s: engine returned no response", ref)
	}

	ok := res.Err == nil && (res.StatusCode == 0 || res.StatusCode < 400)
	result(t, ok, took, "%s -> status=%d name=%q", ref, res.StatusCode, res.Name)
	if res.Err != nil {
		t.Logf("  error: %v", res.Err)
	}
	t.Logf("  response: %s", indent(preview(res.Response)))
	t.Cleanup(func() { dump(t.Failed()) })
	return res
}

func shortPath(p string) string {
	if i := strings.LastIndex(p, "/tests/"); i >= 0 {
		return p[i+1:]
	}
	return p
}
