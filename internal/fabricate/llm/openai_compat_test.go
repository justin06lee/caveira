package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompat_GeneratePlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong Authorization header: %q", r.Header.Get("Authorization"))
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"commits":[]}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &openAICompatProvider{
		name: "groq", baseURL: srv.URL, apiKey: "test-key",
		model: "m", client: srv.Client(), opts: Options{},
	}
	got, err := p.GeneratePlan(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if !strings.Contains(got, "commits") {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestOpenAICompat_MissingKey(t *testing.T) {
	p := newOpenAICompat("groq", "http://unused", "CAVEIRA_TEST_NO_SUCH_KEY", "m", Options{})
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error when API key env var is unset")
	}
}

func TestOpenAICompat_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := &openAICompatProvider{
		name: "groq", baseURL: srv.URL, apiKey: "k", model: "m",
		client: srv.Client(), opts: Options{},
	}
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}
