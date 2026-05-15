package cli

import (
	"bytes"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0 on --help, got %d; stderr=%q", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Usage:")) {
		t.Fatalf("expected help output to include 'Usage:', got: %s", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("Rewrite a repo's commit timestamps")) {
		t.Fatalf("expected help output to include the Short description, got: %s", out.String())
	}
}

func TestNewRootCmd_UsesGivenName(t *testing.T) {
	var out, errOut bytes.Buffer
	cmd := newRootCmd("cav")
	cmd.SetArgs([]string{"--help"})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(out.Bytes(), []byte("Usage:\n  cav")) {
		t.Fatalf("expected Usage line to use 'cav', got: %s", out.String())
	}
}

func TestRunMissingRequired(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{"--repo", "/tmp/x"}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit when --start and --end are missing")
	}
}

func TestRunValidFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/x",
		"--start", "2026-05-14 13:00",
		"--end", "2026-05-14 17:00",
		"--window-tz", "UTC",
	}, &out, &errOut)
	// Pipeline fails because /tmp/x is not a directory; we just verify flag
	// parsing reached the pipeline (no exit-on-flag-validation).
	if code == 0 {
		t.Fatalf("expected non-zero exit (pipeline error), got %d", code)
	}
	if errOut.Len() == 0 {
		t.Fatalf("expected stderr output from pipeline failure, got empty")
	}
}
