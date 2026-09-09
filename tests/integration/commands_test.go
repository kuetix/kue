package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuetix/kue/tests/testutil"
)

func responseString(t *testing.T, r testutil.WorkflowResult) string {
	t.Helper()
	if r.Err != nil {
		t.Fatalf("workflow error: %v", r.Err)
	}
	s, ok := r.Response.(string)
	if !ok {
		t.Fatalf("expected string response, got %T: %#v", r.Response, r.Response)
	}
	return s
}

func TestModulesCommand(t *testing.T) {
	r := testutil.RunCLICommand(t, "@cli/run/modules", testutil.CmdContext("modules", nil))
	if r.StatusCode != 200 {
		t.Fatalf("status %d", r.StatusCode)
	}
	out := responseString(t, r)
	for _, want := range []string{
		"github.com/kuetix/std-redis",
		"github.com/kuetix/std-decision",
		"github.com/kuetix/std-jsondb",
		"github.com/kuetix/std-mysql",
		"github.com/kuetix/std-push",
		"github.com/kuetix/social-oauth",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("modules output missing %q\n%s", want, out)
		}
	}
}

func TestModulesCommandJSON(t *testing.T) {
	flags := map[string]interface{}{"json": testutil.BoolFlag(true)}
	r := testutil.RunCLICommand(t, "@cli/run/modules", testutil.CmdContext("modules", flags))
	out := responseString(t, r)

	var parsed struct {
		Count   int `json:"count"`
		Modules []struct {
			Module      string   `json:"module"`
			Namespaces  []string `json:"namespaces"`
			Transitions int      `json:"transitions"`
		} `json:"modules"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if parsed.Count < 10 {
		t.Fatalf("expected >=10 linked packages, got %d", parsed.Count)
	}
	found := false
	for _, m := range parsed.Modules {
		if m.Module == "github.com/kuetix/std-redis" && m.Transitions > 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("std-redis not present with transitions in JSON output:\n%s", out)
	}
}

func TestTransitionsCommandModuleFilter(t *testing.T) {
	flags := map[string]interface{}{"module": testutil.StringFlag("std-decision")}
	r := testutil.RunCLICommand(t, "@cli/run/transitions", testutil.CmdContext("transitions", flags))
	out := responseString(t, r)
	if !strings.Contains(out, "decision/cond.Number") {
		t.Fatalf("expected decision/cond.Number in filtered output:\n%s", out)
	}
	if strings.Contains(out, "redis/string.LPush") {
		t.Fatalf("filter leaked non-decision transitions:\n%s", out)
	}
}
