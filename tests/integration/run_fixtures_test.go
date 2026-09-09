package integration

import (
	"strings"
	"testing"

	"github.com/kuetix/kue/tests/testutil"
)

func TestRunFixtureWorkflows(t *testing.T) {
	cases := []struct {
		file string
	}{
		{"hello.wsl"},
		{"hello.swsl"},
		{"uses_decision.wsl"}, // exercises std-decision end to end
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			r := testutil.RunWorkflowFile(t, testutil.FixturePath(t, "workflows", tc.file))
			if r.Err != nil {
				t.Fatalf("err: %v", r.Err)
			}
			if r.StatusCode != 200 {
				t.Fatalf("status %d, resp %#v", r.StatusCode, r.Response)
			}
		})
	}
}

func TestRunBrokenWorkflowFailsToValidate(t *testing.T) {
	r := testutil.RunWorkflowFile(t, testutil.FixturePath(t, "workflows", "broken.wsl"))
	if r.Err == nil && r.StatusCode == 200 {
		t.Fatalf("broken.wsl should not run cleanly: status=%d resp=%#v", r.StatusCode, r.Response)
	}
}

// TestRunCommandExecutesFixture drives the full `kue run <file>` command
// workflow (@cli/run/run -> runner.RunCommand -> nested engine).
func TestRunCommandExecutesFixture(t *testing.T) {
	path := testutil.FixturePath(t, "workflows", "uses_decision.wsl")
	ctx := testutil.CmdContext("run."+path, nil)

	r := testutil.RunCLICommand(t, "@cli/run/run", ctx)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	out, ok := r.Response.(string)
	if !ok {
		t.Fatalf("expected string response, got %T", r.Response)
	}
	if !strings.Contains(out, "OK") || !strings.Contains(out, "[200]") {
		t.Fatalf("unexpected run output:\n%s", out)
	}
}

func TestRunCommandCheckMode(t *testing.T) {
	path := testutil.FixturePath(t, "workflows", "hello.wsl")
	ctx := testutil.CmdContext("run."+path, map[string]interface{}{"check": testutil.BoolFlag(true)})

	r := testutil.RunCLICommand(t, "@cli/run/run", ctx)
	if r.Err != nil {
		t.Fatalf("err: %v", r.Err)
	}
	out, _ := r.Response.(string)
	if !strings.Contains(out, "Runnable: yes") {
		t.Fatalf("check report should say runnable:\n%s", out)
	}
}

func TestRunCommandMissingTarget(t *testing.T) {
	r := testutil.RunCLICommand(t, "@cli/run/run", testutil.CmdContext("run", nil))
	if r.Err == nil {
		t.Fatal("expected an error when no target is given")
	}
	if !strings.Contains(r.Err.Error(), "nothing to run") {
		t.Fatalf("unexpected error: %v", r.Err)
	}
}
