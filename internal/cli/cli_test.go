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
	if !bytes.Contains(out.Bytes(), []byte("caveira")) {
		t.Fatalf("expected help output to mention 'caveira', got: %s", out.String())
	}
}
