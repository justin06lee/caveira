package main

import (
	"context"
	"fmt"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/local"
	"github.com/justin06lee/caveira/tui/internal/ui"
)

// Dev mode runs caveira against a model on this machine instead of the
// paid API, for working on caveira itself: `caveira --dev`. The work is
// core/local's; this file only says it is dev mode that failed and hands
// the local models to the /model picker.

func applyDev(cfg *config.Settings, modelFlag, baseFlag string) error {
	err := local.Apply(cfg, modelFlag, baseFlag)
	cfg.KeySource = "dev"
	if err != nil {
		return fmt.Errorf("dev mode: %w", err)
	}
	return nil
}

// devModelList lists what the local server has for the /model picker.
func devModelList(baseURL string) func(context.Context) ([]ui.ModelChoice, error) {
	return func(ctx context.Context) ([]ui.ModelChoice, error) {
		choices, err := local.Choices(ctx, baseURL)
		if err != nil {
			return nil, err
		}
		out := make([]ui.ModelChoice, len(choices))
		for i, c := range choices {
			out[i] = ui.ModelChoice{ID: c.ID, Context: c.Context, Note: c.Note, Unusable: c.Unusable, NoEffort: c.NoEffort}
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
