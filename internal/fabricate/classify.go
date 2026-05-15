package fabricate

import (
	"path"
	"strings"
)

// FileKind is one of Chore, Code, Test.
type FileKind int

const (
	Code FileKind = iota
	Test
	Chore
)

var chorePrefixes = []string{
	"readme",
}

var choreExactBasenames = map[string]bool{
	"makefile":          true,
	"license":           true,
	"license.md":        true,
	"license.txt":       true,
	"go.mod":            true,
	"go.sum":            true,
	"package.json":      true,
	"package-lock.json": true,
	"yarn.lock":         true,
	"pnpm-lock.yaml":    true,
	"cargo.toml":        true,
	"cargo.lock":        true,
	"pyproject.toml":    true,
	"setup.py":          true,
	"setup.cfg":         true,
	"pipfile":           true,
	"pipfile.lock":      true,
	"dockerfile":        true,
	".dockerignore":     true,
	".gitignore":        true,
	".gitattributes":    true,
	".editorconfig":     true,
}

var testPathSubstrings = []string{
	"/test/",
	"/tests/",
	"/__tests__/",
	"/spec/",
}

// Classify returns the FileKind of the given path. Top-level files (no dir)
// with known names are Chore; files matching test patterns are Test;
// everything else is Code.
func Classify(p string) FileKind {
	clean := path.Clean(p)
	base := strings.ToLower(path.Base(clean))
	dir := path.Dir(clean)

	if dir == "." || dir == "/" {
		if isChoreBasename(base) {
			return Chore
		}
		if strings.HasPrefix(base, ".") {
			return Chore
		}
		if strings.HasSuffix(base, ".md") {
			return Chore
		}
		if strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
			return Chore
		}
	}

	if isTestPath(clean) {
		return Test
	}
	return Code
}

func isChoreBasename(base string) bool {
	if choreExactBasenames[base] {
		return true
	}
	for _, p := range chorePrefixes {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	return false
}

func isTestPath(p string) bool {
	lower := strings.ToLower(p)
	for _, s := range testPathSubstrings {
		if strings.Contains("/"+lower, s) {
			return true
		}
	}
	base := strings.ToLower(path.Base(p))
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(stem, "_test"):
		return true
	case strings.HasPrefix(stem, "test_"):
		return true
	case strings.HasSuffix(stem, ".test"):
		return true
	case strings.HasSuffix(stem, ".spec"):
		return true
	}
	return false
}
