package transitions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildPackagePaths(t *testing.T) {
	if got := buildPackageSearchPath("a b/c"); got != "/packages/search?q=a+b%2Fc" {
		t.Fatalf("search path: %q", got)
	}
	if got := buildPackageInstallPath("acme/util"); got != "/packages/install?name=acme%2Futil" {
		t.Fatalf("install path: %q", got)
	}
}

func TestLastPathSegment(t *testing.T) {
	cases := map[string]string{
		"acme/util":  "util",
		"acme/util/": "util",
		"util":       "util",
		"a/b/c/leaf": "leaf",
	}
	for in, want := range cases {
		if got := lastPathSegment(in); got != want {
			t.Errorf("lastPathSegment(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsMajorVersionSuffix(t *testing.T) {
	for _, s := range []string{"v2", "v3", "v10"} {
		if !isMajorVersionSuffix(s) {
			t.Errorf("%q should be a major-version suffix", s)
		}
	}
	for _, s := range []string{"v", "version2", "2", "vx", ""} {
		if isMajorVersionSuffix(s) {
			t.Errorf("%q should not be a major-version suffix", s)
		}
	}
}

func TestFindPackageDirWalksUp(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "kuetix.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := findPackageDir(deep)
	if err != nil {
		t.Fatal(err)
	}
	if want, _ := filepath.EvalSymlinks(root); got != want && got != root {
		t.Fatalf("got %q, want %q", got, root)
	}

	if _, err := findPackageDir(t.TempDir()); err == nil {
		t.Fatal("no kuetix.json anywhere -> error")
	}
}

func TestReadGoModuleName(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("// header\nmodule  github.com/kuetix/std-x  \n\ngo 1.26\n"), 0o600)
	got, err := readGoModuleName(dir)
	if err != nil || got != "github.com/kuetix/std-x" {
		t.Fatalf("got %q err %v", got, err)
	}

	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("go 1.26\n"), 0o600)
	if _, err := readGoModuleName(dir); err == nil {
		t.Fatal("no module directive -> error")
	}
}

func TestBuildPackagePayload(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "kuetix.json"), []byte(`{"name":"acme/util","type":"package","version":"1.2.3","keywords":["a"]}`), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "modules.json"), []byte(`["util/one","util/two"]`), 0o600)

	p, err := buildPackagePayload(dir)
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "acme/util" || p.Version != "1.2.3" || len(p.Keywords) != 1 {
		t.Fatalf("payload: %#v", p)
	}
	if strings.Join(p.Modules, ",") != "util/one,util/two" {
		t.Fatalf("modules: %v", p.Modules)
	}

	if _, err := buildPackagePayload(t.TempDir()); err == nil {
		t.Fatal("missing kuetix.json -> error")
	}
}

func TestBuildPackagePayloadReadsGeneratedModuleCatalog(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kuetix.json"), []byte(`{"name":"ai","type":"package"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	modulesDir := filepath.Join(dir, "modules")
	if err := os.MkdirAll(modulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data := `{"ai/agent":{"info":{"go_module":"github.com/kuetix/std-ai"}},"ai/prompt":{"info":{"go_module":"github.com/kuetix/std-ai"}}}`
	if err := os.WriteFile(filepath.Join(modulesDir, "modules.json"), []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}

	p, err := buildPackagePayload(dir)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(p.Modules, ",") != "github.com/kuetix/std-ai" {
		t.Fatalf("modules: %v", p.Modules)
	}
}

func TestResolvePublishDir(t *testing.T) {
	if resolvePublishDir("") != "." || resolvePublishDir("does-not-exist") != "." {
		t.Fatal("empty / missing -> .")
	}
	dir := t.TempDir()
	if resolvePublishDir(dir) != dir {
		t.Fatalf("existing dir should be returned")
	}
}

func TestGenerateBootstrapFile(t *testing.T) {
	out := t.TempDir()
	path, err := generateBootstrapFile("github.com/kuetix/std-demo", out)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "bootstrap.go" || !strings.Contains(path, filepath.Join("modules", "std-demo")) {
		t.Fatalf("path: %s", path)
	}
	data, _ := os.ReadFile(path)
	src := string(data)
	if !strings.Contains(src, "package std-demo") || !strings.Contains(src, `"github.com/kuetix/std-demo/modules"`) || !strings.Contains(src, "modules.Enable()") {
		t.Fatalf("generated source:\n%s", src)
	}
}

func TestListAndAddEnableCall(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/acme/app\n"), 0o600)
	modDir := filepath.Join(dir, "modules")
	_ = os.MkdirAll(modDir, 0o755)
	modulesFile := filepath.Join(modDir, "modules.go")
	_ = os.WriteFile(modulesFile, []byte("package modules\n\nimport (\n\tdi \"github.com/kuetix/container\"\n)\n\nfunc init() { di.Boot() }\n\nfunc Enable() {\n}\n"), 0o600)

	if err := addEnableCallToModules("github.com/kuetix/std-demo", modulesFile); err != nil {
		t.Fatalf("add: %v", err)
	}
	data, _ := os.ReadFile(modulesFile)
	src := string(data)
	if !strings.Contains(src, "github.com/acme/app/modules/std-demo") || !strings.Contains(src, "std-demo.Enable()") {
		t.Fatalf("after add:\n%s", src)
	}

	// Idempotent.
	before := src
	_ = addEnableCallToModules("github.com/kuetix/std-demo", modulesFile)
	data, _ = os.ReadFile(modulesFile)
	if string(data) != before {
		t.Fatalf("second call changed the file:\n%s", data)
	}

	names, err := listEnabledModules(modulesFile)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, n := range names {
		if strings.Contains(n, "std-demo") {
			found = true
		}
	}
	if !found {
		t.Fatalf("listEnabledModules missing std-demo: %v", names)
	}
}

func TestShowPackageInfo(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "kuetix.json"), []byte(`{"name":"acme/util","type":"package","version":"1.0.0","keywords":["x","y"]}`), 0o600)
	out, err := showPackageInfo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Package: acme/util") || !strings.Contains(out, "Keywords:    x, y") {
		t.Fatalf("output:\n%s", out)
	}
	if _, err := showPackageInfo(t.TempDir()); err == nil {
		t.Fatal("missing kuetix.json -> error")
	}
}
