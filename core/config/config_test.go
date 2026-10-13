package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/justin06lee/caveira/core/cloud"
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

func TestSignedInAccountCarriesTheRequests(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, v := range append(keyVars, baseURLVars...) {
		t.Setenv(v, "")
	}
	t.Setenv("ABLITERATION_API_KEY", "ak_own")
	dir := t.TempDir()

	s, _ := Load(dir)
	if s.OnAccount() || s.APIKey != "ak_own" {
		t.Fatalf("signed out: %+v", s)
	}

	if err := cloud.Save(cloud.Auth{URL: "https://site.test", Token: "tok", Email: "a@b.c"}); err != nil {
		t.Fatal(err)
	}
	s, _ = Load(dir)
	if !s.OnAccount() || s.APIKey != "tok" || s.BaseURL != "https://site.test/api/v1" {
		t.Fatalf("signed in: %+v", s)
	}

	// Without the account, it is back on its own key.
	if own := s.WithoutAccount(); own.OnAccount() || own.APIKey != "ak_own" || own.BaseURL != DefaultBaseURL {
		t.Fatalf("without the account: %+v", own)
	}

	// A server chosen on purpose is not taken over by the account.
	t.Setenv("CAVEIRA_BASE_URL", "http://localhost:11434/v1")
	s, _ = Load(dir)
	if s.OnAccount() || s.BaseURL != "http://localhost:11434/v1" {
		t.Fatalf("own server: %+v", s)
	}
}
