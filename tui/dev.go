package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/justin06lee/caveira/tui/internal/config"
	"github.com/justin06lee/caveira/tui/internal/llm"
)

// Dev mode runs caveira against a model on this machine instead of the
// paid API, for working on caveira itself: `caveira --dev`. The endpoint
// is Ollama's unless CAVEIRA_DEV_BASE_URL says otherwise; the model is the
// -m flag, CAVEIRA_DEV_MODEL, or the first of devModels that is installed.

const devBaseURL = "http://localhost:11434/v1"

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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	models, err := llm.New(cfg.BaseURL, "").Models(ctx)
	if err != nil {
		cfg.Model = firstNonEmpty(modelFlag, os.Getenv("CAVEIRA_DEV_MODEL"), devModels[0])
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
