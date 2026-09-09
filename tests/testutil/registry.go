package testutil

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// RecordedRequest is one request the mock registry received.
type RecordedRequest struct {
	Method string
	Path   string // path only, no query
	Query  string // raw query string
	Header http.Header
	Body   []byte
}

// MockRegistry is an in-process stand-in for api.kuetix.com. Point kue at it
// with KUE_HOST or KueConfig.Host = reg.URL (it speaks plain HTTP).
type MockRegistry struct {
	*httptest.Server

	mu       sync.Mutex
	requests []RecordedRequest

	// Handlers maps "METHOD /path" (path without query) to a handler. A test
	// may add or replace entries before issuing requests. When no entry
	// matches, defaultHandler is used.
	Handlers map[string]http.HandlerFunc
}

// NewMockRegistry starts a mock registry and stops it via t.Cleanup.
func NewMockRegistry(t *testing.T) *MockRegistry {
	t.Helper()
	m := &MockRegistry{Handlers: map[string]http.HandlerFunc{}}
	m.Server = httptest.NewServer(http.HandlerFunc(m.route))
	t.Cleanup(m.Server.Close)
	return m
}

// Requests returns a copy of everything received so far.
func (m *MockRegistry) Requests() []RecordedRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]RecordedRequest(nil), m.requests...)
}

// LastRequest returns the most recent request, or a zero value if none.
func (m *MockRegistry) LastRequest() RecordedRequest {
	rs := m.Requests()
	if len(rs) == 0 {
		return RecordedRequest{}
	}
	return rs[len(rs)-1]
}

// Called reports whether any recorded request matched method+path exactly.
func (m *MockRegistry) Called(method, path string) bool {
	for _, r := range m.Requests() {
		if r.Method == method && r.Path == path {
			return true
		}
	}
	return false
}

// SeedWorkflow makes GET /workflows/get and GET /workflow/<name> return this
// content with a correct SHA-256 so kue's integrity check passes.
func (m *MockRegistry) SeedWorkflow(name, content string, version int) {
	sum := sha256.Sum256([]byte(content))
	hash := hex.EncodeToString(sum[:])
	payload := map[string]interface{}{
		"data": map[string]interface{}{
			"name":         name,
			"owner":        "acme",
			"version":      version,
			"hash":         hash,
			"content":      content,
			"actions":      []interface{}{},
			"dependencies": []interface{}{},
		},
	}
	body, _ := json.Marshal(payload)
	h := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
	m.Handlers["GET /workflows/get"] = h
	m.Handlers["GET /workflow/"+name] = h
}

func (m *MockRegistry) route(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	_ = r.Body.Close()

	m.mu.Lock()
	m.requests = append(m.requests, RecordedRequest{
		Method: r.Method,
		Path:   r.URL.Path,
		Query:  r.URL.RawQuery,
		Header: r.Header.Clone(),
		Body:   body,
	})
	m.mu.Unlock()

	if h, ok := m.Handlers[r.Method+" "+r.URL.Path]; ok {
		h(w, r)
		return
	}
	// Prefix match for "/workflow/<name>" style routes registered without the
	// trailing segment.
	for key, h := range m.Handlers {
		parts := strings.SplitN(key, " ", 2)
		if len(parts) == 2 && parts[0] == r.Method && strings.HasPrefix(r.URL.Path, parts[1]) {
			h(w, r)
			return
		}
	}
	m.defaultHandler(w, r)
}

func (m *MockRegistry) defaultHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/auth/login":
		_, _ = w.Write([]byte(`{"data":{"token":"test-jwt-token"}}`))
	case r.URL.Path == "/auth/register":
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"id":"u_1"}}`))
	case strings.HasPrefix(r.URL.Path, "/packages/search"):
		_, _ = w.Write([]byte(`{"data":{"items":[{"name":"acme/util","description":"demo","version":"1.0.0"}]}}`))
	case strings.HasPrefix(r.URL.Path, "/packages/install"):
		_, _ = w.Write([]byte(`{"data":{"content":"module x\n","hash":"","version":1}}`))
	case r.URL.Path == "/package/publish":
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"data":{"version":"1.0.0","validated_by_engine":"v1.2.0"}}`))
	case r.URL.Path == "/profile":
		_, _ = w.Write([]byte(`{"data":{"username":"tester","email":"t@example.com"}}`))
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"no mock route: ` + r.Method + " " + r.URL.Path + `"}`))
	}
}
