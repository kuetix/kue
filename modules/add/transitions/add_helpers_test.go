package transitions

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func boolThunk(v bool) func() *bool    { return func() *bool { return &v } }
func strThunk(v string) func() *string { return func() *string { return &v } }

// targetName / outputDir receive options already resolved by GetFlags, i.e.
// plain string/bool values.
func TestTargetName(t *testing.T) {
	// Positional wins.
	got := targetName(
		map[string]interface{}{"args": []string{"billing", "extra"}},
		map[string]interface{}{"name": "ignored"},
	)
	if got != "billing" {
		t.Fatalf("positional: %q", got)
	}

	// Falls back to --name.
	got = targetName(map[string]interface{}{}, map[string]interface{}{"name": "  orders  "})
	if got != "orders" {
		t.Fatalf("name: %q", got)
	}

	// Neither present -> empty, no panic (regression: bare options["name"].(string)).
	if got = targetName(map[string]interface{}{}, map[string]interface{}{}); got != "" {
		t.Fatalf("empty: %q", got)
	}
	if got = targetName(nil, nil); got != "" {
		t.Fatalf("nil: %q", got)
	}
}

func TestOutputDir(t *testing.T) {
	if got := outputDir(map[string]interface{}{}); got != "." {
		t.Fatalf("default: %q", got)
	}
	if got := outputDir(map[string]interface{}{"output": "./pkg"}); got != "./pkg" {
		t.Fatalf("set: %q", got)
	}
	if got := outputDir(map[string]interface{}{"output": "   "}); got != "." {
		t.Fatalf("blank -> default: %q", got)
	}
}

// chdirTemp switches into a fresh temp dir for the duration of the test.
func chdirTemp(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
}

// TestAddPackageCommand guards the panic `kue add package <name>` used to hit:
// "interface conversion: interface {} is nil, not string" — the command
// registered no --name option, but the transition asserted options["name"].
func TestAddPackageCommand(t *testing.T) {
	a := &addTransitions{}

	t.Run("positional name", func(t *testing.T) {
		chdirTemp(t)
		r := a.AddPackageCommand("", map[string]interface{}{"args": []string{"billing"}},
			map[string]interface{}{"help": boolThunk(false)})
		if r.Error != nil {
			t.Fatalf("err: %v", r.Error)
		}
		assertPackageName(t, "kuetix.json", "billing")
	})

	t.Run("--name flag", func(t *testing.T) {
		chdirTemp(t)
		r := a.AddPackageCommand("", map[string]interface{}{},
			map[string]interface{}{"help": boolThunk(false), "name": strThunk("orders"), "output": strThunk(".")})
		if r.Error != nil {
			t.Fatalf("err: %v", r.Error)
		}
		assertPackageName(t, "kuetix.json", "orders")
	})

	t.Run("no name -> directory basename, no panic", func(t *testing.T) {
		chdirTemp(t)
		r := a.AddPackageCommand("", map[string]interface{}{}, map[string]interface{}{"help": boolThunk(false)})
		if r.Error != nil {
			t.Fatalf("err: %v", r.Error)
		}
		assertPackageName(t, "kuetix.json", "") // name is the temp dir's basename
	})
}

// TestAddWorkflowFamilyNoPanic: `kue add workflow|feature|solution <name>`
// registered no --name option; the transitions must resolve the positional and
// never panic. A missing template in the test env is a normal error.
func TestAddWorkflowFamilyNoPanic(t *testing.T) {
	a := &addTransitions{}
	cfg := map[string]interface{}{"args": []string{"flows/report"}}
	flags := map[string]interface{}{"help": boolThunk(false)}

	cmds := map[string]func() error{
		"workflow": func() error { chdirTemp(t); return a.AddWorkflowCommand("", cfg, flags).Error },
		"feature":  func() error { chdirTemp(t); return a.AddFeatureCommand("", cfg, flags).Error },
		"solution": func() error { chdirTemp(t); return a.AddSolutionCommand("", cfg, flags).Error },
	}
	for name, fn := range cmds {
		t.Run(name, func(t *testing.T) {
			_ = fn() // must not panic; error is acceptable
		})
	}

	// Empty name -> a clean "name is required" error, still no panic.
	r := a.AddWorkflowCommand("", map[string]interface{}{}, map[string]interface{}{"help": boolThunk(false)})
	if r.Error == nil || !strings.Contains(r.Error.Error(), "workflow name is required") {
		t.Fatalf("expected name-required error, got %v", r.Error)
	}
}

func assertPackageName(t *testing.T, path, wantName string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(data, &obj); err != nil {
		t.Fatalf("invalid JSON in %s: %v\n%s", path, err, data)
	}
	if wantName != "" {
		if got, _ := obj["name"].(string); got != wantName {
			t.Fatalf("name = %q, want %q", got, wantName)
		}
	}
}
