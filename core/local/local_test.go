package local

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/justin06lee/caveira/core/config"
)

// fakeOllama answers the parts of Ollama's API this package uses, with the
// shapes Ollama 0.34 sends: llama3.2 (Llama 3's tool template, loaded at
// the default window), qwen3 (its own template), a model that has no tool
// calling, and a caveira/ copy left from an earlier run. It records what
// /api/create was asked to make; noCreate makes it an Ollama too old to
// create models from JSON.
type fakeOllama struct {
	*httptest.Server
	noCreate bool
	mu       sync.Mutex
	created  map[string]map[string]any
}

func newFakeOllama(t *testing.T) *fakeOllama {
	t.Helper()
	f := &fakeOllama{created: map[string]map[string]any{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"object":"list","data":[{"id":"llama3.2:latest"},{"id":"qwen3.5-0.8b-heretic:latest"},{"id":"qwen3:4b"},{"id":"caveira/llama3.2:latest"}]}`))
		case "/api/ps":
			w.Write([]byte(`{"models":[{"name":"llama3.2:latest","context_length":4096}]}`))
		case "/api/show":
			var req struct{ Model string }
			json.NewDecoder(r.Body).Decode(&req)
			switch req.Model {
			case "llama3.2:latest":
				w.Write([]byte(`{"capabilities":["completion","tools"],"details":{"parameter_size":"3.2B"},"template":"{{ if $.Tools }}Given the following functions, please respond with a JSON for a function call with its proper arguments{{ end }}"}`))
			case "qwen3:4b":
				w.Write([]byte(`{"capabilities":["completion","tools","thinking"],"details":{"parameter_size":"4.0B"},"template":"<|im_start|>system {{ .System }}"}`))
			default:
				w.Write([]byte(`{"capabilities":["completion"],"details":{"parameter_size":"772.85M"}}`))
			}
		case "/api/create":
			if f.noCreate {
				http.NotFound(w, r)
				return
			}
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.created[body["model"].(string)] = body
			f.mu.Unlock()
			w.Write([]byte(`{"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func clearEnv(t *testing.T) {
	t.Setenv("OLLAMA_CONTEXT_LENGTH", "")
	t.Setenv("CAVEIRA_DEV_MODEL", "")
	t.Setenv("CAVEIRA_DEV_BASE_URL", "")
}

// Apply runs a Llama 3 model as a caveira/ copy with room for the
// prompt and a template that does not demand a tool call every message.
func TestRunsOllamaModelsAsCaveiraCopies(t *testing.T) {
	clearEnv(t)
	f := newFakeOllama(t)
	cfg := config.Settings{ContextWindow: 999_999}
	if err := Apply(&cfg, "llama3.2:latest", f.URL+"/v1"); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "caveira/llama3.2:latest" || cfg.ContextWindow != Window {
		t.Fatalf("running %s with %d context, want the caveira/ copy with %d", cfg.Model, cfg.ContextWindow, Window)
	}
	body := f.created["caveira/llama3.2:latest"]
	if body["from"] != "llama3.2:latest" || body["template"] != llama3ToolTemplate {
		t.Fatalf("copy not made from llama3.2 with caveira's template: %v", body)
	}
	params := body["parameters"].(map[string]any)
	if params["num_ctx"] != float64(Window) || params["temperature"] != temperature {
		t.Fatalf("copy parameters %v", params)
	}

	// A copy asked for by name is refreshed from its base, not copied again.
	cfg = config.Settings{}
	if err := Apply(&cfg, "caveira/llama3.2:latest", f.URL+"/v1"); err != nil || cfg.Model != "caveira/llama3.2:latest" {
		t.Fatalf("model %s, err %v", cfg.Model, err)
	}
	if _, ok := f.created["caveira/caveira/llama3.2:latest"]; ok {
		t.Fatal("made a copy of a copy")
	}

	// Other families keep their own template; a bigger server window wins.
	t.Setenv("OLLAMA_CONTEXT_LENGTH", "32768")
	cfg = config.Settings{}
	if err := Apply(&cfg, "qwen3:4b", f.URL+"/v1"); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "caveira/qwen3:4b" || cfg.ContextWindow != 32768 {
		t.Fatalf("qwen3: %s at %d", cfg.Model, cfg.ContextWindow)
	}
	if _, has := f.created["caveira/qwen3:4b"]["template"]; has {
		t.Fatal("qwen3's own template was replaced")
	}
}

// When the copy cannot be made, the model runs as it is, and caveira
// reports the window Ollama really gives it.
func TestFallsBackToTheModelAsItIs(t *testing.T) {
	clearEnv(t)
	f := newFakeOllama(t)
	f.noCreate = true
	cfg := config.Settings{ContextWindow: 999_999}
	if err := Apply(&cfg, "llama3.2:latest", f.URL+"/v1"); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "llama3.2:latest" || cfg.ContextWindow != 4096 {
		t.Fatalf("fallback ran %s at %d, want llama3.2:latest at Ollama's 4096", cfg.Model, cfg.ContextWindow)
	}

	ctx := context.Background()
	if n := ollamaContext(ctx, f.URL+"/v1", "qwen3:4b"); n != 4096 {
		t.Fatalf("model not loaded: window %d, want Ollama's default 4096", n)
	}
	other := httptest.NewServer(http.NotFoundHandler())
	defer other.Close()
	if n := ollamaContext(ctx, other.URL+"/v1", "x"); n != 0 {
		t.Fatalf("a server that is not Ollama: window %d, want 0 (unknown)", n)
	}
	if _, _, ok := ollamaPrepare(ctx, other.URL+"/v1", "x"); ok {
		t.Fatal("prepared a copy on a server that is not Ollama")
	}
}

// The picker offers the models that can run caveira as their copies,
// marks the ones that cannot, and does not list old copies twice.
func TestChoicesSayWhatEachCanDo(t *testing.T) {
	clearEnv(t)
	f := newFakeOllama(t)
	choices, err := Choices(context.Background(), f.URL+"/v1")
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) != 3 {
		t.Fatalf("got %d choices, want 3 (the old copy is not listed twice): %+v", len(choices), choices)
	}
	by := map[string]int{}
	for i, c := range choices {
		by[c.ID] = i
	}
	llama, ok := by["caveira/llama3.2:latest"]
	if !ok {
		t.Fatalf("llama3.2 not offered as its copy: %+v", choices)
	}
	if c := choices[llama]; c.Unusable != "" || !c.NoEffort || c.Note != "3.2B · local" || c.Context != Window {
		t.Errorf("llama3.2: %+v", c)
	}
	if h, ok := by["qwen3.5-0.8b-heretic:latest"]; !ok || choices[h].Unusable == "" {
		t.Errorf("a model without tool calling should be listed as unusable: %+v", choices)
	}
	if q, ok := by["caveira/qwen3:4b"]; !ok || choices[q].NoEffort {
		t.Errorf("qwen3 calls tools and thinks: %+v", choices)
	}
}
