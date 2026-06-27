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

func TestDuplicate_PreservesReadOnlyDirectory(t *testing.T) {
	src := t.TempDir()
	roDir := filepath.Join(src, "ro")
	if err := os.Mkdir(roDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(roDir, "f.txt"), []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the directory read-only (no owner-write) AFTER writing its contents.
	if err := os.Chmod(roDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(roDir, 0o755) }) // so TempDir cleanup can remove it

	dst := filepath.Join(t.TempDir(), "dst")
	if err := Duplicate(src, dst); err != nil {
		t.Fatalf("Duplicate with read-only dir: %v", err)
	}
	t.Cleanup(func() { os.Chmod(filepath.Join(dst, "ro"), 0o755) })

	got, err := os.ReadFile(filepath.Join(dst, "ro", "f.txt"))
	if err != nil {
		t.Fatalf("read copied file inside read-only dir: %v", err)
	}
	if string(got) != "data" {
		t.Errorf("content = %q, want %q", got, "data")
	}
	// The restored directory mode must match the source (read-only).
	fi, err := os.Stat(filepath.Join(dst, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm() != 0o555 {
		t.Errorf("restored dir mode = %o, want 0555", fi.Mode().Perm())
	}
}
