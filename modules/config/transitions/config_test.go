package transitions

import (
	"path/filepath"
	"testing"

	"github.com/kuetix/kue/modules/shared"
)

func resolve(t *testing.T, options map[string]interface{}) map[string]interface{} {
	t.Helper()
	w := &configTransitions{}
	r := w.Resolve(options)
	if !r.Success {
		t.Fatalf("Resolve failed: %v", r.Error)
	}
	m, ok := r.Response.(map[string]interface{})
	if !ok {
		t.Fatalf("Resolve response not a map: %#v", r.Response)
	}
	return m
}

func TestResolveReadsConfigAndToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := shared.SaveKueConfig(path, shared.KueConfig{
		Host:  "http://stored.test",
		Login: map[string]interface{}{"data": map[string]interface{}{"token": "nested-tok"}},
	}); err != nil {
		t.Fatal(err)
	}

	m := resolve(t, map[string]interface{}{"config": path})
	if m["host"] != "http://stored.test" {
		t.Errorf("host: got %v (stored host should be used when no --host given)", m["host"])
	}
	if m["token"] != "nested-tok" {
		t.Errorf("token: got %v, want nested-tok", m["token"])
	}
	if _, ok := m["kueConfig"].(shared.KueConfig); !ok {
		t.Errorf("kueConfig type: %T", m["kueConfig"])
	}
}

func TestResolveHostFlagBeatsStored(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = shared.SaveKueConfig(path, shared.KueConfig{Host: "http://stored.test"})

	// A bare --host value has no scheme, so it resolves to https:// — see
	// shared.NormalizeHost.
	m := resolve(t, map[string]interface{}{"config": path, "host": "override.test"})
	if m["host"] != "https://override.test" {
		t.Errorf("host: got %v, want https://override.test", m["host"])
	}
}

func TestResolveMissingConfigIsNotAnError(t *testing.T) {
	m := resolve(t, map[string]interface{}{"config": filepath.Join(t.TempDir(), "absent.json")})
	if m["token"] != "" {
		t.Errorf("token should be empty, got %v", m["token"])
	}
}

// TestResolveAcceptsGetFlagsClosures guards against a regression where every
// workflows/cli/*/*.wsl "Config" state called config/config.Resolve(options:
// {}) with a hardcoded empty literal, so --host/--config could never reach
// this transition at all (kueConfig.Host silently fell back to a stored
// config file, KUE_HOST, or production). Those states now pass
// $config.flags, whose values are GetFlags' getter closures — not the plain
// strings the tests above use — so strOpt must resolve both shapes.
func TestResolveAcceptsGetFlagsClosures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = shared.SaveKueConfig(path, shared.KueConfig{Host: "http://stored.test"})

	host := "http://override.test"
	cfg := path
	m := resolve(t, map[string]interface{}{
		"host":   func() *string { return &host },
		"config": func() *string { return &cfg },
	})
	if m["host"] != "http://override.test" {
		t.Errorf("host: got %v, want http://override.test", m["host"])
	}
}
