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
	if !bytes.Contains(out.Bytes(), []byte("commit timestamps to fit a chosen time window")) {
		t.Fatalf("expected help output to include the short description, got: %s", out.String())
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

func TestRunFabricateFlagsParse(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate",
		"--pigs", "2",
		"--pig", "Alice <a@x.com>",
		"--pig", "Bob <b@x.com>",
	}, &out, &errOut)
	// Validation should pass (the pipeline failure later is fine).
	if !bytes.Contains(errOut.Bytes(), []byte("not a directory")) &&
		!bytes.Contains(errOut.Bytes(), []byte("no such file")) {
		// Either the pipeline failed cleanly, or an unexpected error occurred.
		if code == 0 {
			t.Fatalf("expected non-zero exit due to missing repo path, got 0; stderr=%q", errOut.String())
		}
	}
}

func TestRunPickFlagParses(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent-pick",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--rats", "3", "--pick",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit (missing repo), got 0; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("--pick requires")) {
		t.Fatalf("--pick with --fabricate --rats should pass validation; stderr=%q", errOut.String())
	}
}

func TestInterrogate_RequiresRepo(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{"interrogate"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit when --repo is missing; stderr=%q", errOut.String())
	}
}

func TestInterrogate_BadRepo(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{"interrogate", "--repo", "/tmp/caveira-no-such-repo-xyz"}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit for a nonexistent repo; stderr=%q", errOut.String())
	}
}

func TestInterrogate_RegisteredAsSubcommand(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{"--help"}, &out, &errOut)
	if code != 0 {
		t.Fatalf("--help exited %d; stderr=%q", code, errOut.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("interrogate")) {
		t.Fatalf("root --help should list the interrogate subcommand:\n%s", out.String())
	}
}

func TestInterrogate_RejectsRewriteFlags(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"interrogate", "--repo", "/tmp/x", "--start", "2026-05-16 12:00",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit: --start is not an interrogate flag; stderr=%q", errOut.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("unknown flag")) {
		t.Fatalf("expected an 'unknown flag' error for --start on interrogate; stderr=%q", errOut.String())
	}
}

func TestRunEarnedFlagParses(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent-earned",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--pigs", "2", "--earned",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit (missing repo), got 0; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("--earned requires")) {
		t.Fatalf("--earned with --fabricate --pigs should pass validation; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("unknown flag")) {
		t.Fatalf("--earned flag is not registered; stderr=%q", errOut.String())
	}
}
