// Package local runs caveira against a model on this machine instead of
// the paid API: the terminal's --dev mode and the desktop app's local
// model setting. The endpoint is Ollama's unless CAVEIRA_DEV_BASE_URL (or
// the caller) says otherwise; the model is the one asked for,
// CAVEIRA_DEV_MODEL, or the first of Preferred that is installed. On
// Ollama the model runs as a caveira/ copy of itself (see ollama.go),
// which gives it room for caveira's prompt and, for Llama 3, a chat
// template that lets it decide whether to call a tool.
package local

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

const (
	DefaultBaseURL = "http://localhost:11434/v1"
	timeout        = 10 * time.Second
)

// Preferred models are tried in order when no model is named, then
// anything the server has. Qwen's templates let the model decide whether
// to call a tool; Llama 3's own tells it to answer every message with a
// call, and even with caveira's template in its place llama3.2 is the
// weakest here, so it comes last.
var Preferred = []string{
	"qwen3:4b",
	"qwen3:1.7b",
	"qwen2.5:3b",
	"qwen2.5:7b",
	"llama3.1:8b",
	"llama3.1:latest",
	"llama3.2:latest",
	"llama3.2",
	"llama3.2:1b",
}

// Apply points the settings at the local server and picks the model. A
// problem it can explain (no server, model not pulled) comes back as an
// error meant for the screen.
func Apply(cfg *config.Settings, model, baseURL string) error {
	cfg.BaseURL = strings.TrimRight(firstNonEmpty(baseURL, os.Getenv("CAVEIRA_DEV_BASE_URL"), DefaultBaseURL), "/")
	// The real key stays home, and local models reject reasoning levels
	// they do not have.
	cfg.APIKey, cfg.KeySource = "", "local"
	cfg.ReasoningEffort = ""

	// A server that is not running refuses at once; the timeout is for
	// one that is running but busy, which can take seconds to list models.
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	models, err := llm.New(cfg.BaseURL, "").Models(ctx)
	if err != nil {
		cfg.Model = firstNonEmpty(model, os.Getenv("CAVEIRA_DEV_MODEL"), Preferred[0])
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("%s is running but did not list its models within %s; it may be busy loading a model. Try again in a moment", cfg.BaseURL, timeout)
		}
		return fmt.Errorf("nothing is answering at %s. Start Ollama with `ollama serve`, "+
			"or point CAVEIRA_DEV_BASE_URL at another OpenAI-compatible server", cfg.BaseURL)
	}
	picked, err := PickModel(models, firstNonEmpty(model, os.Getenv("CAVEIRA_DEV_MODEL")))
	cfg.Model = picked
	if err != nil {
		return err
	}
	// The window is the server's, not the API's.
	if name, window, ok := ollamaPrepare(ctx, cfg.BaseURL, picked); ok {
		cfg.Model, cfg.ContextWindow = name, window
	} else {
		cfg.ContextWindow = ollamaContext(ctx, cfg.BaseURL, picked)
	}
	return nil
}

// PickModel is the named model if there is one, else the first of
// Preferred installed, else whatever the server has.
func PickModel(models []llm.ModelInfo, want string) (string, error) {
	have := map[string]bool{}
	for _, m := range models {
		have[m.ID] = true
	}
	if want != "" {
		if !have[want] && !have[want+":latest"] {
			return want, fmt.Errorf("%s is not on the local server. Pull it with `ollama pull %s`", want, want)
		}
		return want, nil
	}
	for _, m := range Preferred {
		if have[m] {
			return m, nil
		}
	}
	for _, m := range models {
		if !IsCopy(m.ID) {
			return m.ID, nil
		}
	}
	return Preferred[0], errors.New("the local server has no models. Pull a small one with `ollama pull qwen3:4b`")
}

// Choice is one model the local server offers.
type Choice struct {
	ID string `json:"id"`
	// Context is its window in tokens; 0 when unknown.
	Context int `json:"context"`
	// Note is shown beside it: the size, "local".
	Note string `json:"note"`
	// Unusable, when set, is why caveira cannot run on it.
	Unusable string `json:"unusable,omitempty"`
	// NoEffort marks a model that rejects reasoning levels.
	NoEffort bool `json:"noEffort,omitempty"`
}

// Choices lists what the local server has. On Ollama it also asks each
// model what it can do: one that cannot call tools cannot run caveira,
// and one that cannot think rejects an effort. Models that can run
// caveira are offered as their caveira/ copies.
func Choices(ctx context.Context, baseURL string) ([]Choice, error) {
	models, err := llm.New(baseURL, "").Models(ctx)
	if err != nil {
		return nil, err
	}
	loaded, isOllama := ollamaLoaded(ctx, baseURL)
	out := make([]Choice, 0, len(models))
	for _, mi := range models {
		if IsCopy(mi.ID) {
			continue
		}
		c := Choice{ID: mi.ID, Note: "local"}
		if isOllama {
			c.Context = ollamaWindow(loaded, mi.ID)
			if info, ok := ollamaShow(ctx, baseURL, mi.ID); ok {
				if info.Size != "" {
					c.Note = info.Size + " · local"
				}
				c.NoEffort = !slices.Contains(info.Capabilities, "thinking")
				if !slices.Contains(info.Capabilities, "tools") {
					c.Unusable = "cannot call tools"
				} else if name, window, ok := ollamaPrepare(ctx, baseURL, mi.ID); ok {
					c.ID, c.Context = name, window
				}
			}
		}
		out = append(out, c)
	}
	return out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
