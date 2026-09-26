package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
	"github.com/justin06lee/caveira/tui/internal/ui"
)

// Dev mode runs caveira against a model on this machine instead of the
// paid API, for working on caveira itself: `caveira --dev`. The endpoint
// is Ollama's unless CAVEIRA_DEV_BASE_URL says otherwise; the model is the
// -m flag, CAVEIRA_DEV_MODEL, or the first of devModels that is installed.
// On Ollama the model runs as a caveira/ copy of itself (see devollama.go),
// which gives it room for caveira's prompt and, for Llama 3, a chat
// template that lets it decide whether to call a tool.

const (
	devBaseURL = "http://localhost:11434/v1"
	devTimeout = 10 * time.Second
)

// devModels are tried in order when no model is named, then anything the
// server has. Qwen's templates let the model decide whether to call a
// tool; Llama 3's own tells it to answer every message with a call, and
// even with caveira's template in its place llama3.2 is the weakest here,
// so it comes last.
var devModels = []string{
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

// applyDev points the settings at the local server and picks the model.
// A problem it can explain (no server, model not pulled) comes back as an
// error meant for the screen.
func applyDev(cfg *config.Settings, modelFlag, baseFlag string) error {
	cfg.BaseURL = strings.TrimRight(firstNonEmpty(baseFlag, os.Getenv("CAVEIRA_DEV_BASE_URL"), devBaseURL), "/")
	// The real key stays home, and local models reject reasoning levels
	// they do not have.
	cfg.APIKey, cfg.KeySource = "", "dev"
	cfg.ReasoningEffort = ""

	// A server that is not running refuses at once; the timeout is for
	// one that is running but busy, which can take seconds to list models.
	ctx, cancel := context.WithTimeout(context.Background(), devTimeout)
	defer cancel()
	models, err := llm.New(cfg.BaseURL, "").Models(ctx)
	if err != nil {
		cfg.Model = firstNonEmpty(modelFlag, os.Getenv("CAVEIRA_DEV_MODEL"), devModels[0])
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("dev mode: %s is running but did not list its models within %s; it may be busy loading a model. Try again in a moment", cfg.BaseURL, devTimeout)
		}
		return fmt.Errorf("dev mode: nothing is answering at %s. Start Ollama with `ollama serve`, "+
			"or point CAVEIRA_DEV_BASE_URL at another OpenAI-compatible server", cfg.BaseURL)
	}
	model, err := pickDevModel(models, firstNonEmpty(modelFlag, os.Getenv("CAVEIRA_DEV_MODEL")))
	cfg.Model = model
	if err != nil {
		return err
	}
	// The window is the server's, not the API's.
	if name, window, ok := ollamaPrepare(ctx, cfg.BaseURL, model); ok {
		cfg.Model, cfg.ContextWindow = name, window
	} else {
		cfg.ContextWindow = ollamaContext(ctx, cfg.BaseURL, model)
	}
	return nil
}

// pickDevModel is the named model if there is one, else the first of
// devModels installed, else whatever the server has.
func pickDevModel(models []llm.ModelInfo, want string) (string, error) {
	have := map[string]bool{}
	for _, m := range models {
		have[m.ID] = true
	}
	if want != "" {
		if !have[want] && !have[want+":latest"] {
			return want, fmt.Errorf("dev mode: %s is not on the local server. Pull it with `ollama pull %s`", want, want)
		}
		return want, nil
	}
	for _, m := range devModels {
		if have[m] {
			return m, nil
		}
	}
	for _, m := range models {
		if !isDevCopy(m.ID) {
			return m.ID, nil
		}
	}
	return devModels[0], errors.New("dev mode: the local server has no models. Pull a small one with `ollama pull qwen3:4b`")
}

// devModelList lists what the local server has for the /model picker. On
// Ollama it also asks each model what it can do: one that cannot call
// tools cannot run caveira, and one that cannot think rejects an effort.
// Models that can run caveira are offered as their caveira/ copies.
func devModelList(baseURL string) func(context.Context) ([]ui.ModelChoice, error) {
	return func(ctx context.Context) ([]ui.ModelChoice, error) {
		models, err := llm.New(baseURL, "").Models(ctx)
		if err != nil {
			return nil, err
		}
		loaded, isOllama := ollamaLoaded(ctx, baseURL)
		out := make([]ui.ModelChoice, 0, len(models))
		for _, mi := range models {
			if isDevCopy(mi.ID) {
				continue
			}
			c := ui.ModelChoice{ID: mi.ID, Note: "local"}
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
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
