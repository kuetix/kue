package transitions

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuetix/engine/engine/workflow"
)

func TestIsWorkflowFile(t *testing.T) {
	for _, s := range []string{"x.wsl", "a/b.WSL", "flow.swsl", "deep/nested/report.SWSL"} {
		if !isWorkflowFile(s) {
			t.Errorf("%q should be a workflow file", s)
		}
	}
	for _, s := range []string{"x.txt", "acme/name", "noext", "x.wslx"} {
		if isWorkflowFile(s) {
			t.Errorf("%q should not be a workflow file", s)
		}
	}
}

func TestWorkflowExt(t *testing.T) {
	wsl := "module x\nworkflow x {\n  start: A\n  state A {\n    end ok\n  }\n}\n"
	if got := workflowExt(wsl); got != ".wsl" {
		t.Errorf("explicit state block -> .wsl, got %q", got)
	}
	swsl := "import services/common\nfoo/bar.Baz() as r <- eh -> .\n"
	if got := workflowExt(swsl); got != ".swsl" {
		t.Errorf("chaining -> .swsl, got %q", got)
	}
	if got := workflowExt("just some text"); got != ".wsl" {
		t.Errorf("ambiguous -> .wsl default, got %q", got)
	}
}

func TestSafeSegAndShort(t *testing.T) {
	if got := safeSeg(""); got != "_" {
		t.Errorf("empty -> _, got %q", got)
	}
	if got := safeSeg("a/b:c\\d"); got != "a_b_c_d" {
		t.Errorf("got %q", got)
	}
	if got := short("abcdef"); got != "abcdef" {
		t.Errorf("short string unchanged, got %q", got)
	}
	if got := short("0123456789abcdef"); got != "0123456789ab" {
		t.Errorf("long string truncated to 12, got %q", got)
	}
}

func TestAsStringSlice(t *testing.T) {
	if got := asStringSlice([]string{"a", "b"}); len(got) != 2 || got[0] != "a" {
		t.Errorf("[]string: %#v", got)
	}
	if got := asStringSlice([]interface{}{"a", 1, "b"}); len(got) != 2 || got[1] != "b" {
		t.Errorf("[]interface{}: %#v", got)
	}
	if got := asStringSlice(42); got != nil {
		t.Errorf("unsupported -> nil, got %#v", got)
	}
}

func TestBoolFlag(t *testing.T) {
	opts := map[string]interface{}{"json": true, "check": "yes"}
	if !boolFlag(opts, "json") {
		t.Error("json should be true")
	}
	if boolFlag(opts, "check") {
		t.Error("non-bool value -> false")
	}
	if boolFlag(opts, "absent") {
		t.Error("absent -> false")
	}
}

func TestTargetAndArgs(t *testing.T) {
	// --name flag wins.
	target, extra := targetAndArgs(nil, map[string]interface{}{"args": []string{"k=v"}}, map[string]interface{}{"name": " acme/flow "})
	if target != "acme/flow" || len(extra) != 1 {
		t.Fatalf("name flag: target=%q extra=%#v", target, extra)
	}

	// Reconstructed from requestedCommand: command "run.report.wsl", main "run".
	target, extra = targetAndArgs(
		map[string]interface{}{"command": "run.flows/report.wsl", "main_command": "run"},
		map[string]interface{}{"args": []interface{}{"env=prod"}},
		map[string]interface{}{},
	)
	if target != "flows/report.wsl" || len(extra) != 1 || extra[0] != "env=prod" {
		t.Fatalf("reconstruct: target=%q extra=%#v", target, extra)
	}

	// Fallback: first positional in args.
	target, extra = targetAndArgs(nil, map[string]interface{}{"args": []string{"acme/x", "a=1"}}, map[string]interface{}{})
	if target != "acme/x" || len(extra) != 1 || extra[0] != "a=1" {
		t.Fatalf("fallback: target=%q extra=%#v", target, extra)
	}

	// Nothing to run.
	if target, _ := targetAndArgs(nil, map[string]interface{}{}, map[string]interface{}{}); target != "" {
		t.Fatalf("empty: target=%q", target)
	}
}

func TestRawFlags(t *testing.T) {
	req := map[string]interface{}{
		"options": []string{"--json", "-C", "--owner", "acme", "--host", "h.test", "unrelated"},
	}
	got := rawFlags(req)
	if got["json"] != "true" || got["check"] != "true" {
		t.Fatalf("bool flags: %#v", got)
	}
	if got["owner"] != "acme" || got["host"] != "h.test" {
		t.Fatalf("string flags: %#v", got)
	}
	if _, ok := got["unrelated"]; ok {
		t.Fatalf("stray token captured: %#v", got)
	}
	if len(rawFlags(nil)) != 0 {
		t.Fatal("nil -> empty map")
	}
}

func TestCheckReport(t *testing.T) {
	actions := []workflow.WorkflowAction{
		{Module: "services/common", Name: "assert"},
		{Module: "redis", Name: "string"},
		{Module: "services/common", Name: "response"},
	}

	human := checkReport("acme/x", "/tmp/x.wsl", nil, actions, nil, false)
	if !strings.Contains(human, "Runnable: yes") || !strings.Contains(human, "[ok] redis") {
		t.Fatalf("human (no missing):\n%s", human)
	}

	human = checkReport("acme/x", "/tmp/x.wsl", nil, actions, []string{"redis"}, false)
	if !strings.Contains(human, "Runnable: NO") || !strings.Contains(human, "[MISSING] redis") {
		t.Fatalf("human (missing):\n%s", human)
	}

	js := checkReport("acme/x", "/tmp/x.wsl", &remoteMeta{Owner: "acme", Version: 3, Hash: "abcdef"}, actions, []string{"redis"}, true)
	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(js), &parsed); err != nil {
		t.Fatalf("json: %v\n%s", err, js)
	}
	if parsed["runnable"] != false {
		t.Fatalf("runnable should be false: %#v", parsed)
	}
	if _, ok := parsed["registry"].(map[string]interface{}); !ok {
		t.Fatalf("registry block missing: %#v", parsed)
	}
}

func TestBestResponse(t *testing.T) {
	// Empty -> failure sentinel.
	if got := bestResponse(map[string]*workflow.WorkerResponse{}); got.Success || got.Error == "" {
		t.Fatalf("empty: %#v", got)
	}

	ok := &workflow.WorkerResponse{StatusCode: 200, Response: "good"}
	bad := &workflow.WorkerResponse{StatusCode: 500}
	got := bestResponse(map[string]*workflow.WorkerResponse{"a": bad, "b": ok, "c": nil})
	if !got.Success || got.StatusCode != 200 || got.Response != "good" {
		t.Fatalf("prefer success: %#v", got)
	}
}

func TestHumanResult(t *testing.T) {
	s := humanResult(runResult{Success: true, StatusCode: 200, DurationMs: 5, Response: "hi"})
	if !strings.Contains(s, "OK (5ms) [200]") || !strings.Contains(s, "hi") {
		t.Fatalf("success:\n%s", s)
	}
	s = humanResult(runResult{Success: false, StatusCode: 422, Error: "boom", Response: map[string]any{"k": "v"}})
	if !strings.Contains(s, "FAILED (0ms) [422]") || !strings.Contains(s, "error: boom") || !strings.Contains(s, `"k": "v"`) {
		t.Fatalf("failure:\n%s", s)
	}
}
