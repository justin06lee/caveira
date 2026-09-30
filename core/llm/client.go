package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Client talks to one OpenAI-compatible base URL.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
	// Headers are sent with every request, for routing hints like
	// x-abliteration-session-id.
	Headers map[string]string
	// MaxRetries is how many times a request that failed before producing
	// output (429, 5xx, connection reset) is retried with backoff.
	MaxRetries int
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		// No overall timeout: a long completion legitimately streams for
		// minutes. Cancellation is the caller's context.
		HTTP:       &http.Client{},
		Headers:    map[string]string{},
		MaxRetries: 3,
	}
}

// APIError is a non-2xx response with whatever the server said about it.
type APIError struct {
	Status  int
	Type    string
	Code    string
	Message string
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("HTTP %d", e.Status)
	}
	return e.Message
}

// Retryable reports whether the same request may succeed if sent again.
// A model that did not fit in memory will not fit the next time either.
func (e *APIError) Retryable() bool {
	return (e.Status == http.StatusTooManyRequests || e.Status >= 500) && !OutOfMemory(e)
}

// OutOfMemory reports whether err is a local model failing to load for
// want of memory: Ollama's own check, or CUDA, Metal or Vulkan refusing
// the buffers.
func OutOfMemory(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	for _, m := range []string{
		"out of memory", "outofdevicememory", "requires more system memory",
		"failed to allocate", "unable to allocate", "insufficient memory", "not enough memory",
	} {
		if strings.Contains(s, m) {
			return true
		}
	}
	return false
}

// Stream sends a streaming chat completion, calls fn for every delta as it
// arrives, and returns the assembled message. fn runs on the calling
// goroutine; returning an error from it aborts the stream.
func (c *Client) Stream(ctx context.Context, req Request, fn func(Delta) error) (*Completion, error) {
	req.Stream = true
	if req.StreamOptions == nil {
		req.StreamOptions = &StreamOptions{IncludeUsage: true}
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	var res *http.Response
	for attempt := 0; ; attempt++ {
		res, err = c.post(ctx, "/chat/completions", body)
		if err == nil {
			break
		}
		var apiErr *APIError
		retryable := errors.As(err, &apiErr) && apiErr.Retryable()
		if !retryable {
			// Connection-level failures are worth one more try too.
			retryable = !errors.As(err, &apiErr) && ctx.Err() == nil
		}
		if !retryable || attempt >= c.MaxRetries {
			return nil, err
		}
		wait := backoff(attempt, res)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	defer res.Body.Close()

	return readStream(res.Body, fn)
}

func backoff(attempt int, res *http.Response) time.Duration {
	if res != nil {
		if ra := res.Header.Get("Retry-After"); ra != "" {
			if secs, err := strconv.Atoi(ra); err == nil && secs > 0 && secs <= 120 {
				return time.Duration(secs) * time.Second
			}
		}
	}
	d := time.Duration(1<<uint(attempt)) * time.Second
	if d > 30*time.Second {
		d = 30 * time.Second
	}
	return d
}

// post issues the request and returns the response only on 2xx. On any other
// status the body is read for the error message and the response is closed.
func (c *Client) post(ctx context.Context, path string, body []byte) (*http.Response, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", c.BaseURL, err)
	}
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return res, nil
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 64<<10))
	return res, parseError(res.StatusCode, raw)
}

