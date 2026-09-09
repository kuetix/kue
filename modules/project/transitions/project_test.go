package transitions

import (
	"os"
	"path/filepath"
	"testing"
)

func writeApp(t *testing.T, dir, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "application.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestIsProjectDir(t *testing.T) {
	empty := t.TempDir()
	if IsProjectDir(empty) {
		t.Error("empty dir is not a project")
	}

	proj := t.TempDir()
	writeApp(t, proj, `{"name":"p","type":"cli"}`)
	if !IsProjectDir(proj) {
		t.Error("dir with application.json is a project")
	}

	// A directory named application.json must not count.
	weird := t.TempDir()
	if err := os.Mkdir(filepath.Join(weird, "application.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if IsProjectDir(weird) {
		t.Error("application.json as a directory must not count")
	}
}

func TestReadAppType(t *testing.T) {
	proj := t.TempDir()
	writeApp(t, proj, `{"name":"p","type":"api"}`)
	got, err := readAppType(proj)
	if err != nil || got != "api" {
		t.Fatalf("got %q err %v", got, err)
	}

	noType := t.TempDir()
	writeApp(t, noType, `{"name":"p"}`)
	if _, err := readAppType(noType); err == nil {
		t.Error("missing type should error")
	}

	bad := t.TempDir()
	writeApp(t, bad, `{not json`)
	if _, err := readAppType(bad); err == nil {
		t.Error("invalid json should error")
	}
}

func TestResolveProjectDir(t *testing.T) {
	proj := t.TempDir()
	writeApp(t, proj, `{"name":"p","type":"cli"}`)

	got, err := resolveProjectDir(proj)
	abs, _ := filepath.Abs(proj)
	if err != nil || got != abs {
		t.Fatalf("explicit: got %q err %v", got, err)
	}

	if _, err := resolveProjectDir(t.TempDir()); err == nil {
		t.Error("non-project dir should error")
	}

	// No arg -> uses cwd.
	prev, _ := os.Getwd()
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(proj); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveProjectDir(""); err != nil {
		t.Fatalf("cwd project: %v", err)
	}
}
