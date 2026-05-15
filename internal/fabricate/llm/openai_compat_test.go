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

		var req struct {
			Model          string  `json:"model"`
			Temperature    float64 `json:"temperature"`
			ResponseFormat struct {
				Type string `json:"type"`
			} `json:"response_format"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decoding request body: %v", err)
		}
		if req.Model != "m" {
			t.Errorf("model = %q, want %q", req.Model, "m")
		}
		if req.Temperature != 0.2 {
			t.Errorf("temperature = %v, want 0.2", req.Temperature)
		}
		if req.ResponseFormat.Type != "json_object" {
			t.Errorf("response_format.type = %q, want %q", req.ResponseFormat.Type, "json_object")
		}
		if len(req.Messages) == 0 {
			t.Fatalf("request had no messages")
		}
		if req.Messages[0].Content != "prompt" {
			t.Errorf("messages[0].content = %q, want %q", req.Messages[0].Content, "prompt")
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

func TestOpenAICompat_ForwardsSeed(t *testing.T) {
	t.Run("HasSeed true forwards seed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			seed, ok := raw["seed"]
			if !ok {
				t.Fatalf("expected seed key in request body, got %v", raw)
			}
			if got, ok := seed.(float64); !ok || got != 99 {
				t.Errorf("seed = %v, want 99", seed)
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
			name: "groq", baseURL: srv.URL, apiKey: "k",
			model: "m", client: srv.Client(),
			opts: Options{Seed: 99, HasSeed: true},
		}
		if _, err := p.GeneratePlan(context.Background(), "prompt"); err != nil {
			t.Fatalf("GeneratePlan: %v", err)
		}
	})

	t.Run("HasSeed false omits seed", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var raw map[string]any
			if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
				t.Fatalf("decoding request body: %v", err)
			}
			if _, ok := raw["seed"]; ok {
				t.Errorf("expected no seed key in request body, got %v", raw)
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
			name: "groq", baseURL: srv.URL, apiKey: "k",
			model: "m", client: srv.Client(),
			opts: Options{HasSeed: false},
		}
		if _, err := p.GeneratePlan(context.Background(), "prompt"); err != nil {
			t.Fatalf("GeneratePlan: %v", err)
		}
	})
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
