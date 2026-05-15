package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// openAICompatProvider talks to any OpenAI-compatible /chat/completions API.
type openAICompatProvider struct {
	name      string
	baseURL   string
	apiKeyEnv string
	apiKey    string // resolved lazily from apiKeyEnv if empty
	model     string
	client    *http.Client
	opts      Options
}

func newOpenAICompat(name, baseURL, apiKeyEnv, defaultModel string, opts Options) *openAICompatProvider {
	model := defaultModel
	if opts.Model != "" {
		model = opts.Model
	}
	return &openAICompatProvider{
		name:      name,
		baseURL:   baseURL,
		apiKeyEnv: apiKeyEnv,
		model:     model,
		client:    &http.Client{Timeout: opts.timeout()},
		opts:      opts,
	}
}

func (p *openAICompatProvider) Name() string { return p.name }

func (p *openAICompatProvider) GeneratePlan(ctx context.Context, prompt string) (string, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv(p.apiKeyEnv)
	}
	if key == "" {
		return "", fmt.Errorf("%s: environment variable %s is not set", p.name, p.apiKeyEnv)
	}

	reqBody := map[string]any{
		"model":           p.model,
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if p.opts.HasSeed {
		reqBody["seed"] = p.opts.Seed
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: request failed: %w", p.name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: HTTP %d: %s", p.name, resp.StatusCode, truncate(string(body), 300))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("%s: decoding response: %w", p.name, err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("%s: response had no choices", p.name)
	}
	return parsed.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
