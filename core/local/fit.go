package local

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/llm"
)

// A model that does not fit in memory fails when Ollama loads it, on the
// first request. On a Jetson, whose GPU shares the machine's 8 GB with
// the desktop, qwen3:4b loads with an 8K window but not with caveira's
// 16K, and Ollama's own estimate does not see that coming. Nothing in the
// API says what will fit, so caveira finds out the way Ollama does: when a
// request fails for memory, Refit halves the window, down to MinWindow,
// then moves to the next smaller model, and the agent tries again. What
// fitted is remembered while the process runs, so later chats and the
// model lists start there.

// MinWindow is the smallest window caveira is worth running in: its
// prompt and tools take about 3K of it.
const MinWindow = 8192

var (
	fitMu sync.Mutex
	// fits is the largest window known to load, per server and model;
	// 0 means the model did not load even with MinWindow.
	fits = map[string]int{}
)

func fitKey(baseURL, model string) string {
	return strings.TrimRight(baseURL, "/") + "|" + strings.TrimPrefix(model, copyPrefix)
}

// fitted is what is known about model on the server: ok is false when
// nothing is.
func fitted(baseURL, model string) (window int, ok bool) {
	fitMu.Lock()
	defer fitMu.Unlock()
	window, ok = fits[fitKey(baseURL, model)]
	return window, ok
}

func remember(baseURL, model string, window int) {
	fitMu.Lock()
	fits[fitKey(baseURL, model)] = window
	fitMu.Unlock()
}

// doesNotFit reports whether model is known not to load at all.
func doesNotFit(baseURL, model string) bool {
	w, ok := fitted(baseURL, model)
	return ok && w == 0
}

// Refitter is the agent's Refit for a local server at baseURL.
func Refitter(baseURL string) func(context.Context, string, int, error) (agent.Switch, bool) {
	return func(ctx context.Context, model string, window int, err error) (agent.Switch, bool) {
		return Refit(ctx, baseURL, model, window, err)
	}
}

// Refit answers a request to model, running with window, that failed with
// err. When the model did not fit in memory it makes a copy with half the
// window, or, at MinWindow, moves to the next smaller of Preferred that
// is installed. ok is false when there is nothing to try.
func Refit(ctx context.Context, baseURL, model string, window int, err error) (s agent.Switch, ok bool) {
	if !llm.OutOfMemory(err) || !IsCopy(model) || ctx.Err() != nil {
		return agent.Switch{}, false
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	base := strings.TrimPrefix(model, copyPrefix)
	if window > MinWindow {
		smaller := max(MinWindow, window/2)
		if name, w, ok := ollamaPrepareWindow(ctx, baseURL, base, smaller); ok {
			remember(baseURL, base, w)
			return agent.Switch{Model: name, Window: w, Note: fmt.Sprintf(
				"%s did not fit in memory with a %s window; trying %s", base, tokensK(window), tokensK(w))}, true
		}
		return agent.Switch{}, false
	}
	remember(baseURL, base, 0)
	models, lerr := llm.New(baseURL, "").Models(ctx)
	if lerr != nil {
		return agent.Switch{}, false
	}
	next := nextSmaller(models, base, func(m string) bool { return doesNotFit(baseURL, m) })
	if next == "" {
		return agent.Switch{}, false
	}
	name, w, ok := ollamaPrepare(ctx, baseURL, next)
	if !ok {
		return agent.Switch{}, false
	}
	return agent.Switch{Model: name, Window: w, Note: fmt.Sprintf(
		"%s does not fit in memory even with a %s window; switched to %s", base, tokensK(window), next)}, true
}

// nextSmaller is the first of Preferred after model that is installed
// and not known to be too big, or "" when there is none. A model not in
// Preferred has no known smaller one.
func nextSmaller(models []llm.ModelInfo, model string, tooBig func(string) bool) string {
	have := map[string]bool{}
	for _, m := range models {
		have[m.ID] = true
	}
	after := false
	for _, p := range Preferred {
		if p == model || p+":latest" == model || p == model+":latest" {
			after = true
			continue
		}
		if after && have[p] && !tooBig(p) {
			return p
		}
	}
	return ""
}

func tokensK(n int) string {
	if n%1024 == 0 {
		return fmt.Sprintf("%dK", n/1024)
	}
	return fmt.Sprintf("%.1fK", float64(n)/1024)
}
