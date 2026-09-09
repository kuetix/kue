//go:build e2e

package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuetix/kue/tests/testutil"
)

// TestAddCommandsNoPanic guards the regression where `kue add package <name>`
// (and add workflow/feature/solution with a positional) panicked with
// "interface conversion: interface {} is nil, not string" because the command
// registered no --name option.
func TestAddCommandsNoPanic(t *testing.T) {
	cases := [][]string{
		{"add", "package", "billing"},
		{"add", "package", "--name", "billing"},
		{"add", "package"}, // name defaults to the directory
		{"add", "module", "orders"},
		{"add", "workflow", "flows/report"},
		{"add", "feature", "checkout"},
		{"add", "solution", "onboarding"},
		{"add", "transition", "orders/Ship"},
	}
	for _, args := range cases {
		args := args
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			dir := t.TempDir()
			res := testutil.RunCLI(t, dir, nil, args...)
			for _, m := range panicMarkers {
				if strings.Contains(res.Combined(), m) {
					t.Fatalf("kue %s: %q in output\n%s", strings.Join(args, " "), m, res.Combined())
				}
			}
		})
	}
}

// TestAddPackageWritesFile: the package descriptor path has no template
// dependency (it falls back to hand-written JSON), so it must fully succeed.
func TestAddPackageWritesFile(t *testing.T) {
	dir := t.TempDir()
	res := testutil.RunCLI(t, dir, nil, "add", "package", "billing")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	data, err := os.ReadFile(filepath.Join(dir, "kuetix.json"))
	if err != nil {
		t.Fatalf("kuetix.json not written: %v", err)
	}
	if !strings.Contains(string(data), `"billing"`) {
		t.Fatalf("kuetix.json missing name:\n%s", data)
	}
}
