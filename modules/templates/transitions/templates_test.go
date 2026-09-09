package transitions

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boolThunk(v bool) func() *bool { return func() *bool { return &v } }

func TestPrintCacheVersion(t *testing.T) {
	var sb strings.Builder
	dir := t.TempDir()
	printCacheVersion(&sb, dir, "web", "v1")
	if !strings.Contains(sb.String(), "v1 (web): cached, last modified") {
		t.Fatalf("existing: %q", sb.String())
	}

	sb.Reset()
	printCacheVersion(&sb, filepath.Join(dir, "missing"), "git", "v2")
	if !strings.Contains(sb.String(), "v2 (git): error reading cache") {
		t.Fatalf("missing: %q", sb.String())
	}
}

func TestTemplatesStatusHelp(t *testing.T) {
	tr := &templatesTransitions{}
	fs := flag.NewFlagSet("templates", flag.ContinueOnError)
	r := tr.TemplatesStatusCommand("", map[string]interface{}{
		"usage":   "USAGE: kue templates status",
		"flagSet": fs,
	}, map[string]interface{}{"help": boolThunk(true)})
	if r.Error != nil {
		t.Fatalf("err: %v", r.Error)
	}
	if out, _ := r.Response.(string); !strings.Contains(out, "USAGE: kue templates status") {
		t.Fatalf("help output: %v", r.Response)
	}
}

func TestTemplatesStatusNoCache(t *testing.T) {
	// Point every source flag at a fresh temp dir; with no cache subdirs the
	// command reports "No cached templates found" without error.
	empty := t.TempDir()
	_ = os.Chmod(empty, 0o755)

	tr := &templatesTransitions{}
	fs := flag.NewFlagSet("templates", flag.ContinueOnError)
	r := tr.TemplatesStatusCommand("", map[string]interface{}{"usage": "", "flagSet": fs}, map[string]interface{}{
		"help":             boolThunk(false),
		"template-url":     strThunk(""),
		"template-path":    strThunk(filepath.Join(empty, "no-such-templates")),
		"template-git":     strThunk(""),
		"template-version": strThunk(""),
	})
	if r.Error != nil {
		t.Fatalf("err: %v", r.Error)
	}
	if out, _ := r.Response.(string); !strings.Contains(out, "Template Cache Status") {
		t.Fatalf("status output: %v", r.Response)
	}
}

func strThunk(v string) func() *string { return func() *string { return &v } }
