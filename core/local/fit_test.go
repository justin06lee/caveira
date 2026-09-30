package local

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// jetsonOllama has qwen3:4b and qwen3:1.7b, nothing loaded, and records
// the window each caveira/ copy was made with.
type jetsonOllama struct {
	*httptest.Server
	mu      sync.Mutex
	windows map[string]int
}

func newJetsonOllama(t *testing.T) *jetsonOllama {
	t.Helper()
	f := &jetsonOllama{windows: map[string]int{}}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			w.Write([]byte(`{"object":"list","data":[{"id":"qwen3:4b"},{"id":"qwen3:1.7b"},{"id":"caveira/qwen3:4b"}]}`))
		case "/api/ps":
			w.Write([]byte(`{"models":[]}`))
		case "/api/show":
			w.Write([]byte(`{"capabilities":["completion","tools","thinking"],"details":{"parameter_size":"4.0B"},"template":"<|im_start|>"}`))
		case "/api/create":
			var body struct {
				Model      string
				Parameters struct {
					NumCtx int `json:"num_ctx"`
				}
			}
			json.NewDecoder(r.Body).Decode(&body)
			f.mu.Lock()
			f.windows[body.Model] = body.Parameters.NumCtx
			f.mu.Unlock()
			w.Write([]byte(`{"status":"success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

func (f *jetsonOllama) window(model string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.windows[model]
}

func resetFits(t *testing.T) {
	t.Helper()
	fitMu.Lock()
	fits = map[string]int{}
	fitMu.Unlock()
	t.Cleanup(func() {
		fitMu.Lock()
		fits = map[string]int{}
		fitMu.Unlock()
	})
}

// What Ollama said on the Jetson when qwen3:4b's 16K window did not fit.
var jetsonOOM = &llm.APIError{Status: 500, Message: "llama-server process has terminated: exit status 1: cudaMalloc failed: out of memory\nalloc_tensor_range: failed to allocate CUDA0 buffer of size 1879048192\nllama_init_from_model: failed to initialize the context: failed to allocate buffer for kv cache"}

// On a machine that cannot hold qwen3:4b's 16K window, Refit makes an
// 8K copy, and everything after starts there; when 8K does not fit
// either, it moves to qwen3:1.7b, and Apply passes over qwen3:4b.
func TestRefitStepsDownThenAcross(t *testing.T) {
	clearEnv(t)
	resetFits(t)
	f := newJetsonOllama(t)
	base := f.URL + "/v1"
	ctx := context.Background()

	cfg := config.Settings{}
	if err := Apply(&cfg, "", base); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "caveira/qwen3:4b" || cfg.ContextWindow != Window {
		t.Fatalf("picked %s@%d", cfg.Model, cfg.ContextWindow)
	}

	s, ok := Refit(ctx, base, cfg.Model, cfg.ContextWindow, jetsonOOM)
	if !ok || s.Model != "caveira/qwen3:4b" || s.Window != MinWindow || f.window("caveira/qwen3:4b") != MinWindow {
		t.Fatalf("first refit %+v %v, copy made with %d", s, ok, f.window("caveira/qwen3:4b"))
	}
	if s.Note == "" {
		t.Fatal("no note")
	}

	cfg = config.Settings{}
	if err := Apply(&cfg, "", base); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "caveira/qwen3:4b" || cfg.ContextWindow != MinWindow {
		t.Fatalf("a new chat starts at %s@%d, not where it fitted", cfg.Model, cfg.ContextWindow)
	}

	s, ok = Refit(ctx, base, s.Model, s.Window, jetsonOOM)
	if !ok || s.Model != "caveira/qwen3:1.7b" || s.Window != Window {
		t.Fatalf("second refit %+v %v", s, ok)
	}

	cfg = config.Settings{}
	if err := Apply(&cfg, "", base); err != nil {
		t.Fatal(err)
	}
	if cfg.Model != "caveira/qwen3:1.7b" {
		t.Fatalf("a new chat picks %s", cfg.Model)
	}
	choices, err := Choices(ctx, base)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range choices {
		if c.ID == "qwen3:4b" && c.Unusable != "does not fit in memory" {
			t.Fatalf("qwen3:4b offered as %+v", c)
		}
	}

	// 1.7b at 8K is the last thing to try.
	if _, ok := Refit(ctx, base, "caveira/qwen3:1.7b", MinWindow, jetsonOOM); ok {
		t.Fatal("refit past the smallest model")
	}
}

func TestRefitLeavesOtherErrors(t *testing.T) {
	resetFits(t)
	f := newJetsonOllama(t)
	if _, ok := Refit(context.Background(), f.URL+"/v1", "caveira/qwen3:4b", Window, errors.New("connection reset")); ok {
		t.Fatal("refit on an error that is not memory")
	}
	if _, ok := Refit(context.Background(), f.URL+"/v1", "qwen3:4b", Window, jetsonOOM); ok {
		t.Fatal("refit a model that is not a caveira/ copy")
	}
}
