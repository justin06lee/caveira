package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSwap_RenamesOriginalToDead(t *testing.T) {
	parent := t.TempDir()
	orig := filepath.Join(parent, "myrepo")
	stage := filepath.Join(parent, "myrepo.interrogating")
	os.MkdirAll(orig, 0755)
	os.MkdirAll(stage, 0755)
	os.WriteFile(filepath.Join(orig, "marker.txt"), []byte("old"), 0644)
	os.WriteFile(filepath.Join(stage, "marker.txt"), []byte("new"), 0644)

	deadPath, err := Swap(orig, stage)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if filepath.Base(deadPath) != "myrepo.dead" {
		t.Errorf("dead path: got %s, want myrepo.dead", deadPath)
	}
	content, _ := os.ReadFile(filepath.Join(orig, "marker.txt"))
	if string(content) != "new" {
		t.Errorf("after swap, orig content = %q, want %q", content, "new")
	}
}

func TestSwap_AutoVersionsDead(t *testing.T) {
	parent := t.TempDir()
	orig := filepath.Join(parent, "myrepo")
	stage := filepath.Join(parent, "myrepo.interrogating")
	os.MkdirAll(orig, 0755)
	os.MkdirAll(stage, 0755)
	os.MkdirAll(filepath.Join(parent, "myrepo.dead"), 0755)

	deadPath, err := Swap(orig, stage)
	if err != nil {
		t.Fatalf("Swap: %v", err)
	}
	if filepath.Base(deadPath) != "myrepo.dead.1" {
		t.Errorf("got %s, want myrepo.dead.1", deadPath)
	}
}
