package transitions

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWorkflowResponse(t *testing.T) {
	i := &installTransitions{}

	// Wrapped under "data".
	content, imports, deps, err := i.parseWorkflowResponse(`{"data":{
		"content":"module x\n",
		"imports":{"a":"module a\n"},
		"dependencies":[{"go_module":"github.com/kuetix/std-http"},{"go_module":"github.com/kuetix/std-http"},{"nope":1}]
	}}`)
	if err != nil {
		t.Fatal(err)
	}
	if content != "module x\n" || imports["a"] != "module a\n" {
		t.Fatalf("content=%q imports=%v", content, imports)
	}
	if len(deps) != 1 || deps[0] != "github.com/kuetix/std-http" {
		t.Fatalf("deps=%v (should dedupe)", deps)
	}

	// Non-JSON body is treated as raw workflow content.
	c, _, _, err := i.parseWorkflowResponse("module raw\n")
	if err != nil || c != "module raw" {
		t.Fatalf("raw: c=%q err=%v", c, err)
	}

	if _, _, _, err := i.parseWorkflowResponse("   "); err == nil {
		t.Fatal("empty body should error")
	}
}

func TestExtractPackageGoModules(t *testing.T) {
	i := &installTransitions{}
	body := `{"data":{
		"modules":["github.com/a/b", {"go_module":"github.com/c/d"}],
		"dependencies":[{"go_module":"github.com/c/d"},{"go_module":"github.com/e/f"}]
	}}`
	got := i.extractPackageGoModules(body)
	want := "github.com/a/b,github.com/c/d,github.com/e/f"
	if strings.Join(got, ",") != want {
		t.Fatalf("got %v want %s", got, want)
	}
	if i.extractPackageGoModules("not json") != nil {
		t.Fatal("invalid JSON -> nil")
	}
}

func TestReadExistingRequires(t *testing.T) {
	i := &installTransitions{}
	dir := t.TempDir()
	gomod := "module example\n\ngo 1.26\n\nrequire (\n\tgithub.com/kuetix/std-core v1.0.0\n\t// a comment\n\tgithub.com/kuetix/std-http v0.1.0 // indirect\n)\n\nrequire github.com/kuetix/engine v1.2.0\n"
	path := filepath.Join(dir, "go.mod")
	if err := os.WriteFile(path, []byte(gomod), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := i.readExistingRequires(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"github.com/kuetix/std-core", "github.com/kuetix/std-http", "github.com/kuetix/engine"} {
		if !got[want] {
			t.Errorf("missing %q in %v", want, got)
		}
	}

	if _, err := i.readExistingRequires(filepath.Join(dir, "absent")); err == nil {
		t.Fatal("missing go.mod should error")
	}
}

func TestWriteWorkflowFile(t *testing.T) {
	i := &installTransitions{}
	path := filepath.Join(t.TempDir(), "sub", "flow.wsl")

	st, err := i.writeWorkflowFile(path, "module x", false)
	if err != nil || st != "created" {
		t.Fatalf("first write: st=%q err=%v", st, err)
	}
	data, _ := os.ReadFile(path)
	if string(data) != "module x\n" {
		t.Fatalf("trailing newline not added: %q", data)
	}

	// Idempotent: identical content -> unchanged, no rewrite needed.
	if st, err := i.writeWorkflowFile(path, "module x\n", false); err != nil || st != "unchanged" {
		t.Fatalf("re-write identical: st=%q err=%v", st, err)
	}

	// Different content -> updated in place (get is meant to refresh).
	if st, err := i.writeWorkflowFile(path, "module y", false); err != nil || st != "updated" {
		t.Fatalf("update: st=%q err=%v", st, err)
	}
	data, _ = os.ReadFile(path)
	if string(data) != "module y\n" {
		t.Fatalf("did not overwrite: %q", data)
	}
}

func TestIsInProject(t *testing.T) {
	empty := t.TempDir()
	if isInProject(empty) {
		t.Error("empty dir is not a project")
	}
	withMod := t.TempDir()
	_ = os.WriteFile(filepath.Join(withMod, "go.mod"), []byte("module x\n"), 0o600)
	if !isInProject(withMod) {
		t.Error("dir with go.mod is a project")
	}
	withApp := t.TempDir()
	_ = os.WriteFile(filepath.Join(withApp, "application.json"), []byte("{}"), 0o600)
	if !isInProject(withApp) {
		t.Error("dir with application.json is a project")
	}
}

func TestInstallFlagHelpers(t *testing.T) {
	i := &installTransitions{}
	opts := map[string]interface{}{"name": "  acme/x  ", "force": true, "owner": 1}
	if got := i.firstPositional(nil, map[string]interface{}{}, opts); got != "acme/x" {
		t.Fatalf("firstPositional name flag: %q", got)
	}
	if got := i.firstPositional(nil, map[string]interface{}{"args": []string{"acme/y"}}, map[string]interface{}{}); got != "acme/y" {
		t.Fatalf("firstPositional args: %q", got)
	}
	if !i.boolOpt(opts, "force") || i.boolOpt(opts, "missing") {
		t.Error("boolOpt")
	}
	if i.strOpt(opts, "owner") != "" {
		t.Error("strOpt non-string -> empty")
	}
	if !i.isNotFound(http.StatusNotFound) || i.isNotFound(http.StatusOK) {
		t.Error("isNotFound")
	}
}

func TestInstallPackageFromBodyReport(t *testing.T) {
	i := &installTransitions{}
	// No project at output dir -> deps parsed but not wired; report mentions raw body.
	out, err := i.installPackageFromBody("acme/util", `{"data":{"modules":[]}}`, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `Installed package "acme/util"`) || !strings.Contains(out, "did not advertise any Go modules") {
		t.Fatalf("report:\n%s", out)
	}
}
