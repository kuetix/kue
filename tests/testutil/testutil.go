// Package testutil holds shared helpers for kue's unit, integration and e2e
// tests: an isolated HOME, an in-process mock registry, a workflow runner that
// drives the real engine, and a build-the-binary-once helper for subprocess
// e2e tests.
//
// It deliberately takes testing.TB (not *testing.T) so the same helpers work
// from tests, benchmarks and TestMain.
package testutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ModuleRoot returns the absolute path of the kue module root (the directory
// holding go.mod), located by walking up from this source file.
func ModuleRoot(tb testing.TB) string {
	tb.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		tb.Fatal("testutil: runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			tb.Fatal("testutil: could not locate module root (no go.mod found)")
		}
		dir = parent
	}
}

// FixturePath resolves a path under tests/fixtures/.
func FixturePath(tb testing.TB, parts ...string) string {
	tb.Helper()
	return filepath.Join(append([]string{ModuleRoot(tb), "tests", "fixtures"}, parts...)...)
}
