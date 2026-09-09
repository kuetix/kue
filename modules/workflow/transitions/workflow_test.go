package transitions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuetix/engine/engine/domain/interfaces"
	"github.com/kuetix/kue/modules/shared"
)

func TestSplitModule(t *testing.T) {
	ns, cls := splitModule("services/common/assert")
	if ns != "services/common" || cls != "assert" {
		t.Fatalf("got %q %q", ns, cls)
	}
	if ns, cls := splitModule("redis/string"); ns != "redis" || cls != "string" {
		t.Fatalf("got %q %q", ns, cls)
	}
	if ns, cls := splitModule("noslash"); ns != "" || cls != "" {
		t.Fatalf("no slash -> empty, got %q %q", ns, cls)
	}
}

func TestNormalizeContent(t *testing.T) {
	if got := normalizeContent("  a\r\nb\r\n\n  "); got != "a\nb" {
		t.Fatalf("got %q", got)
	}
}

func TestStrOptBoolOptOptsFromFlags(t *testing.T) {
	opts := map[string]interface{}{
		"projectName": "  demo  ",
		"public":      true,
		"other":       123,
	}
	if strOpt(opts, "projectName") != "  demo  " {
		t.Fatal("strOpt raw value")
	}
	if strOpt(opts, "other") != "" {
		t.Fatal("strOpt non-string -> empty")
	}
	if !boolOpt(opts, "public") || boolOpt(opts, "missing") {
		t.Fatal("boolOpt")
	}
	uo := optsFromFlags(opts)
	if uo.ProjectName != "demo" || !uo.Public {
		t.Fatalf("optsFromFlags: %#v", uo)
	}
}

func TestJSONEncode(t *testing.T) {
	if got := jsonEncode(map[string]int{"a": 1}); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
	if got := jsonEncode(make(chan int)); got != "" {
		t.Fatalf("unencodable -> empty, got %q", got)
	}
}

func TestFirstPositional(t *testing.T) {
	w := &workflowTransitions{}
	if got := w.firstPositional(nil, map[string]interface{}{"name": " acme/x "}); got != "acme/x" {
		t.Fatalf("name flag: %q", got)
	}
	if got := w.firstPositional(map[string]interface{}{"args": []string{" y "}}, map[string]interface{}{}); got != "y" {
		t.Fatalf("args: %q", got)
	}
	if got := w.firstPositional(map[string]interface{}{}, map[string]interface{}{}); got != "" {
		t.Fatalf("none: %q", got)
	}
}

func TestFindProjectRoot(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o600)
	deep := filepath.Join(root, "a", "b")
	_ = os.MkdirAll(deep, 0o755)

	got, ok := findProjectRoot(deep)
	if !ok || got != root {
		t.Fatalf("got %q ok=%v want %q", got, ok, root)
	}
	if _, ok := findProjectRoot(t.TempDir()); ok {
		t.Fatal("no go.mod -> not found")
	}
}

func TestFindWorkflowsRootAndDiscover(t *testing.T) {
	base := t.TempDir()
	wfRoot := filepath.Join(base, "workflows")
	_ = os.MkdirAll(filepath.Join(wfRoot, "cli", "auth"), 0o755)
	_ = os.WriteFile(filepath.Join(wfRoot, "top.wsl"), []byte("module top\n"), 0o600)
	_ = os.WriteFile(filepath.Join(wfRoot, "cli", "auth", "login.swsl"), []byte("x\n"), 0o600)
	_ = os.WriteFile(filepath.Join(wfRoot, "cli", "notes.txt"), []byte("ignore\n"), 0o600)

	if got := findWorkflowsRoot(filepath.Join(wfRoot, "cli", "auth", "login.swsl")); got != wfRoot {
		t.Fatalf("findWorkflowsRoot = %q, want %q", got, wfRoot)
	}
	if findWorkflowsRoot(filepath.Join(base, "nowhere.wsl")) != "" {
		t.Fatal("no workflows dir -> empty")
	}

	names, err := discoverWorkflowNames(wfRoot)
	if err != nil {
		t.Fatal(err)
	}
	set := strings.Join(names, ",")
	if !strings.Contains(set, "top") || !strings.Contains(set, "cli/auth/login") || strings.Contains(set, "notes") {
		t.Fatalf("discoverWorkflowNames = %v", names)
	}
}

