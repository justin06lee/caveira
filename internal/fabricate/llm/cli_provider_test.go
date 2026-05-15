package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeFakeBinary creates an executable shell script that echoes a fixed
// response, and prepends its directory to PATH for the test.
func writeFakeBinary(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCLIProvider_GeneratePlan(t *testing.T) {
	writeFakeBinary(t, "claude", `echo '{"commits":[{"message":"chore: x","type":"chore","changes":[]}]}'`)
	p := newCLIProvider("claude-code", "claude", []string{"-p"}, Options{})
	got, err := p.GeneratePlan(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty output from fake binary")
	}
}

func TestCLIProvider_MissingBinary(t *testing.T) {
	p := newCLIProvider("codex", "caveira-no-such-binary-xyz", []string{"exec"}, Options{})
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error when the CLI binary is absent from PATH")
	}
}

func TestCLIProvider_NonZeroExit(t *testing.T) {
	writeFakeBinary(t, "opencode", `echo boom >&2; exit 1`)
	p := newCLIProvider("opencode", "opencode", []string{"run"}, Options{})
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error on non-zero subprocess exit")
	}
}
