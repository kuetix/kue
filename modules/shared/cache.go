package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/kuetix/kue"
)

// CachedWorkflow describes a workflow that `kue install` / `kue get` has
// fetched and verified into ~/.kue/cache/workflows.
type CachedWorkflow struct {
	// Path is the absolute path to the cached .wsl / .swsl source file.
	Path string
	// Name is the registry name (e.g. "ai/agent").
	Name string
	// Owner is the registry owner id the copy was fetched under.
	Owner string
	// Version is the registry version integer this copy pins.
	Version int
	// Meta holds the .meta.json sidecar (hash, dependencies, actions,
	// fetched_at, ...) when one is present.
	Meta map[string]interface{}
}

// WorkflowCacheRoot is ~/.kue/cache/workflows.
func WorkflowCacheRoot() string {
	return filepath.Join(kue.HomeDir, kue.CacheDir, "cache", "workflows")
}

var cacheVersionRe = regexp.MustCompile(`@v(\d+)\.(?:wsl|swsl)$`)

// FindCachedWorkflow looks for a workflow by registry name in the local
// ~/.kue cache, with no network call. When the same name is cached under
// several owners or versions the highest version wins (ties: newest mtime).
// ok is false when nothing is cached.
func FindCachedWorkflow(name string) (CachedWorkflow, bool) {
	name = strings.TrimSpace(strings.TrimSuffix(strings.TrimSuffix(name, ".wsl"), ".swsl"))
	if name == "" {
		return CachedWorkflow{}, false
	}
	root := WorkflowCacheRoot()
	// <root>/<owner>/<name>@v*.{wsl,swsl}
	pattern := filepath.Join(root, "*", filepath.FromSlash(name)+"@v*")
	matches, _ := filepath.Glob(pattern)

	type cand struct {
		path    string
		owner   string
		version int
		mtime   int64
	}
	var cands []cand
	for _, m := range matches {
		if strings.HasSuffix(m, ".meta.json") {
			continue
		}
		sub := cacheVersionRe.FindStringSubmatch(m)
		if sub == nil {
			continue
		}
		v, _ := strconv.Atoi(sub[1])
		fi, err := os.Stat(m)
		if err != nil {
			continue
		}
		rel, rerr := filepath.Rel(root, m)
		owner := ""
		if rerr == nil {
			if i := strings.IndexRune(rel, filepath.Separator); i > 0 {
				owner = rel[:i]
			}
		}
		cands = append(cands, cand{path: m, owner: owner, version: v, mtime: fi.ModTime().UnixNano()})
	}
	if len(cands) == 0 {
		return CachedWorkflow{}, false
	}
	sort.Slice(cands, func(i, j int) bool {
		if cands[i].version != cands[j].version {
			return cands[i].version > cands[j].version
		}
		return cands[i].mtime > cands[j].mtime
	})
	best := cands[0]

	cw := CachedWorkflow{Path: best.path, Name: name, Owner: best.owner, Version: best.version}
	if raw, err := os.ReadFile(best.path + ".meta.json"); err == nil {
		var meta map[string]interface{}
		if json.Unmarshal(raw, &meta) == nil {
			cw.Meta = meta
		}
	}
	return cw, true
}
