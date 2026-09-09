//go:build e2e

package e2e

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuetix/kue/modules/shared"
	"github.com/kuetix/kue/tests/testutil"
)

// registryEnv returns the env slice pointing the CLI at an isolated HOME and
// the mock registry.
func registryEnv(t *testing.T, reg *testutil.MockRegistry) (home string, env []string) {
	t.Helper()
	home = t.TempDir()
	if err := shared.SaveKueConfig(filepath.Join(home, ".kue", "config.json"), shared.KueConfig{Host: reg.URL}); err != nil {
		t.Fatal(err)
	}
	// Pre-seed the template cache so `modules.Enable()` in the child process
	// doesn't reach out to templates.kuetix.com — keeps these tests hermetic
	// and fast on a fresh HOME.
	if err := os.MkdirAll(filepath.Join(home, ".kue", "templates", "web", "latest"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, []string{"HOME=" + home, "USERPROFILE=" + home, "KUE_HOST=" + reg.URL}
}

// TestHostFlagAloneReachesRegistry guards against a regression where every
// workflows/cli/*/*.wsl "Config" state called config/config.Resolve(options:
// {}) with a hardcoded empty literal, so the --host flag never reached that
// transition — kueConfig.Host fell back to a stored config file, KUE_HOST,
// or (worst case, with neither of those set) production api.kuetix.com.
// Here HOME is isolated with no ~/.kue/config.json and KUE_HOST is unset, so
// --host is the only thing that can possibly point the request at the mock
// registry.
func TestHostFlagAloneReachesRegistry(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	home := t.TempDir()
	// No config file, no KUE_HOST — only HOME is isolated (so a real
	// ~/.kue/config.json on the machine running the test can't leak in) and
	// the template cache is pre-seeded so modules.Enable() stays hermetic.
	if err := os.MkdirAll(filepath.Join(home, ".kue", "templates", "web", "latest"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "USERPROFILE=" + home, "KUE_HOST="}

	res := testutil.RunCLI(t, "", env, "package", "search", "util", "--host", reg.URL)
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("GET", "/packages/search") {
		t.Fatalf("--host alone did not route the request to the mock registry; requests=%+v", reg.Requests())
	}
}

// TestGlobalVerboseFlagBeforeCommandStillDispatches guards against a
// regression in the argument tokenizer where a global boolean flag (-v,
// --debug, -q, -h) placed before the command word was treated as if it
// needed a separately-tokenized value, consuming the command word itself —
// `kue -v wsl status` resolved to the bogus command "status" instead of
// "wsl.status" ("wsl" having been swallowed as -v's "value").
func TestGlobalVerboseFlagBeforeCommandStillDispatches(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	_, env := registryEnv(t, reg)

	res := testutil.RunCLI(t, "", env, "-v", "package", "search", "util")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("GET", "/packages/search") {
		t.Fatalf("-v before the command broke dispatch; requests=%+v", reg.Requests())
	}
}

// TestUploadBoolFlagBeforePositionalStillFindsName guards against a
// regression in the argument tokenizer where a boolean flag placed before
// its command's positional argument swallowed that argument as the flag's
// own value — `kue wsl upload --public acme_flagorder` must still see
// "acme_flagorder" as the filter query (uploading exactly that workflow),
// not lose it into the raw option tokens.
func TestUploadBoolFlagBeforePositionalStillFindsName(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	home := t.TempDir()
	if err := shared.SaveKueConfig(filepath.Join(home, ".kue", "config.json"), shared.KueConfig{
		Host:  reg.URL,
		Login: map[string]interface{}{"token": "test-jwt-token"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".kue", "templates", "web", "latest"), 0o755); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "USERPROFILE=" + home, "KUE_HOST=" + reg.URL}
	reg.Handlers["POST /workflow"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"name":"acme/flagorder","version":1}}`))
	}

	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "workflows"), 0o755); err != nil {
		t.Fatal(err)
	}
	const content = "module flagorder\n\nworkflow flagorder {\n  start: A\n  state A {\n    end ok\n  }\n}\n"
	if err := os.WriteFile(filepath.Join(project, "workflows", "acme_flagorder.wsl"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	res := testutil.RunCLI(t, project, env, "wsl", "upload", "--public", "acme_flagorder")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("POST", "/workflow") {
		t.Fatalf("--public before the name broke upload; requests=%+v", reg.Requests())
	}
	// The query matched exactly the one local workflow — not zero (name
	// swallowed by --public) and not "all".
	if !strings.Contains(res.Combined(), "1 uploaded") || strings.Contains(res.Combined(), "of 0)") {
		t.Fatalf("expected exactly one workflow uploaded:\n%s", res.Combined())
	}
}

// TestWslSearchFindsPublicWorkflowAnonymously guards against a regression
// where `kue wsl <query>` paginated the caller's own /workflow list (which
// requires login and only ever contains that caller's own workflows) instead
// of the public /workflows/search registry endpoint. Here the CLI runs with
// no login configured at all — the search must still succeed and find a
// workflow "uploaded" by someone else.
func TestWslSearchFindsPublicWorkflowAnonymously(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	_, env := registryEnv(t, reg) // no login token in this config
	reg.Handlers["GET /workflows/search"] = func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("anonymous search sent an Authorization header: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"workflows":[
			{"name":"ai/prompt","owner":"someone-else","version":1,"public":true,"actions_count":2,"dependencies_count":1}
		],"count":1,"total":1}}`))
	}

	res := testutil.RunCLI(t, "", env, "wsl", "ai")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("GET", "/workflows/search") {
		t.Fatalf("registry search endpoint was not called; requests=%+v", reg.Requests())
	}
	if !strings.Contains(res.Combined(), "ai/prompt") {
		t.Fatalf("expected ai/prompt in output:\n%s", res.Combined())
	}
}

