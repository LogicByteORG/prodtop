package favorites

import (
	"path/filepath"
	"testing"
)

func TestAddAndReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.yaml")
	first, err := AddTo(path, "SELECT 1")
	if err != nil {
		t.Fatal(err)
	}
	if first.Name != "q1" {
		t.Errorf("first name = %q, want q1", first.Name)
	}
	if _, err := AddTo(path, "SELECT 2"); err != nil {
		t.Fatal(err)
	}
	entries, err := LoadPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Query != "SELECT 1" || entries[1].Name != "q2" {
		t.Errorf("unexpected entries: %+v", entries)
	}
}

func TestAddRejectsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "favorites.yaml")
	if _, err := AddTo(path, ""); err == nil {
		t.Error("AddTo(empty) = nil, want error")
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	entries, err := LoadPath(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil || len(entries) != 0 {
		t.Errorf("LoadPath(missing) = %+v, %v; want empty, nil", entries, err)
	}
}
