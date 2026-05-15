package llm

import "testing"

func TestNewProvider_KnownNames(t *testing.T) {
	for _, name := range []string{"groq", "nvidia", "claude-code", "codex", "opencode"} {
		p, err := NewProvider(name, Options{})
		if err != nil {
			t.Fatalf("NewProvider(%q): %v", name, err)
		}
		if p.Name() != name {
			t.Fatalf("provider Name() = %q, want %q", p.Name(), name)
		}
	}
}

func TestNewProvider_UnknownRejected(t *testing.T) {
	if _, err := NewProvider("bogus", Options{}); err == nil {
		t.Fatal("expected error for unknown provider name")
	}
}
