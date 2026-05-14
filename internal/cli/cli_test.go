package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("expected exit 0 on --help, got %d; stderr=%q", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("caveira")) {
		t.Fatalf("expected help output to mention 'caveira', got: %s", out.String())
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
	if code != 0 {
		t.Fatalf("expected exit 0, got %d; stderr=%q", code, errOut.String())
	}
	if !strings.Contains(out.String(), "parsed config:") {
		t.Fatalf("expected parsed config message, got: %s", out.String())
	}
}
