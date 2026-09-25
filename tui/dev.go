package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strconv"
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

const (
	devBaseURL = "http://localhost:11434/v1"
	devTimeout = 10 * time.Second
)

// devModels are tried in order when no model is named: small models whose
// Ollama templates take tools, then anything the server has.
var devModels = []string{
	"llama3.2:1b",
	"llama3.2:latest",
	"llama3.2",
	"qwen3:1.7b",
	"qwen3:4b",
	"qwen2.5:3b",
	"llama3.1:8b",
	"llama3.1:latest",
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
	// The window is the server's, not the API's, whichever model is picked.
	defer func() { cfg.ContextWindow = ollamaContext(ctx, cfg.BaseURL, cfg.Model) }()
	models, err := llm.New(cfg.BaseURL, "").Models(ctx)
	if err != nil {
		cfg.Model = firstNonEmpty(modelFlag, os.Getenv("CAVEIRA_DEV_MODEL"), devModels[0])
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("dev mode: %s is running but did not list its models within %s; it may be busy loading a model. Try again in a moment", cfg.BaseURL, devTimeout)
		}
		return fmt.Errorf("dev mode: nothing is answering at %s. Start Ollama with `ollama serve`, "+
			"or point CAVEIRA_DEV_BASE_URL at another OpenAI-compatible server", cfg.BaseURL)
	}
	have := map[string]bool{}
	for _, m := range models {
		have[m.ID] = true
	}

	if want := firstNonEmpty(modelFlag, os.Getenv("CAVEIRA_DEV_MODEL")); want != "" {
		cfg.Model = want
		if !have[want] && !have[want+":latest"] {
			return fmt.Errorf("dev mode: %s is not on the local server. Pull it with `ollama pull %s`", want, want)
		}
		return nil
	}
	for _, m := range devModels {
		if have[m] {
			cfg.Model = m
			return nil
		}
	}
	if len(models) > 0 {
		cfg.Model = models[0].ID
		return nil
	}
	cfg.Model = devModels[0]
	return errors.New("dev mode: the local server has no models. Pull a small one with `ollama pull llama3.2:1b`")
}

// devModelList lists what the local server has for the /model picker. On
// Ollama it also asks each model what it can do: one that cannot call
// tools cannot run caveira, and one that cannot think rejects an effort.
func devModelList(baseURL string) func(context.Context) ([]ui.ModelChoice, error) {
	return func(ctx context.Context) ([]ui.ModelChoice, error) {
		models, err := llm.New(baseURL, "").Models(ctx)
		if err != nil {
			return nil, err
		}
		loaded, isOllama := ollamaLoaded(ctx, baseURL)
		out := make([]ui.ModelChoice, 0, len(models))
		for _, mi := range models {
			c := ui.ModelChoice{ID: mi.ID, Note: "local"}
			if isOllama {
				c.Context = ollamaWindow(loaded, mi.ID)
				if caps, size, ok := ollamaShow(ctx, baseURL, mi.ID); ok {
					if size != "" {
						c.Note = size + " · local"
					}
					if !slices.Contains(caps, "tools") {
						c.Unusable = "cannot call tools"
					}
					c.NoEffort = !slices.Contains(caps, "thinking")
				}
			}
			out = append(out, c)
		}
		return out, nil
	}
}

// ollamaRoot is Ollama's own API beside its OpenAI-compatible /v1.
func ollamaRoot(baseURL string) string {
	return strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
}

// ollamaContext is the context window Ollama runs model with, or 0 when
// the server is not Ollama. Ollama cuts a conversation that outgrows it
// from the front, system prompt included, and the OpenAI-compatible
// endpoint caveira talks to cannot ask for more: only the server's own
// setting (OLLAMA_CONTEXT_LENGTH, or the app's context length) changes it.
func ollamaContext(ctx context.Context, baseURL, model string) int {
	loaded, ok := ollamaLoaded(ctx, baseURL)
	if !ok {
		return 0
	}
	return ollamaWindow(loaded, model)
}

// ollamaWindow is a model's window: its own when it is loaded, otherwise
// the server default, which is OLLAMA_CONTEXT_LENGTH if caveira can see it
// and Ollama's 4096 if not.
func ollamaWindow(loaded map[string]int, model string) int {
	for _, id := range []string{model, model + ":latest"} {
		if n := loaded[id]; n > 0 {
			return n
		}
	}
	if n, err := strconv.Atoi(os.Getenv("OLLAMA_CONTEXT_LENGTH")); err == nil && n > 0 {
		return n
	}
	return 4096
}

// ollamaLoaded maps the models Ollama has in memory to their windows. ok
// is false when the server does not answer like Ollama.
func ollamaLoaded(ctx context.Context, baseURL string) (map[string]int, bool) {
	var ps struct {
		Models []struct {
			Name          string `json:"name"`
			ContextLength int    `json:"context_length"`
		} `json:"models"`
	}
	if !ollamaCall(ctx, http.MethodGet, ollamaRoot(baseURL)+"/api/ps", nil, &ps) {
		return nil, false
	}
	out := map[string]int{}
	for _, m := range ps.Models {
		out[m.Name] = m.ContextLength
	}
	return out, true
}

// ollamaShow asks Ollama what a model can do ("tools", "thinking") and
// how big it is.
func ollamaShow(ctx context.Context, baseURL, model string) (caps []string, size string, ok bool) {
	var show struct {
		Capabilities []string `json:"capabilities"`
		Details      struct {
			ParameterSize string `json:"parameter_size"`
		} `json:"details"`
	}
	if !ollamaCall(ctx, http.MethodPost, ollamaRoot(baseURL)+"/api/show", map[string]string{"model": model}, &show) {
		return nil, "", false
	}
	return show.Capabilities, show.Details.ParameterSize, true
}

func ollamaCall(ctx context.Context, method, url string, body, out any) bool {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return false
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return false
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer res.Body.Close()
	return res.StatusCode == http.StatusOK && json.NewDecoder(res.Body).Decode(out) == nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
