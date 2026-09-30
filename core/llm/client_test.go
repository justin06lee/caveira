package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sample = `data: {"id":"1","choices":[{"index":0,"delta":{"role":"assistant","reasoning":"think "},"finish_reason":null}]}

data: {"id":"1","choices":[{"index":0,"delta":{"content":"Hel"},"finish_reason":null}]}

data: {"id":"1","choices":[{"index":0,"delta":{"content":"lo"},"finish_reason":null}]}

data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"read_file","arguments":"{\"pa"}}]},"finish_reason":null}]}

data: {"id":"1","choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"x\"}"}}]},"finish_reason":null}]}

data: {"id":"1","choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}

data: {"id":"1","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"prompt_tokens_details":{"cached_tokens":4}}}

data: [DONE]

`

func TestReadStream(t *testing.T) {
	var deltas []Delta
	comp, err := readStream(strings.NewReader(sample), func(d Delta) error {
		deltas = append(deltas, d)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if comp.Message.Content != "Hello" {
		t.Fatalf("content %q", comp.Message.Content)
	}
	if comp.Message.Reasoning != "think " {
		t.Fatalf("reasoning %q", comp.Message.Reasoning)
	}
	if len(comp.Message.ToolCalls) != 1 || comp.Message.ToolCalls[0].ID != "call_1" || comp.Message.ToolCalls[0].Function.Arguments != `{"path":"x"}` {
		t.Fatalf("tool calls %+v", comp.Message.ToolCalls)
	}
	if comp.FinishReason != "tool_calls" {
		t.Fatalf("finish %q", comp.FinishReason)
	}
	if comp.Usage.PromptTokens != 10 || comp.Usage.Cached() != 4 {
		t.Fatalf("usage %+v", comp.Usage)
	}
	if len(deltas) < 5 {
		t.Fatalf("expected deltas, got %d", len(deltas))
	}
}

func TestStreamErrorAndRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("missing auth header")
		}
		if calls == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			_, _ = w.Write([]byte(`{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limited"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sample))
	}))
	defer srv.Close()

	c := New(srv.URL, "k")
	comp, err := c.Stream(context.Background(), Request{Model: "m"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || comp.Message.Content != "Hello" {
		t.Fatalf("calls=%d content=%q", calls, comp.Message.Content)
	}
}

func TestStreamFatalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":{"message":"Invalid API key","type":"invalid_request_error","code":"invalid_api_key"}}`))
	}))
	defer srv.Close()
	_, err := New(srv.URL, "bad").Stream(context.Background(), Request{Model: "m"}, nil)
	if err == nil || !strings.Contains(err.Error(), "Invalid API key") || !strings.Contains(err.Error(), "check your API key") {
		t.Fatalf("err = %v", err)
	}
}

func TestOutOfMemory(t *testing.T) {
	for _, err := range []error{
		&APIError{Status: 500, Message: "llama-server process has terminated: exit status 1: cudaMalloc failed: out of memory"},
		errors.New("model requires more system memory (5.1 GiB) than is available (3.2 GiB)"),
		errors.New("ggml_vulkan: vk::Device::allocateMemory: ErrorOutOfDeviceMemory"),
	} {
		if !OutOfMemory(err) {
			t.Errorf("not seen as out of memory: %v", err)
		}
	}
	for _, err := range []error{nil, errors.New("connection refused"), errors.New("model 'x' not found")} {
		if OutOfMemory(err) {
			t.Errorf("seen as out of memory: %v", err)
		}
	}
}

// An out-of-memory 500 is not sent again: the model will not fit the
// second time either.
func TestOutOfMemoryIsNotRetried(t *testing.T) {
	if (&APIError{Status: 500, Message: "cudaMalloc failed: out of memory"}).Retryable() {
		t.Fatal("out of memory retried")
	}
	if !(&APIError{Status: 503, Message: "busy"}).Retryable() {
		t.Fatal("503 not retried")
	}
}
