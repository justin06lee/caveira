package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justin06lee/caveira/tui/internal/config"
)

// fakeOllama answers the parts of Ollama's API dev mode reads, with the
// shapes Ollama 0.34 sends: two models installed, llama3.2 loaded with the
// default window, and a small model that has no tool calling.
func fakeOllama(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"object":"list","data":[{"id":"llama3.2:latest"},{"id":"qwen3.5-0.8b-heretic:latest"},{"id":"qwen3:4b"}]}`))
		case "/api/ps":
			w.Write([]byte(`{"models":[{"name":"llama3.2:latest","context_length":4096}]}`))
		case "/api/show":
			var req struct{ Model string }
			json.NewDecoder(r.Body).Decode(&req)
			switch req.Model {
			case "llama3.2:latest":
				w.Write([]byte(`{"capabilities":["completion","tools"],"details":{"parameter_size":"3.2B"}}`))
			case "qwen3:4b":
				w.Write([]byte(`{"capabilities":["completion","tools","thinking"],"details":{"parameter_size":"4.0B"}}`))
			default:
				w.Write([]byte(`{"capabilities":["completion"],"details":{"parameter_size":"772.85M"}}`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDevReadsOllamasWindow(t *testing.T) {
	t.Setenv("OLLAMA_CONTEXT_LENGTH", "")
	t.Setenv("CAVEIRA_DEV_MODEL", "")
	t.Setenv("CAVEIRA_DEV_BASE_URL", "")
	srv := fakeOllama(t)
	base := srv.URL + "/v1"

	cfg := config.Settings{ContextWindow: 999_999}
	if err := applyDev(&cfg, "llama3.2:latest", base); err != nil {
		t.Fatal(err)
	}
	if cfg.ContextWindow != 4096 {
		t.Fatalf("loaded model: window %d, want the 4096 Ollama reports", cfg.ContextWindow)
	}

	ctx := context.Background()
	if n := ollamaContext(ctx, base, "qwen3:4b"); n != 4096 {
		t.Fatalf("model not loaded: window %d, want Ollama's default 4096", n)
	}
	t.Setenv("OLLAMA_CONTEXT_LENGTH", "32768")
	if n := ollamaContext(ctx, base, "qwen3:4b"); n != 32768 {
		t.Fatalf("with OLLAMA_CONTEXT_LENGTH: window %d, want 32768", n)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	if n := ollamaContext(ctx, other.URL+"/v1", "x"); n != 0 {
		t.Fatalf("a server that is not Ollama: window %d, want 0 (unknown)", n)
	}
}

func TestDevModelListSaysWhatEachCanDo(t *testing.T) {
	srv := fakeOllama(t)
	choices, err := devModelList(srv.URL + "/v1")(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 3 {
		t.Fatalf("got %d choices, want 3", len(choices))
	}
	by := map[string]int{}
	for i, c := range choices {
		by[c.ID] = i
	}
	llama := choices[by["llama3.2:latest"]]
	if llama.Unusable != "" || !llama.NoEffort || llama.Note != "3.2B · local" || llama.Context != 4096 {
		t.Errorf("llama3.2: %+v", llama)
	}
	if h := choices[by["qwen3.5-0.8b-heretic:latest"]]; h.Unusable == "" {
		t.Errorf("a model without tool calling should be unusable: %+v", h)
	}
	if q := choices[by["qwen3:4b"]]; q.Unusable != "" || q.NoEffort {
		t.Errorf("qwen3 calls tools and thinks: %+v", q)
	}
}
