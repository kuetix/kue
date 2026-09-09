//go:build e2e

package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuetix/kue/tests/testutil"
)

func TestRunHelloFixture(t *testing.T) {
	res := testutil.RunCLI(t, "", nil, "run", testutil.FixturePath(t, "workflows", "hello.wsl"))
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !strings.Contains(res.Combined(), "OK") || !strings.Contains(res.Combined(), "[200]") {
		t.Fatalf("unexpected output:\n%s", res.Combined())
	}
}

func TestRunUsesDecisionFixture(t *testing.T) {
	// End-to-end proof that std-decision is linked and runnable.
	res := testutil.RunCLI(t, "", nil, "run", testutil.FixturePath(t, "workflows", "uses_decision.wsl"))
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
}

func TestRunBrokenFixtureExitsNonZero(t *testing.T) {
	res := testutil.RunCLI(t, "", nil, "run", testutil.FixturePath(t, "workflows", "broken.wsl"))
	if res.ExitCode == 0 {
		t.Fatalf("broken.wsl should fail\n%s", res.Combined())
	}
}

func TestRunJSONOutput(t *testing.T) {
	// NOTE: `kue run` only recognizes flags AFTER the positional target.
	res := testutil.RunCLI(t, "", nil, "run", testutil.FixturePath(t, "workflows", "hello.wsl"), "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	var parsed struct {
		Success    bool `json:"success"`
		StatusCode int  `json:"status_code"`
	}
	dec := json.NewDecoder(strings.NewReader(res.JSONStdout()))
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("decode: %v\n%s", err, res.Stdout)
	}
	if !parsed.Success {
		t.Fatalf("expected success in JSON result:\n%s", res.Stdout)
	}
}

func TestRunProjectDir(t *testing.T) {
	res := testutil.RunCLI(t, testutil.FixturePath(t, "project"), nil, "run", ".", "--check")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !strings.Contains(res.Combined(), "project directory") {
		t.Fatalf("expected project-dir check message:\n%s", res.Combined())
	}
}
