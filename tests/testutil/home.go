package testutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kuetix/kue/modules/shared"
)

// TempHome points HOME (and USERPROFILE on Windows) at a fresh temp directory
// for the duration of the test and seeds ~/.kue/config.json with cfg. It
// returns the home path. Env vars are restored via t.Cleanup.
//
// Note: github.com/kuetix/kue.HomeDir is resolved once at package init, so
// in-process code that reads it is unaffected; TempHome is for helpers that
// read os.UserHomeDir() / $HOME live (shared.*, and any child process).
func TempHome(t *testing.T, cfg shared.KueConfig) string {
	t.Helper()
	home := t.TempDir()

	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	cfgPath := filepath.Join(home, ".kue", "config.json")
	if err := shared.SaveKueConfig(cfgPath, cfg); err != nil {
		t.Fatalf("seed kue config: %v", err)
	}
	return home
}

// ChdirTemp switches into a fresh temp dir that already contains the empty
// modules/ and workflows/ subdirectories the engine's path helpers expect,
// restoring the previous working directory on cleanup. It returns the dir.
func ChdirTemp(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"modules", "workflows"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", sub, err)
		}
	}
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	return dir
}
