package shared

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeHost(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		// A bare host — no scheme at all — is a real domain, not a local dev
		// server, and must default to https. Getting this wrong is a serious,
		// silent bug: nginx (or any TLS-terminating proxy) 301/302-redirects
		// plain HTTP to HTTPS, and net/http replays that redirect as a GET
		// with the body dropped for any non-GET method — so a POST upload
		// over http:// "succeeds" (200 from the followed GET) while never
		// reaching the create/update endpoint at all.
		{"", "https://" + DefaultAPIHost},
		{"api.kuetix.com", "https://api.kuetix.com"},
		// An explicit http:// prefix is still honored as-is — needed for
		// local/dev servers that genuinely run plaintext (every e2e test's
		// httptest.Server URL, "http://localhost:PORT", relies on this).
		{"http://api.kuetix.com", "http://api.kuetix.com"},
		{"https://api.kuetix.com", "https://api.kuetix.com"},
		{"  https://x.test/  ", "https://x.test/"},
		{"  HTTP://X.TEST  ", "http://X.TEST"},
	}
	for _, tc := range cases {
		if got := NormalizeHost(tc.in); got != tc.want {
			t.Errorf("NormalizeHost(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestIsHostUseSecure(t *testing.T) {
	if !IsHostUseSecure("https://x") {
		t.Error("https:// should be secure")
	}
	if IsHostUseSecure("http://x") || IsHostUseSecure("") || IsHostUseSecure("x") {
		t.Error("non-https should not be secure")
	}
}

func TestResolveAPIHost(t *testing.T) {
	t.Setenv("KUE_HOST", "")
	// A bare host — from the default, an env var, or an explicit --host
	// argument, all without a scheme — resolves to https://, matching
	// NormalizeHost("") and every other bare-host case (see TestNormalizeHost).
	if got := ResolveAPIHost(); got != "https://"+DefaultAPIHost {
		t.Errorf("default: got %q", got)
	}
	if got := ResolveAPIHost("", "  ", "example.test"); got != "https://example.test" {
		t.Errorf("explicit arg: got %q", got)
	}
	t.Setenv("KUE_HOST", "env.test")
	if got := ResolveAPIHost(); got != "https://env.test" {
		t.Errorf("env: got %q", got)
	}
	if got := ResolveAPIHost("arg.test"); got != "https://arg.test" {
		t.Errorf("arg beats env: got %q", got)
	}
}

func TestIsHostProvided(t *testing.T) {
	t.Setenv("KUE_HOST", "")
	if IsHostProvided("", "   ") {
		t.Error("blank args, no env: not provided")
	}
	if !IsHostProvided("x") {
		t.Error("arg present")
	}
	t.Setenv("KUE_HOST", "y")
	if !IsHostProvided() {
		t.Error("env present")
	}
}

func TestResolveConfigPath(t *testing.T) {
	t.Setenv("KUE_CONFIG_PATH", "")
	if got := ResolveConfigPath("  ", "/explicit/path.json"); got != "/explicit/path.json" {
		t.Errorf("explicit: got %q", got)
	}
	t.Setenv("KUE_CONFIG_PATH", "/from/env.json")
	if got := ResolveConfigPath(); got != "/from/env.json" {
		t.Errorf("env: got %q", got)
	}
	if got := ResolveConfigPath("/win.json"); got != "/win.json" {
		t.Errorf("arg beats env: got %q", got)
	}
	t.Setenv("KUE_CONFIG_PATH", "")
	if got := ResolveConfigPath(); got != DefaultKueConfigPath() {
		t.Errorf("default: got %q", got)
	}
}

func TestLoadSaveKueConfigRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")

	// Missing file -> zero config, no error.
	if cfg, err := LoadKueConfig(path); err != nil || cfg.Host != "" {
		t.Fatalf("missing file: cfg=%#v err=%v", cfg, err)
	}

	want := KueConfig{Host: "https://api.example.test", Secure: true, Login: map[string]interface{}{"token": "abc123"}}
	if err := SaveKueConfig(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadKueConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Host != want.Host || got.Secure != want.Secure || got.Login["token"] != "abc123" {
		t.Fatalf("round-trip mismatch: %#v", got)
	}

	// Empty file -> zero config, no error.
	empty := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(empty, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, err := LoadKueConfig(empty); err != nil || cfg.Host != "" {
		t.Fatalf("empty file: cfg=%#v err=%v", cfg, err)
	}

	// Garbage -> error.
	bad := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(bad, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadKueConfig(bad); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestGetLoginToken(t *testing.T) {
	cases := []struct {
		name string
		cfg  KueConfig
		want string
	}{
		{"nil login", KueConfig{}, ""},
		{"flat token", KueConfig{Login: map[string]interface{}{"token": "  t1  "}}, "t1"},
		{"jwt key", KueConfig{Login: map[string]interface{}{"jwt": "t2"}}, "t2"},
		{"nested data.token", KueConfig{Login: map[string]interface{}{"data": map[string]interface{}{"token": "t3"}}}, "t3"},
		{"nested access_token", KueConfig{Login: map[string]interface{}{"data": map[string]interface{}{"access_token": "t4"}}}, "t4"},
		{"absent", KueConfig{Login: map[string]interface{}{"other": "x"}}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := GetLoginToken(tc.cfg); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := FirstNonEmpty("", "  ", "  x ", "y"); got != "x" {
		t.Fatalf("got %q", got)
	}
	if got := FirstNonEmpty("", "   "); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestReadPasswordFromInput(t *testing.T) {
	got, err := ReadPasswordFromInput(strings.NewReader("s3cret\n"), io.Discard)
	if err != nil || got != "s3cret" {
		t.Fatalf("got %q err %v", got, err)
	}
	got, err = ReadPasswordFromInput(strings.NewReader("noeol"), io.Discard)
	if err != nil || got != "noeol" {
		t.Fatalf("no-EOL: got %q err %v", got, err)
	}
}

func TestPerformRequestAuthAndStatus(t *testing.T) {
	var gotAuth, gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte(`{"data":"yes"}`))
		case "/boom":
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte(`nope`))
		}
	}))
	defer srv.Close()

	cfg := KueConfig{Host: srv.URL, Login: map[string]interface{}{"token": "tok"}}

	body, status, err := PerformAuthenticatedRequest(cfg, http.MethodPost, "/ok", map[string]string{"a": "b"})
	if err != nil || status != http.StatusOK || !strings.Contains(body, "yes") {
		t.Fatalf("ok: body=%q status=%d err=%v", body, status, err)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth header = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Fatalf("content-type = %q", gotCT)
	}

	_, status, err = PerformAuthenticatedRequest(cfg, http.MethodGet, "/boom", nil)
	if err == nil || status != http.StatusTeapot {
		t.Fatalf("boom: status=%d err=%v", status, err)
	}

	// No token -> 401 without hitting the network.
	if _, status, err := PerformAuthenticatedRequest(KueConfig{Host: srv.URL}, http.MethodGet, "/ok", nil); err == nil || status != http.StatusUnauthorized {
		t.Fatalf("no-token: status=%d err=%v", status, err)
	}
}

// TestPerformRequestRefusesRedirectOnWrite guards against a serious, silent
// correctness bug: net/http replays a 301/302/303 redirect on a non-GET
// request as a GET with the body dropped. A POST/PUT/DELETE against a host
// that redirects (e.g. an http:// request to a server that enforces HTTPS —
// see TestNormalizeHost) would therefore appear to "succeed" (200, from the
// followed GET) while the write never reached its endpoint at all. GET
// requests still follow redirects normally.
func TestPerformRequestRefusesRedirectOnWrite(t *testing.T) {
	var getHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/workflow":
			http.Redirect(w, r, "/workflow/", http.StatusFound)
		case "/workflow/":
			getHits++
			_, _ = w.Write([]byte(`{"data":{"workflows":[]}}`))
		}
	}))
	defer srv.Close()

	cfg := KueConfig{Host: srv.URL, Login: map[string]interface{}{"token": "tok"}}

	_, status, err := PerformAuthenticatedRequest(cfg, http.MethodPost, "/workflow", map[string]string{"name": "x"})
	if err == nil {
		t.Fatal("expected an error refusing to follow the redirect on POST")
	}
	if !strings.Contains(err.Error(), "refusing to follow") {
		t.Fatalf("error should explain the redirect refusal, got: %v", err)
	}
	if status != http.StatusFound {
		t.Fatalf("status = %d, want %d (the redirect itself)", status, http.StatusFound)
	}
	if getHits != 0 {
		t.Fatalf("the redirect target must never be reached for a refused write, got %d hits", getHits)
	}

	// A GET redirect is harmless and still followed.
	body, status, err := PerformAuthenticatedRequest(cfg, http.MethodGet, "/workflow", nil)
	if err != nil || status != http.StatusOK || !strings.Contains(body, "workflows") {
		t.Fatalf("GET redirect: body=%q status=%d err=%v", body, status, err)
	}
	if getHits != 1 {
		t.Fatalf("expected the redirect target to be reached once for GET, got %d", getHits)
	}
}

func TestPostJSONStatusMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "1" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	if err := PostJSON(srv.URL, "/x", map[string]int{"n": 1}, map[string]string{"X-Custom": "1"}); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if err := PostJSON(srv.URL, "/x", nil, nil); err == nil {
		t.Fatal("expected error for 400 response")
	}
}
