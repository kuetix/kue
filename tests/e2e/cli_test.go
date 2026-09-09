//go:build e2e

// Package e2e drives the compiled `kue` binary as a subprocess. Run with:
//
//	go test -tags e2e ./tests/e2e/...
package e2e

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuetix/kue/tests/testutil"
)

func TestModulesListsLinkedPackages(t *testing.T) {
	res := testutil.RunCLI(t, "", nil, "modules")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	for _, want := range []string{"std-redis", "std-decision", "std-jsondb", "std-mysql", "std-push", "social-oauth"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("`kue modules` missing %q\n%s", want, res.Stdout)
		}
	}
}

func TestTransitionsModuleFilter(t *testing.T) {
	res := testutil.RunCLI(t, "", nil, "transitions", "--module", "std-decision")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !strings.Contains(res.Stdout, "decision/cond.Number") {
		t.Fatalf("missing decision/cond.Number:\n%s", res.Stdout)
	}
}

func TestModulesJSON(t *testing.T) {
	res := testutil.RunCLI(t, "", nil, "modules", "--json")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	var parsed struct {
		Count int `json:"count"`
	}
	dec := json.NewDecoder(strings.NewReader(res.JSONStdout()))
	if err := dec.Decode(&parsed); err != nil {
		t.Fatalf("decode: %v\n%s", err, res.Stdout)
	}
	if parsed.Count < 10 {
		t.Fatalf("expected >=10 packages, got %d", parsed.Count)
	}
}

func TestHelpAndDefaultCommand(t *testing.T) {
	help := testutil.RunCLI(t, "", nil, "help")
	if help.ExitCode != 0 || help.Combined() == "" {
		t.Fatalf("help: exit %d\n%s", help.ExitCode, help.Combined())
	}

	// No args -> defaultCommand "help", still exit 0.
	bare := testutil.RunCLI(t, "", nil)
	if bare.ExitCode != 0 {
		t.Fatalf("bare invocation exit %d\n%s", bare.ExitCode, bare.Combined())
	}
}

func TestUnknownCommand(t *testing.T) {
	res := testutil.RunCLI(t, "", nil, "definitely-not-a-command")
	if res.ExitCode == 0 {
		t.Fatalf("unknown command should be non-zero exit\n%s", res.Combined())
	}
}
