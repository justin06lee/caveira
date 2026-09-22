// Package llm is a small client for OpenAI-compatible chat completion
// endpoints: abliteration.ai in production, and anything else that speaks the
// same wire format (Ollama, vLLM, llama.cpp) for local testing.
package llm

import "encoding/json"

const (
	RoleSystem    = "system"
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleTool      = "tool"
)

// Message is one turn in the conversation, in the chat-completions shape.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// Reasoning replays the model's own thinking on assistant turns. GLM-style
	// models do noticeably better on long tool-calling chains when the
	// reasoning that led to each call is sent back with it.
	Reasoning  string     `json:"reasoning,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is a request from the model to run one tool.
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// Tool is a tool definition as sent to the model.
type Tool struct {
	Type     string       `json:"type"`
	Function ToolFunction `json:"function"`
}

type ToolFunction struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// Request is a chat completion request. Only the fields caveira uses.
type Request struct {
	Model             string         `json:"model"`
	Messages          []Message      `json:"messages"`
	Tools             []Tool         `json:"tools,omitempty"`
	ToolChoice        any            `json:"tool_choice,omitempty"`
	ParallelToolCalls *bool          `json:"parallel_tool_calls,omitempty"`
	Stream            bool           `json:"stream"`
	StreamOptions     *StreamOptions `json:"stream_options,omitempty"`
	MaxTokens         int            `json:"max_tokens,omitempty"`
	Temperature       *float64       `json:"temperature,omitempty"`
	ReasoningEffort   string         `json:"reasoning_effort,omitempty"`
	PromptCacheKey    string         `json:"prompt_cache_key,omitempty"`
}

type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// Usage is the token accounting for one completion.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
	PromptDetails    *struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details,omitempty"`
	CompletionDetails *struct {
		ReasoningTokens int `json:"reasoning_tokens"`
	} `json:"completion_tokens_details,omitempty"`
}

// Cached is the number of prompt tokens served from the prefix cache.
func (u Usage) Cached() int {
	if u.PromptDetails == nil {
		return 0
	}
	return u.PromptDetails.CachedTokens
}

// Delta is one streamed fragment, already flattened from the chunk shape.
type Delta struct {
	Content   string
	Reasoning string
	ToolCalls []ToolCallDelta
	// FinishReason is set on the final content chunk: "stop", "tool_calls",
	// "length", or whatever the backend reports.
	FinishReason string
	Usage        *Usage
}

// ToolCallDelta is a fragment of a tool call. Arguments arrive spread over
// many chunks and are concatenated by index.
type ToolCallDelta struct {
	Index     int
	ID        string
	Name      string
	Arguments string
}

// Completion is an assembled streamed response.
type Completion struct {
	Message      Message
	FinishReason string
	Usage        Usage
}

// ModelInfo is one entry from /v1/models. Only the fields caveira shows.
type ModelInfo struct {
	ID            string `json:"id"`
	ContextLength int    `json:"context_length"`
	MaxOutput     int    `json:"max_output_length"`
	Pricing       *struct {
		Prompt     string `json:"prompt"`
		Completion string `json:"completion"`
		CacheRead  string `json:"input_cache_read"`
	} `json:"pricing"`
	SupportedFeatures []string `json:"supported_features"`
}

// wire shapes for decoding chunks and errors

type chunk struct {
	Choices []struct {
		Index int `json:"index"`
		Delta struct {
			Role             string `json:"role"`
			Content          string `json:"content"`
			Reasoning        string `json:"reasoning"`
			ReasoningContent string `json:"reasoning_content"`
			ToolCalls        []struct {
				Index    int    `json:"index"`
				ID       string `json:"id"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			} `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
	Usage *Usage `json:"usage"`
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
}

type errorBody struct {
	Error *struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    any    `json:"code"`
	} `json:"error"`
	// Some backends (Ollama among them) return a bare string.
	Message string `json:"message"`
}
