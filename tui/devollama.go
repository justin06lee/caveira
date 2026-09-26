package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"strings"
)

// Ollama runs every model with a 4096-token window unless its own settings
// say otherwise, and cuts a longer conversation from the front, system
// prompt first. caveira's prompt and tool definitions take about 3k of
// that, and the OpenAI-compatible endpoint ignores a num_ctx in the
// request. Llama 3's template also puts the tool list into the user's
// last message with an order to answer with a function call, so llama3.2
// ran a tool for "hi". Both are fixed without touching the server: dev
// mode creates a caveira/ copy of the model (Ollama shares the weights, so
// it costs no disk) with a bigger window, a steadier temperature, and,
// where the model has Llama 3's template, llama3ToolTemplate instead.

const (
	devCopyPrefix  = "caveira/"
	devWindow      = 16_384
	devTemperature = 0.3
)

func isDevCopy(model string) bool { return strings.HasPrefix(model, devCopyPrefix) }

// ollamaPrepare creates or refreshes the caveira/ copy of model and
// returns its name and window. ok is false when the server is not Ollama
// or would not make the copy; the caller then runs the model as it is.
func ollamaPrepare(ctx context.Context, baseURL, model string) (name string, window int, ok bool) {
	base := strings.TrimPrefix(model, devCopyPrefix)
	info, ok := ollamaShow(ctx, baseURL, base)
	if !ok {
		return "", 0, false
	}
	window = max(devWindow, ollamaContext(ctx, baseURL, base))
	body := map[string]any{
		"model":      devCopyPrefix + base,
		"from":       base,
		"parameters": map[string]any{"num_ctx": window, "temperature": devTemperature},
		"stream":     false,
	}
	if forcesToolCalls(info.Template) {
		body["template"] = llama3ToolTemplate
	}
	var res struct {
		Status string `json:"status"`
	}
	if !ollamaCall(ctx, http.MethodPost, ollamaRoot(baseURL)+"/api/create", body, &res) || res.Status != "success" {
		return "", 0, false
	}
	return devCopyPrefix + base, window, true
}

// forcesToolCalls spots Llama 3's template, which tells the model to
// answer with a function call whenever tools are offered.
func forcesToolCalls(template string) bool {
	return strings.Contains(template, "please respond with a JSON for a function call")
}

// llama3ToolTemplate is Llama 3's chat template with one change: the
// tools still come with the last user message, where the model was
// trained to find them, but a call is only for when the message needs
// one. Assistant tool calls render exactly as in the original, so Ollama
// still parses them.
const llama3ToolTemplate = `<|start_header_id|>system<|end_header_id|>

Cutting Knowledge Date: December 2023

{{ if .System }}{{ .System }}
{{- end }}<|eot_id|>
{{- range $i, $_ := .Messages }}
{{- $last := eq (len (slice $.Messages $i)) 1 }}
{{- if eq .Role "user" }}<|start_header_id|>user<|end_header_id|>
{{- if and $.Tools $last }}

You can call these functions:

{{ range $.Tools }}
{{- . }}
{{ end }}
If answering this message needs one of them, respond with only a JSON function call in the format {"name": function name, "parameters": dictionary of argument name and its value}. If it does not (a greeting, thanks, or something you can answer yourself), reply in plain text.

{{ .Content }}<|eot_id|>
{{- else }}

{{ .Content }}<|eot_id|>
{{- end }}{{ if $last }}<|start_header_id|>assistant<|end_header_id|>

{{ end }}
{{- else if eq .Role "assistant" }}<|start_header_id|>assistant<|end_header_id|>
{{- if .ToolCalls }}
{{ range .ToolCalls }}
{"name": "{{ .Function.Name }}", "parameters": {{ .Function.Arguments }}}{{ end }}
{{- else }}

{{ .Content }}
{{- end }}{{ if not $last }}<|eot_id|>{{ end }}
{{- else if eq .Role "tool" }}<|start_header_id|>ipython<|end_header_id|>

{{ .Content }}<|eot_id|>{{ if $last }}<|start_header_id|>assistant<|end_header_id|>

{{ end }}
{{- end }}
{{- end }}`

// ollamaRoot is Ollama's own API beside its OpenAI-compatible /v1.
func ollamaRoot(baseURL string) string {
	return strings.TrimSuffix(strings.TrimRight(baseURL, "/"), "/v1")
}

// ollamaContext is the context window Ollama runs model with as it is,
// or 0 when the server is not Ollama.
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

// ollamaModel is what Ollama says about an installed model.
type ollamaModel struct {
	Capabilities []string // "tools", "thinking", …
	Size         string   // parameter count, like "3.2B"
	Template     string   // its chat template
}

// ollamaShow asks Ollama about a model.
func ollamaShow(ctx context.Context, baseURL, model string) (ollamaModel, bool) {
	var show struct {
		Capabilities []string `json:"capabilities"`
		Template     string   `json:"template"`
		Details      struct {
			ParameterSize string `json:"parameter_size"`
		} `json:"details"`
	}
	if !ollamaCall(ctx, http.MethodPost, ollamaRoot(baseURL)+"/api/show", map[string]string{"model": model}, &show) {
		return ollamaModel{}, false
	}
	return ollamaModel{Capabilities: show.Capabilities, Size: show.Details.ParameterSize, Template: show.Template}, true
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
