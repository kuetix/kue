//go:build e2e

package e2e

import (
	"strings"
	"testing"

	"github.com/kuetix/kue/tests/testutil"
)

// panicMarkers are strings that must never appear in `kue` output — they mean a
// transition panicked or a workflow returned the wrong type.
var panicMarkers = []string{
	"interface conversion",
	"Please return domain.FlowStepResult",
	"panic:",
	"runtime error",
}

func assertClean(t *testing.T, res testutil.CLIResult, args ...string) {
	t.Helper()
	if res.ExitCode != 0 {
		t.Fatalf("kue %s: exit %d\n%s", strings.Join(args, " "), res.ExitCode, res.Combined())
	}
	out := res.Combined()
	for _, m := range panicMarkers {
		if strings.Contains(out, m) {
			t.Fatalf("kue %s: output contains %q\n%s", strings.Join(args, " "), m, out)
		}
	}
	if strings.Contains(out, "file://") {
		t.Fatalf("kue %s: help text leaked an unresolved file:// reference\n%s", strings.Join(args, " "), out)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("kue %s: empty help output", strings.Join(args, " "))
	}
}

// TestCommandHelpFlag guards the regression where `kue show --help` (and every
// other command whose transition reads config["flagSet"]) panicked with
// "interface conversion: interface {} is nil, not *flag.FlagSet", and the
// follow-up where the same commands printed a raw "file://…" path instead of
// the usage text.
func TestCommandHelpFlag(t *testing.T) {
	cases := [][]string{
		{"show", "--help"},
		{"update", "--help"},
		{"create", "--help"},
		{"add", "--help"},
		{"add", "module", "--help"},
		{"add", "transition", "--help"},
		{"add", "package", "--help"},
		{"templates", "--help"},
		{"templates", "status", "--help"},
		{"wsl", "--help"},
		{"wsl", "inspect", "--help"},
		{"wsl", "list", "--help"},
		{"wsl", "status", "--help"},
		{"wsl", "upload", "--help"},
		{"wsl", "get", "--help"},
		{"package", "search", "--help"},
		{"package", "list", "--help"},
		{"package", "publish", "--help"},
		{"completion", "--help"},
		{"login", "--help"},
		{"logout", "--help"},
		{"register", "--help"},
		{"whoami", "--help"},
		{"profile", "get", "--help"},
		{"install", "--help"},
		{"run", "--help"},
		{"modules", "--help"},
		{"transitions", "--help"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			assertClean(t, testutil.RunCLI(t, "", nil, args...), args...)
		})
	}
}

// TestHelpSubtopics: examples and env vars moved off the default help screen
// into `kue help examples` / `kue help env`.
func TestHelpSubtopics(t *testing.T) {
	def := testutil.RunCLI(t, "", nil, "help")
	if def.ExitCode != 0 {
		t.Fatalf("help: exit %d\n%s", def.ExitCode, def.Combined())
	}
	if strings.Contains(def.Stdout, "KUE_HOST") || strings.Contains(def.Stdout, "kue run ./flows") {
		t.Fatalf("default help must not carry the examples/env blocks:\n%s", def.Stdout)
	}
	for _, ref := range []string{"kue help examples", "kue help env"} {
		if !strings.Contains(def.Stdout, ref) {
			t.Fatalf("default help should point at %q:\n%s", ref, def.Stdout)
		}
	}

	ex := testutil.RunCLI(t, "", nil, "help", "examples")
	assertClean(t, ex, "help", "examples")
	if !strings.Contains(ex.Stdout, "kue run ./flows") {
		t.Fatalf("`kue help examples` missing examples:\n%s", ex.Stdout)
	}

	env := testutil.RunCLI(t, "", nil, "help", "env")
	assertClean(t, env, "help", "env")
	if !strings.Contains(env.Stdout, "KUE_HOST") {
		t.Fatalf("`kue help env` missing env vars:\n%s", env.Stdout)
	}
}

// TestCompletionCommand covers `kue completion bash|zsh` and the guardrails.
func TestCompletionCommand(t *testing.T) {
	bash := testutil.RunCLI(t, "", nil, "completion", "bash")
	if bash.ExitCode != 0 || !strings.Contains(bash.Stdout, "complete -F _kue kue") {
		t.Fatalf("completion bash:\nexit %d\n%s", bash.ExitCode, bash.Combined())
	}

	zsh := testutil.RunCLI(t, "", nil, "completion", "zsh")
	if zsh.ExitCode != 0 || !strings.Contains(zsh.Stdout, "#compdef kue") {
		t.Fatalf("completion zsh:\nexit %d\n%s", zsh.ExitCode, zsh.Combined())
	}

	// No shell -> install instructions, exit 0.
	none := testutil.RunCLI(t, "", nil, "completion")
	if none.ExitCode != 0 || !strings.Contains(none.Stdout, "kue completion <shell>") {
		t.Fatalf("bare completion:\nexit %d\n%s", none.ExitCode, none.Combined())
	}

	// Unsupported shell -> non-zero exit.
	fish := testutil.RunCLI(t, "", nil, "completion", "fish")
	if fish.ExitCode == 0 {
		t.Fatalf("completion fish should be a non-zero exit\n%s", fish.Combined())
	}
}
