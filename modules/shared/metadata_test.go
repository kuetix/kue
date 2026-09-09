package shared

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateApplicationMetadata(t *testing.T) {
	dir := t.TempDir()
	if err := CreateApplicationMetadata(dir, "demo", "cli"); err != nil {
		t.Fatalf("create: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "application.json"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var meta ApplicationMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if meta.Name != "demo" || meta.Type != "cli" || meta.Version != "0.1.0" {
		t.Fatalf("unexpected metadata: %#v", meta)
	}
	if meta.CreatedAt.IsZero() {
		t.Error("CreatedAt not set")
	}
}

func TestReadPackageInfo(t *testing.T) {
	dir := t.TempDir()
	want := PackageInfo{Name: "acme/util", Type: "package", Version: "1.2.3", Keywords: []string{"a", "b"}}
	data, _ := json.Marshal(want)
	if err := os.WriteFile(filepath.Join(dir, "kuetix.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := ReadPackageInfo(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if got.Name != want.Name || got.Version != want.Version || len(got.Keywords) != 2 {
		t.Fatalf("mismatch: %#v", got)
	}

	if _, err := ReadPackageInfo(t.TempDir()); err == nil {
		t.Fatal("expected error when kuetix.json is missing")
	}
}

func TestResolvePackageJSONPath(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "kuetix.json")
	if err := os.WriteFile(jsonPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Directory -> appends kuetix.json.
	if got, err := ResolvePackageJSONPath(dir); err != nil || got != jsonPath {
		t.Fatalf("dir: got %q err %v", got, err)
	}
	// Explicit file.
	if got, err := ResolvePackageJSONPath(jsonPath); err != nil || got != jsonPath {
		t.Fatalf("file: got %q err %v", got, err)
	}
	// Wrong filename.
	other := filepath.Join(dir, "other.json")
	_ = os.WriteFile(other, []byte("{}"), 0o600)
	if _, err := ResolvePackageJSONPath(other); err == nil {
		t.Fatal("expected error for non-kuetix.json file")
	}
	// Missing path.
	if _, err := ResolvePackageJSONPath(filepath.Join(dir, "nope")); err == nil {
		t.Fatal("expected error for missing path")
	}
}
