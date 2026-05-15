package fabricate

import "testing"

func TestClassify(t *testing.T) {
	cases := map[string]FileKind{
		// Chore
		"README.md":      Chore,
		"Makefile":       Chore,
		"go.mod":         Chore,
		"go.sum":         Chore,
		"LICENSE":        Chore,
		".gitignore":     Chore,
		".editorconfig":  Chore,
		"package.json":   Chore,
		"Cargo.toml":     Chore,
		"Dockerfile":     Chore,
		"pyproject.toml": Chore,
		// Tests
		"internal/walk/load_test.go": Test,
		"src/test_module.py":         Test,
		"src/module.test.ts":         Test,
		"src/module.spec.ts":         Test,
		"tests/foo.py":               Test,
		"src/__tests__/foo.js":       Test,
		"src/spec/foo.rb":            Test,
		// Code
		"internal/walk/load.go":     Code,
		"cmd/caveira/main.go":       Code,
		"src/module.ts":             Code,
		"src/components/Button.tsx": Code,
	}
	for path, want := range cases {
		got := Classify(path)
		if got != want {
			t.Errorf("Classify(%q) = %v, want %v", path, got, want)
		}
	}
}