// TestWslSearchFlagOverridesSubcommand: `kue wsl get` dispatches to the `get`
// subcommand, but `kue wsl --search get` must run a registry search for the
// literal query "get" — the escape hatch for queries that collide with a
// reserved subcommand name (and the explicit form for scripts/CI).
func TestWslSearchFlagOverridesSubcommand(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	_, env := registryEnv(t, reg)
	var gotQuery string
	reg.Handlers["GET /workflows/search"] = func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"workflows":[{"name":"acme/getter","version":1,"public":true}],"total":1}}`))
	}

	res := testutil.RunCLI(t, "", env, "wsl", "--search", "get")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("GET", "/workflows/search") || gotQuery != "get" {
		t.Fatalf("--search get did not search for %q (q=%q); requests=%+v", "get", gotQuery, reg.Requests())
	}
	if reg.Called("GET", "/workflow/get") {
		t.Fatalf("--search get wrongly dispatched to the `get` subcommand")
	}
	if !strings.Contains(res.Combined(), "acme/getter") {
		t.Fatalf("expected search results:\n%s", res.Combined())
	}
}

func TestSearchPackages(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	_, env := registryEnv(t, reg)

	res := testutil.RunCLI(t, "", env, "package", "search", "util")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("GET", "/packages/search") {
		t.Fatalf("registry search endpoint was not called; requests=%+v", reg.Requests())
	}
	if !strings.Contains(res.Combined(), "acme/util") {
		t.Fatalf("expected mock result in output:\n%s", res.Combined())
	}
}

func TestInstallCachesWorkflow(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	home, env := registryEnv(t, reg)

	const content = "module cached\n\nworkflow cached {\n  start: A\n  state A {\n    end ok\n  }\n}\n"
	reg.SeedWorkflow("acme/cached", content, 1)

	res := testutil.RunCLI(t, "", env, "install", "acme/cached")
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}
	if !reg.Called("GET", "/workflows/get") {
		t.Fatalf("registry fetch endpoint was not called; requests=%+v", reg.Requests())
	}

	// The workflow must now sit in the isolated cache.
	var found bool
	_ = filepath.WalkDir(filepath.Join(home, ".kue"), func(path string, d os.DirEntry, _ error) error {
		if d != nil && !d.IsDir() && strings.Contains(path, "cached") && strings.HasSuffix(path, ".wsl") {
			found = true
		}
		return nil
	})
	if !found {
		t.Fatalf("cached workflow file not found under %s/.kue", home)
	}
}

func TestInstallIntegrityMismatchFails(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	_, env := registryEnv(t, reg)

	// Serve content whose hash won't match the advertised one.
	reg.Handlers["GET /workflows/get"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"name":"acme/bad","owner":"acme","version":1,"hash":"deadbeef","content":"module bad\n"}}`))
	}

	res := testutil.RunCLI(t, "", env, "install", "acme/bad")
	if res.ExitCode == 0 {
		t.Fatalf("integrity mismatch should fail the install\n%s", res.Combined())
	}
}

// TestInstallRespectsOutputFlag guards against a regression in the CLI's
// dispatch layer (std-cli's RegisterCommands): a bare `<command> <name>`
// invocation with no real subcommand keyword — `install <name>`, like
// `run <target>` — used to have every option after the name (--output,
// --owner, --host, --check, ...) silently unparsed and stuck at its
// JSON-declared default, because the flag-registration gate compared against
// the pre-wildcard-resolution command string. --output would then quietly
// fall back to ".", so the workflow landed under the process's current
// working directory instead of the requested project.
func TestInstallRespectsOutputFlag(t *testing.T) {
	reg := testutil.NewMockRegistry(t)
	// A logged-in config: install only takes the workflow (rather than
	// falling back to the anonymous package) branch when a login token is
	// present — see installTransitions.InstallCommand.
	home := t.TempDir()
	if err := shared.SaveKueConfig(filepath.Join(home, ".kue", "config.json"), shared.KueConfig{
		Host:  reg.URL,
		Login: map[string]interface{}{"token": "test-jwt-token"},
	}); err != nil {
		t.Fatal(err)
	}
	env := []string{"HOME=" + home, "USERPROFILE=" + home, "KUE_HOST=" + reg.URL}

	const content = "module cached\n\nworkflow cached {\n  start: A\n  state A {\n    end ok\n  }\n}\n"
	reg.SeedWorkflow("acme/cached", content, 1)
	// `install` (logged in) first lists the caller's own workflows to expand
	// glob patterns; an empty list falls back to treating the name as exact.
	reg.Handlers["GET /workflow"] = func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"workflows":[],"count":0}}`))
	}

	// A project directory distinct from the test binary's own working
	// directory, so a regression that ignores --output and falls back to cwd
	// is caught rather than accidentally masked.
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "go.mod"), []byte("module example.com/proj\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	res := testutil.RunCLI(t, "", env, "install", "acme/cached", "--output", projectDir)
	if res.ExitCode != 0 {
		t.Fatalf("exit %d\n%s", res.ExitCode, res.Combined())
	}

	wsl := filepath.Join(projectDir, "workflows", "acme", "cached.wsl")
	if _, err := os.Stat(wsl); err != nil {
		t.Fatalf("expected --output to place the workflow at %s: %v\n%s", wsl, err, res.Combined())
	}
}
