package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFindsEnvFileUpTheTree(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range append(append(append(keyVars, baseURLVars...), modelVars...), effortVars...) {
		t.Setenv(v, "")
	}
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".env.local"), []byte("# comment\nexport ABLITERATION_API_KEY=\"ak_test\"\nCAVEIRA_MODEL=abliterated-model-large-v2 # trailing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, ".env"), []byte("CAVEIRA_REASONING_EFFORT=high\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	if s.APIKey != "ak_test" || s.KeySource != ".env.local" {
		t.Fatalf("key %q from %q", s.APIKey, s.KeySource)
	}
	if s.Model != "abliterated-model-large-v2" || s.ReasoningEffort != "high" {
		t.Fatalf("model %q effort %q", s.Model, s.ReasoningEffort)
	}
	if s.BaseURL != DefaultBaseURL {
		t.Fatalf("base url %q", s.BaseURL)
	}

	// Environment beats files; config.json sits under them.
	if err := Save(Settings{APIKey: "from-config", Model: "cfg-model", BaseURL: "http://localhost:1234/v1/"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAVEIRA_MODEL", "env-model")
	s, err = Load(sub)
	if err != nil {
		t.Fatal(err)
	}
	if s.APIKey != "ak_test" || s.Model != "env-model" || s.BaseURL != "http://localhost:1234/v1" {
		t.Fatalf("precedence: %+v", s)
	}
}

func TestSpecAndEffort(t *testing.T) {
	if Spec("abliterated-model").ContextWindow != 262_144 || Spec("nope").ContextWindow != 128_000 {
		t.Fatal("spec")
	}
	if !ValidEffort("max") || ValidEffort("turbo") {
		t.Fatal("effort")
	}
}
