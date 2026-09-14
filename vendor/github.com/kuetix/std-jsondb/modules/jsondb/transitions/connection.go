package transitions

import (
	"fmt"
	"os"
	"strings"
	"sync"

	filejsondb "github.com/anare/filejsondb"
)

// filejsondb.NewDB resolves path relative to the process's working directory
// (its own test suite relies on this too), not as an absolute filesystem path.
// Pass a path relative to wherever the engine process runs from, e.g. "data/app".
var (
	dbMu    sync.Mutex
	dbCache = map[string]*filejsondb.DB{}

	collectionMu    sync.Mutex
	collectionCache = map[string]*filejsondb.Collection{}
)

// resolvePath returns path, falling back to the JSONDB_PATH environment
// variable when path is blank.
func resolvePath(path string) string {
	path = strings.TrimSpace(path)
	if path != "" {
		return path
	}
	return strings.TrimSpace(os.Getenv("JSONDB_PATH"))
}

// resolveDB returns the cached *filejsondb.DB for path, opening it on first use.
func resolveDB(path string) (db *filejsondb.DB, resolvedPath string, err error) {
	resolvedPath = resolvePath(path)
	if resolvedPath == "" {
		return nil, "", fmt.Errorf("db path is required (pass path or set JSONDB_PATH)")
	}

	dbMu.Lock()
	defer dbMu.Unlock()

	if cached, ok := dbCache[resolvedPath]; ok {
		return cached, resolvedPath, nil
	}

	db, err = filejsondb.NewDB(resolvedPath)
	if err != nil {
		return nil, resolvedPath, fmt.Errorf("failed to open store at %q: %w", resolvedPath, err)
	}
	dbCache[resolvedPath] = db
	return db, resolvedPath, nil
}

// resolveCollection returns the cached *filejsondb.Collection for (path, name),
// opening the underlying DB and collection on first use. filejsondb.NewCollection
// panics on failure, so this recovers into a plain error to keep the engine alive.
func resolveCollection(path, name string) (col *filejsondb.Collection, resolvedPath string, err error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, "", fmt.Errorf("collection is required")
	}

	db, resolvedPath, err := resolveDB(path)
	if err != nil {
		return nil, resolvedPath, err
	}

	key := resolvedPath + "\x00" + name

	collectionMu.Lock()
	defer collectionMu.Unlock()

	if cached, ok := collectionCache[key]; ok {
		return cached, resolvedPath, nil
	}

	col, err = func() (col *filejsondb.Collection, err error) {
		defer func() {
			if rec := recover(); rec != nil {
				err = fmt.Errorf("failed to open collection %q at %q: %v", name, resolvedPath, rec)
			}
		}()
		return db.NewCollection(name), nil
	}()
	if err != nil {
		return nil, resolvedPath, err
	}

	collectionCache[key] = col
	return col, resolvedPath, nil
}
