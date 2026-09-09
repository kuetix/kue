package transitions

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseWorkflowArg(t *testing.T) {
	sub, name := parseWorkflowArg("acme/sub/flow")
	if sub != "acme/sub" || name != "flow" {
		t.Fatalf("got %q %q", sub, name)
	}
	if sub, name := parseWorkflowArg("bare"); sub != "cli" || name != "bare" {
		t.Fatalf("bare -> cli/bare, got %q %q", sub, name)
	}
}

func TestParseTransitionArg(t *testing.T) {
	p, n := parseTransitionArg("modules/orders/ship")
	if p != "modules/orders" || n != "ship" {
		t.Fatalf("got %q %q", p, n)
	}
	if p, n := parseTransitionArg("ship"); p != "" || n != "ship" {
		t.Fatalf("no slash: %q %q", p, n)
	}
}

func TestParseDotNotation(t *testing.T) {
	m, tr := parseDotNotation("orders.Create", "fallback")
	if m != "orders" || tr != "Create" {
		t.Fatalf("dotted: %q %q", m, tr)
	}
	if m, tr := parseDotNotation("Create", "orders"); m != "orders" || tr != "Create" {
		t.Fatalf("plain: %q %q", m, tr)
	}
}

func TestAddTransitionCoreAndAppend(t *testing.T) {
	src := addTransitionCore("orders", "ShipCommand", "ship it")
	if !strings.Contains(src, "func (o *ordersTransitions) ShipCommand(") {
		t.Fatalf("bad signature:\n%s", src)
	}

	// addTransition appends to an existing file and rejects duplicates.
	path := filepath.Join(t.TempDir(), "orders.go")
	base := "package transitions\n\nfunc (o *ordersTransitions) Existing() {}\n"
	if err := os.WriteFile(path, []byte(base), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := addTransition(path, "orders", "ShipCommand", "ship it"); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "ShipCommand(") {
		t.Fatalf("method not appended:\n%s", data)
	}
	if err := addTransition(path, "orders", "ShipCommand", ""); err == nil {
		t.Fatal("duplicate method should be rejected")
	}
}

func TestCreateMinimalModuleFileParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orders.go")
	if err := createMinimalModuleFile(path, "orders", "Orders"); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if _, err := parser.ParseFile(token.NewFileSet(), path, data, parser.AllErrors); err != nil {
		t.Fatalf("generated file does not parse: %v\n%s", err, data)
	}
}
