package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	di "github.com/kuetix/container"
	"github.com/kuetix/engine/boot"
	"github.com/kuetix/engine/engine/defines"
	"github.com/kuetix/kue/tests/testutil"
)

// expectedPackages are the Go modules whose transitions must be linked into the
// kue binary — the full std library plus the six added in modules/modules.go.
var expectedPackages = []string{
	"github.com/kuetix/kue",
	"github.com/kuetix/social-oauth",
	"github.com/kuetix/std-ai",
	"github.com/kuetix/std-auth",
	"github.com/kuetix/std-cli",
	"github.com/kuetix/std-core",
	"github.com/kuetix/std-decision",
	"github.com/kuetix/std-http",
	"github.com/kuetix/std-jsondb",
	"github.com/kuetix/std-mysql",
	"github.com/kuetix/std-push",
	"github.com/kuetix/std-redis",
}

func TestExpectedPackagesAreLinked(t *testing.T) {
	seen := map[string]bool{}
	for _, classes := range boot.MetaFunctionCache {
		for _, methods := range classes {
			for _, m := range methods {
				if m.GoModule != "" {
					seen[m.GoModule] = true
				}
			}
		}
	}
	for _, pkg := range expectedPackages {
		if !seen[pkg] {
			t.Errorf("package %q has no transitions in the metadata cache — is it enabled in modules/modules.go?", pkg)
		}
	}
}

// knownUnresolvable documents transitions that carry metadata (so `kue
// transitions` lists them) but cannot actually run, because of the DI
// namespace collision pinned by TestKnownAuthNamespaceCollision. Remove an
// entry here once the collision is fixed.
var knownUnresolvable = map[string]bool{
	"auth/bff":      true,
	"auth/jwt":      true,
	"auth/mfa":      true,
	"auth/password": true,
}

// TestEveryTransitionResolves guards the DI wiring: every namespace/class that
// reports metadata must also be resolvable through the container, otherwise a
// workflow calling it fails at runtime with an unresolved action.
func TestEveryTransitionResolves(t *testing.T) {
	for ns, classes := range boot.MetaFunctionCache {
		for class := range classes {
			if knownUnresolvable[ns+"/"+class] {
				continue
			}
			key := defines.TransitionPrefix + ns + "/" + class
			if !di.CanResolve(key) {
				t.Errorf("%s/%s: metadata present but %q does not resolve in the DI container", ns, class, key)
			}
		}
	}
}

// TestKnownAuthNamespaceCollision pins a pre-existing bug: kue's own CLI
// transition registers di.DependencyInjection["auth"] (transition/auth/auth),
// and std-auth registers the same map key for transition/auth/{bff,jwt,mfa,
// password}. The map key collides, kue's init runs last, and std-auth's auth
// transitions become unresolvable even though `kue transitions` advertises
// them. If this test starts failing, the collision was fixed — delete this
// test and the knownUnresolvable entries above.
func TestKnownAuthNamespaceCollision(t *testing.T) {
	if !di.CanResolve(defines.TransitionPrefix + "auth/auth") {
		t.Fatal("transition/auth/auth (kue CLI login) should resolve")
	}
	for class := range knownUnresolvable {
		key := defines.TransitionPrefix + class
		if di.CanResolve(key) {
			t.Fatalf("%s now resolves — the auth namespace collision looks fixed; "+
				"remove TestKnownAuthNamespaceCollision and the knownUnresolvable map", key)
		}
	}
}

// TestRootModulesJSONNamespacesResolve checks kue's own generated manifest
// against the live container.
func TestRootModulesJSONNamespacesResolve(t *testing.T) {
	root := testutil.ModuleRoot(t)
	data, err := os.ReadFile(filepath.Join(root, "modules", "modules.json"))
	if err != nil {
		t.Fatalf("read modules.json: %v", err)
	}
	var manifest map[string]json.RawMessage
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatalf("parse modules.json: %v", err)
	}
	for nsClass := range manifest {
		key := defines.TransitionPrefix + nsClass
		if !di.CanResolve(key) {
			t.Errorf("modules.json declares %q but it does not resolve", nsClass)
		}
	}
}
