package repo

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Duplicate recursively copies src to dst. Fails if dst already exists.
func Duplicate(src, dst string) error {
	if _, err := os.Stat(dst); err == nil {
		return fmt.Errorf("destination %s already exists", dst)
	}
	// Directories whose source mode lacks owner-write are created writable first
	// (so their contents can be written) and have their real mode restored after
	// the walk, deepest-first. Without this, a read-only source directory (e.g.
	// mode 0555) would block writing the files inside it.
	type dirRestore struct {
		path string
		mode os.FileMode
	}
	var restore []dirRestore
	err := filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			if err := os.MkdirAll(target, info.Mode()|0o700); err != nil {
				return err
			}
			if info.Mode()&0o700 != 0o700 {
				restore = append(restore, dirRestore{target, info.Mode()})
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		in, err := os.Open(p)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer out.Close()
		_, err = io.Copy(out, in)
		return err
	})
	if err != nil {
		return err
	}
	// Restore tightened directory modes deepest-first, so re-applying a parent's
	// restrictive mode never blocks a child's chmod.
	for i := len(restore) - 1; i >= 0; i-- {
		if err := os.Chmod(restore[i].path, restore[i].mode); err != nil {
			return err
		}
	}
	return nil
}