func (c *Client) setHeaders(r *http.Request) {
	if c.APIKey != "" {
		r.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	r.Header.Set("User-Agent", "caveira")
	for k, v := range c.Headers {
		r.Header.Set(k, v)
	}
}

func parseError(status int, raw []byte) error {
	e := &APIError{Status: status}
	var eb errorBody
	if json.Unmarshal(raw, &eb) == nil {
		if eb.Error != nil {
			e.Message = eb.Error.Message
			e.Type = eb.Error.Type
			e.Code = fmt.Sprint(eb.Error.Code)
			if eb.Error.Code == nil {
				e.Code = ""
			}
		} else if eb.Message != "" {
			e.Message = eb.Message
		}
	}
	if e.Message == "" {
		text := strings.TrimSpace(string(raw))
		if len(text) > 300 {
			text = text[:300] + "…"
		}
		if text != "" {
			e.Message = fmt.Sprintf("HTTP %d: %s", status, text)
		} else {
			e.Message = fmt.Sprintf("HTTP %d %s", status, http.StatusText(status))
		}
	}
	switch status {
	case http.StatusUnauthorized:
		e.Message += " (check your API key)"
	case http.StatusPaymentRequired:
		e.Message += " (out of credits)"
	}
	return e
}

// readStream parses SSE frames into deltas and accumulates the message.
func readStream(r io.Reader, fn func(Delta) error) (*Completion, error) {
	comp := &Completion{Message: Message{Role: RoleAssistant}}
	var content, reasoning strings.Builder
	calls := map[int]*ToolCall{}

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 16<<20)
	var event string
	var data strings.Builder

	flush := func() error {
		defer func() { event = ""; data.Reset() }()
		payload := strings.TrimSpace(data.String())
		if payload == "" || payload == "[DONE]" {
			return nil
		}
		if event == "error" {
			return parseError(0, []byte(payload))
		}
		var ch chunk
		if err := json.Unmarshal([]byte(payload), &ch); err != nil {
			// A malformed frame is not worth killing the stream over.
			return nil
		}
		if ch.Error != nil {
			return &APIError{Message: ch.Error.Message, Type: ch.Error.Type}
		}
		var d Delta
		if ch.Usage != nil {
			comp.Usage = *ch.Usage
			d.Usage = ch.Usage
		}
		for _, choice := range ch.Choices {
			if choice.Index != 0 {
				continue
			}
			d.Content += choice.Delta.Content
			content.WriteString(choice.Delta.Content)
			think := choice.Delta.Reasoning
			if think == "" {
				think = choice.Delta.ReasoningContent
			}
			d.Reasoning += think
			reasoning.WriteString(think)
			for _, tc := range choice.Delta.ToolCalls {
				call := calls[tc.Index]
				if call == nil {
					call = &ToolCall{Type: "function"}
					calls[tc.Index] = call
				}
				if tc.ID != "" {
					call.ID = tc.ID
				}
				if tc.Function.Name != "" {
					call.Function.Name += tc.Function.Name
				}
				call.Function.Arguments += tc.Function.Arguments
				d.ToolCalls = append(d.ToolCalls, ToolCallDelta{
					Index: tc.Index, ID: tc.ID, Name: tc.Function.Name, Arguments: tc.Function.Arguments,
				})
			}
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				comp.FinishReason = *choice.FinishReason
				d.FinishReason = *choice.FinishReason
			}
		}
		if fn != nil {
			return fn(d)
		}
		return nil
	}

	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return finish(comp, &content, &reasoning, calls), err
			}
		case strings.HasPrefix(line, ":"):
			// comment / keepalive
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[len("event:"):])
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(line[len("data:"):], " "))
		}
	}
	if err := flush(); err != nil {
		return finish(comp, &content, &reasoning, calls), err
	}
	if err := sc.Err(); err != nil {
		return finish(comp, &content, &reasoning, calls), err
	}
	return finish(comp, &content, &reasoning, calls), nil
}

func finish(comp *Completion, content, reasoning *strings.Builder, calls map[int]*ToolCall) *Completion {
	comp.Message.Content = content.String()
	comp.Message.Reasoning = reasoning.String()
	idx := make([]int, 0, len(calls))
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for n, i := range idx {
		call := *calls[i]
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_%d_%d", time.Now().UnixNano()%1_000_000, n)
		}
		comp.Message.ToolCalls = append(comp.Message.ToolCalls, call)
	}
	if comp.FinishReason == "" && len(comp.Message.ToolCalls) > 0 {
		comp.FinishReason = "tool_calls"
	}
	return comp
}

// Models lists what the key can use. Sorted by ID for stable display.
func (c *Client) Models(ctx context.Context) ([]ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/models", nil)
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)
	res, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("cannot reach %s: %w", c.BaseURL, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 {
		return nil, parseError(res.StatusCode, raw)
	}
	var out struct {
		Data []ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	sort.Slice(out.Data, func(i, j int) bool { return out.Data[i].ID < out.Data[j].ID })
	return out.Data, nil
}
