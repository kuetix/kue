package transitions

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kuetix/kue/modules/shared"
)

func TestParseWorkflowSearchResponse(t *testing.T) {
	hits, total, err := parseWorkflowSearchResponse(`{"data":{"workflows":[
		{"name":"acme/one","owner":"u1","version":2,"public":true,"actions_count":3,"dependencies_count":1,"matched_actions":["ai/prompt.Prompt"]},
		{"name":"","owner":"u1"}
	],"total":5}}`)
	if err != nil {
		t.Fatal(err)
	}
	if total != 5 {
		t.Fatalf("total = %d", total)
	}
	if len(hits) != 1 || hits[0].Name != "acme/one" || hits[0].Owner != "u1" || hits[0].Version != 2 || !hits[0].Public {
		t.Fatalf("hits = %#v", hits)
	}
	if strings.Join(hits[0].MatchedActions, ",") != "ai/prompt.Prompt" {
		t.Fatalf("matched actions = %v", hits[0].MatchedActions)
	}

	if _, _, err := parseWorkflowSearchResponse(`{bad`); err == nil {
		t.Fatal("expected JSON error")
	}
	if hits, total, err := parseWorkflowSearchResponse("  "); err != nil || hits != nil || total != 0 {
		t.Fatalf("empty body: %v %d %v", hits, total, err)
	}
}

func TestRenderWorkflowHits(t *testing.T) {
	if s := renderWorkflowHits("q", nil, 0, 10); !strings.Contains(s, `No workflows match "q"`) {
		t.Fatalf("empty: %q", s)
	}
	s := renderWorkflowHits("q", []workflowHit{{Name: "acme/a", Version: 1, Owner: "u1", Public: true, Project: "github.com/acme/lib"}}, 1, 10)
	if !strings.Contains(s, "Found 1 match") || !strings.Contains(s, "acme/a  v1  (public, owner=u1)") {
		t.Fatalf("one: %q", s)
	}
	if !strings.Contains(s, "project: github.com/acme/lib") {
		t.Fatalf("project line missing: %q", s)
	}
	if !strings.Contains(s, "page:    https://pkg.kuetix.com/workflows/acme/a?owner=u1") {
		t.Fatalf("page url missing: %q", s)
	}
	hits := []workflowHit{
		{Name: "acme/a", Version: 1, Public: false, MatchedActions: []string{"foo/bar.Baz"}},
		{Name: "acme/b", Version: 2, Public: true},
	}
	s = renderWorkflowHits("q", hits, 2, 10)
	if !strings.Contains(s, "Found 2 matches") || !strings.Contains(s, "(private)") {
		t.Fatalf("two: %q", s)
	}
	if !strings.Contains(s, "matched actions: foo/bar.Baz") {
		t.Fatalf("matched actions line missing: %q", s)
	}
}

func TestFirstPositionalAndOpts(t *testing.T) {
	if got := firstPositional(map[string]interface{}{"args": []string{"  q  "}}, nil); got != "q" {
		t.Fatalf("got %q", got)
	}
	if got := firstPositional(map[string]interface{}{}, nil); got != "" {
		t.Fatalf("empty got %q", got)
	}
	if got := intOpt(map[string]interface{}{"limit": 5}, "limit", 10); got != 5 {
		t.Fatalf("int: %d", got)
	}
	if got := intOpt(map[string]interface{}{"limit": float64(7)}, "limit", 10); got != 7 {
		t.Fatalf("float64: %d", got)
	}
	if got := intOpt(map[string]interface{}{"limit": ""}, "limit", 10); got != 10 {
		t.Fatalf("empty string -> fallback: %d", got)
	}
}

func TestQueryPositional(t *testing.T) {
	// Normal form: query in args.
	if got := queryPositional(map[string]interface{}{"args": []string{" auth "}}, nil); got != "auth" {
		t.Fatalf("args form: %q", got)
	}
	// `kue wsl <query>` — parsed as command "wsl.<query>" with empty args.
	req := map[string]interface{}{
		"command":      "wsl.cli/project",
		"main_command": "wsl",
		"args":         []string{},
	}
	if got := queryPositional(req, nil); got != "cli/project" {
		t.Fatalf("reconstructed form: %q", got)
	}
	// Bare `kue wsl` — nothing to search.
	if got := queryPositional(map[string]interface{}{"command": "wsl", "main_command": "wsl"}, nil); got != "" {
		t.Fatalf("bare: %q", got)
	}
	// Wildcard token is not a query.
	if got := queryPositional(map[string]interface{}{"command": "wsl.*", "main_command": "wsl"}, nil); got != "" {
		t.Fatalf("wildcard: %q", got)
	}
	// --search/-s is explicit and wins over everything, including a token that
	// the CLI routed as a subcommand (`kue wsl --search get`).
	if got := queryPositional(
		map[string]interface{}{"command": "wsl.get", "main_command": "wsl", "args": []string{}},
		map[string]interface{}{"search": "get"},
	); got != "get" {
		t.Fatalf("--search override: %q", got)
	}
	if got := queryPositional(
		map[string]interface{}{"args": []string{"ignored"}},
		map[string]interface{}{"search": "  chosen  "},
	); got != "chosen" {
		t.Fatalf("--search beats positional: %q", got)
	}
}

func boolThunk(v bool) func() *bool    { return func() *bool { return &v } }
func strThunk(v string) func() *string { return func() *string { return &v } }

// TestSearchWorkflowCommandHitsRegistryAnonymously guards against a
// regression where `kue wsl <query>` paginated the caller's own /workflow
// list (requiring login, and never finding anyone else's public workflow)
// instead of calling the public /workflows/search registry endpoint. With no
// login configured, the request must still succeed and must not carry an
// Authorization header — proving the "no login required" contract holds.
func TestSearchWorkflowCommandHitsRegistryAnonymously(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path + "?" + r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"data":{"workflows":[{"name":"ai/prompt","owner":"someone-else","version":1,"public":true}],"total":1}}`))
	}))
	defer srv.Close()

	s := &searchTransitions{}
	r := s.SearchWorkflowCommand(
		map[string]interface{}{"args": []string{"ai"}},
		map[string]interface{}{"usage": ""},
		shared.KueConfig{Host: srv.URL}, // no Login token — anonymous caller
		nil,
		map[string]interface{}{"help": boolThunk(false), "limit": strThunk("")},
	)
	if r.Error != nil {
		t.Fatalf("err: %v", r.Error)
	}
	if !strings.Contains(gotPath, "/workflows/search?q=ai") {
		t.Fatalf("registry path = %q", gotPath)
	}
	if gotAuth != "" {
		t.Fatalf("expected no Authorization header for an anonymous search, got %q", gotAuth)
	}
	out, _ := r.Response.(string)
	if !strings.Contains(out, "ai/prompt") || !strings.Contains(out, "owner=someone-else") {
		t.Fatalf("response = %v", r.Response)
	}
}
