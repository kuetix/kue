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
	if sub, name := parseWorkflowArg("  bare  "); sub != "cli" || name != "bare" {
		t.Fatalf("bare -> cli/bare, got %q %q", sub, name)
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

func TestAddTransitionCore(t *testing.T) {
	src := addTransitionCore("orders", "ShipCommand", "ship an order")
	if !strings.Contains(src, "// ShipCommand — ship an order") {
		t.Fatalf("missing desc comment:\n%s", src)
	}
	if !strings.Contains(src, "func (o *ordersTransitions) ShipCommand(command string, config map[string]interface{}, flags map[string]interface{})") {
		t.Fatalf("bad signature:\n%s", src)
	}

	// No description -> no comment line.
	if strings.Contains(addTransitionCore("orders", "ShipCommand", "  "), "—") {
		t.Fatal("blank description should not produce a comment")
	}
}

func TestCreateMinimalModuleFileParses(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "orders.go")
	if err := createMinimalModuleFile(path, "orders", "Orders"); err != nil {
		t.Fatalf("create: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := parser.ParseFile(token.NewFileSet(), path, data, parser.AllErrors); err != nil {
		t.Fatalf("generated file does not parse: %v\n%s", err, data)
	}
	src := string(data)
	if !strings.Contains(src, "type ordersTransitions struct") || !strings.Contains(src, "func NewOrdersTransition()") {
		t.Fatalf("unexpected content:\n%s", src)
	}
}