func TestCollectImports(t *testing.T) {
	base := t.TempDir()
	wfRoot := filepath.Join(base, "workflows")
	_ = os.MkdirAll(filepath.Join(wfRoot, "cli"), 0o755)
	_ = os.WriteFile(filepath.Join(wfRoot, "consts.wsl"), []byte("module consts\nconst { x: 1 }\n"), 0o600)
	_ = os.WriteFile(filepath.Join(wfRoot, "shared.wsl"), []byte("module shared\n"), 0o600)
	main := filepath.Join(wfRoot, "cli", "startup.wsl")
	_ = os.WriteFile(main, []byte("module startup\nextends consts\nimport shared\n"), 0o600)

	got := collectImports(main, "module startup\nextends consts\nimport shared\n")
	if _, ok := got["consts"]; !ok {
		t.Fatalf("expected 'consts' in imports: %v", got)
	}
	if _, ok := got["shared"]; !ok {
		t.Fatalf("expected 'shared' in imports: %v", got)
	}

	if collectImports("", "x") != nil || collectImports("/x.wsl", "") != nil {
		t.Fatal("empty inputs -> nil")
	}
}

func TestMetaSignature(t *testing.T) {
	m := interfaces.FunctionMetadata{
		Name:        "Set",
		ArgNames:    []string{"key", "value", "ttlSeconds"},
		ArgTypes:    []string{"string", "string", "int"},
		ReturnNames: []string{"r"},
		ReturnTypes: []string{"domain.FlowStepResult"},
	}
	if got := metaSignature(m); got != "Set(key: string, value: string, ttlSeconds: int) → r: domain.FlowStepResult" {
		t.Fatalf("sig: %q", got)
	}
	noRet := interfaces.FunctionMetadata{Name: "Ping"}
	if got := metaSignature(noRet); got != "Ping()" {
		t.Fatalf("no-ret sig: %q", got)
	}
}

func TestSynthModuleInfoFromCacheUnknown(t *testing.T) {
	if got := synthModuleInfoFromCache("github.com/x/y", "modules", "nope-ns", "nope-cls"); got != nil {
		t.Fatalf("unknown ns/class -> nil, got %#v", got)
	}
}

func TestMatchWorkflowNames(t *testing.T) {
	names := []string{"api_server/routes", "api_server/endpoints/health", "cli/help", "top"}

	eq := func(got, want []string) bool {
		if len(got) != len(want) {
			return false
		}
		for i := range got {
			if got[i] != want[i] {
				return false
			}
		}
		return true
	}

	if got := matchWorkflowNames(names, ""); !eq(got, names) {
		t.Fatalf("empty -> all: %v", got)
	}
	if got := matchWorkflowNames(names, "*"); !eq(got, names) {
		t.Fatalf("* -> all: %v", got)
	}
	if got := matchWorkflowNames(names, "api_server/routes"); !eq(got, []string{"api_server/routes"}) {
		t.Fatalf("exact: %v", got)
	}
	if got := matchWorkflowNames(names, "nope/nope"); len(got) != 0 {
		t.Fatalf("exact miss: %v", got)
	}
	if got := matchWorkflowNames(names, "api_server/*"); !eq(got, []string{"api_server/routes", "api_server/endpoints/health"}) {
		t.Fatalf("prefix glob -> everything below: %v", got)
	}
	if got := matchWorkflowNames(names, "*/help"); !eq(got, []string{"cli/help"}) {
		t.Fatalf("path.Match glob: %v", got)
	}
}

func TestUploadAllCommandHelp(t *testing.T) {
	w := &workflowTransitions{}
	boolThunk := func(v bool) func() *bool { return func() *bool { return &v } }

	// --help: renders the usage text, no panic despite a nil session.
	r := w.UploadAllCommand("", map[string]interface{}{"usage": "USAGE: kue wsl upload"},
		shared.KueConfig{}, nil, map[string]interface{}{"help": boolThunk(true)})
	if r.Error != nil {
		t.Fatalf("help err: %v", r.Error)
	}
	if out, _ := r.Response.(string); !strings.Contains(out, "USAGE: kue wsl upload") {
		t.Fatalf("help output: %v", r.Response)
	}
}

func TestPackageNameFromGoMod(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module github.com/kuetix/std-ai\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := packageNameFromGoMod(dir); got != "kuetix-std-ai" {
		t.Fatalf("package name = %q, want kuetix-std-ai", got)
	}
}
