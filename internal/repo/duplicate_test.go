package repo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDuplicate_CopiesFilesRecursively(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "a.txt"), []byte("hello"), 0644)
	os.Mkdir(filepath.Join(src, "sub"), 0755)
	os.WriteFile(filepath.Join(src, "sub", "b.txt"), []byte("world"), 0644)

	dst := filepath.Join(t.TempDir(), "dst")
	if err := Duplicate(src, dst); err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	check := func(p, want string) {
		got, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	check(filepath.Join(dst, "a.txt"), "hello")
	check(filepath.Join(dst, "sub", "b.txt"), "world")
}

func TestDuplicate_RefusesIfDestExists(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	if err := Duplicate(src, dst); err == nil {
		t.Fatal("expected error when dst exists")
	}
}
