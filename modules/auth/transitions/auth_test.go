package transitions

import (
	"encoding/json"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kuetix/kue/modules/shared"
)

func boolThunk(v bool) func() *bool    { return func() *bool { return &v } }
func strThunk(v string) func() *string { return func() *string { return &v } }

func TestLogoutClearsLogin(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := shared.SaveKueConfig(cfgPath, shared.KueConfig{
		Host:  "http://x.test",
		Login: map[string]interface{}{"token": "abc"},
	}); err != nil {
		t.Fatal(err)
	}

	a := &authTransitions{}
	r := a.LogoutCommand("", map[string]interface{}{"usage": ""}, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{}, map[string]interface{}{
			"help":   boolThunk(false),
			"config": strThunk(cfgPath),
		})
	if r.Error != nil || !r.Success {
		t.Fatalf("logout: %v", r.Error)
	}
	cfg, _ := shared.LoadKueConfig(cfgPath)
	if cfg.Login != nil {
		t.Fatalf("login data not cleared: %#v", cfg.Login)
	}
}

func TestRegisterValidatesAndPosts(t *testing.T) {
	var gotPath string
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		w.WriteHeader(http.StatusCreated)
	}))
	defer srv.Close()

	a := &authTransitions{}

	// Missing email/password -> error, no request.
	r := a.RegisterCommand("", map[string]interface{}{"usage": ""}, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{}, registerFlags("", "", srv.URL))
	if r.Error == nil {
		t.Fatal("expected validation error")
	}

	r = a.RegisterCommand("", map[string]interface{}{"usage": ""}, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{}, registerFlags("t@example.com", "pw", srv.URL))
	if r.Error != nil || !r.Success {
		t.Fatalf("register: %v", r.Error)
	}
	if gotPath != "/auth/register" || gotBody["email"] != "t@example.com" || gotBody["password"] != "pw" {
		t.Fatalf("request: path=%q body=%v", gotPath, gotBody)
	}
}

func registerFlags(email, pw, host string) map[string]interface{} {
	return map[string]interface{}{
		"help":     boolThunk(false),
		"email":    strThunk(email),
		"password": strThunk(pw),
		"name":     strThunk(""),
		"username": strThunk(""),
		"host":     strThunk(host),
		"config":   strThunk(""),
	}
}

func TestLoginPostsAndSavesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/login" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"token":"jwt-xyz"}`))
	}))
	defer srv.Close()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	a := &authTransitions{}
	r := a.LoginCommand("", map[string]interface{}{"usage": ""}, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{}, map[string]interface{}{
			"help":      boolThunk(false),
			"host":      strThunk(srv.URL),
			"username":  strThunk("tester"),
			"password":  strThunk("pw"),
			"config":    strThunk(cfgPath),
			"workflows": strThunk(""),
		})
	if r.Error != nil || !r.Success {
		t.Fatalf("login: %v", r.Error)
	}
	cfg, _ := shared.LoadKueConfig(cfgPath)
	if shared.GetLoginToken(cfg) != "jwt-xyz" {
		t.Fatalf("token not saved: %#v", cfg.Login)
	}
}

func TestProfileGetRequiresAuthAndReturnsBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"username":"tester"}`))
	}))
	defer srv.Close()

	a := &authTransitions{}
	flags := map[string]interface{}{"help": boolThunk(false)}

	r := a.ProfileGetCommand("", map[string]interface{}{"usage": ""}, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{Host: srv.URL, Login: map[string]interface{}{"token": "tok"}}, flags)
	if r.Error != nil {
		t.Fatalf("profile get: %v", r.Error)
	}
	if out, _ := r.Response.(string); !strings.Contains(out, "tester") {
		t.Fatalf("body: %v", r.Response)
	}

	// No token -> unauthorized error.
	r = a.ProfileGetCommand("", map[string]interface{}{"usage": ""}, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{Host: srv.URL}, flags)
	if r.Error == nil {
		t.Fatal("expected auth error without a token")
	}
}

func TestWhoamiCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"email":"a@b.test","userId":"u_42"}}`))
	}))
	defer srv.Close()

	a := &authTransitions{}
	base := map[string]interface{}{"help": boolThunk(false), "json": boolThunk(false), "host": strThunk(""), "config": strThunk("")}
	cfg := map[string]interface{}{"usage": ""}

	// Not logged in -> local answer, no error, mentions "Not logged in".
	r := a.WhoamiCommand("", cfg, flag.NewFlagSet("x", flag.ContinueOnError), shared.KueConfig{Host: srv.URL}, base)
	if r.Error != nil {
		t.Fatalf("whoami (logged out): %v", r.Error)
	}
	if out, _ := r.Response.(string); !strings.Contains(out, "Not logged in") {
		t.Fatalf("logged-out body: %v", r.Response)
	}

	// Logged in -> hits /profile, shows the email + id.
	r = a.WhoamiCommand("", cfg, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{Host: srv.URL, Login: map[string]interface{}{"token": "tok"}}, base)
	if r.Error != nil {
		t.Fatalf("whoami (logged in): %v", r.Error)
	}
	out, _ := r.Response.(string)
	if !strings.Contains(out, "a@b.test") || !strings.Contains(out, "u_42") {
		t.Fatalf("logged-in body: %v", r.Response)
	}

	// --json passes the raw body straight through.
	jsonFlags := map[string]interface{}{"help": boolThunk(false), "json": boolThunk(true), "host": strThunk(""), "config": strThunk("")}
	r = a.WhoamiCommand("", cfg, flag.NewFlagSet("x", flag.ContinueOnError),
		shared.KueConfig{Host: srv.URL, Login: map[string]interface{}{"token": "tok"}}, jsonFlags)
	if out, _ := r.Response.(string); !strings.Contains(out, `"userId":"u_42"`) {
		t.Fatalf("json body: %v", r.Response)
	}
}
