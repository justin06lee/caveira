package llm

import (
	"context"
	"fmt"
	"time"
)

// DefaultTimeout is the per-call timeout when Options.Timeout is zero.
const DefaultTimeout = 120 * time.Second

// Options configures a provider at construction time.
type Options struct {
	Model   string        // optional model override; "" = provider default
	Timeout time.Duration // per-call timeout; 0 = DefaultTimeout
	Seed    int64         // forwarded to API providers when HasSeed is true
	HasSeed bool
}

func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return DefaultTimeout
}

// Provider is one LLM engine. GeneratePlan performs a single call; retries are
// the caller's responsibility.
type Provider interface {
	Name() string
	GeneratePlan(ctx context.Context, prompt string) (rawJSON string, err error)
}

// NewProvider constructs the named provider. Known names: groq, nvidia,
// claude-code, codex, opencode.
func NewProvider(name string, opts Options) (Provider, error) {
	switch name {
	case "groq":
		return newOpenAICompat("groq", "https://api.groq.com/openai/v1",
			"GROQ_API_KEY", "llama-3.3-70b-versatile", opts), nil
	case "nvidia":
		return newOpenAICompat("nvidia", "https://integrate.api.nvidia.com/v1",
			"NVIDIA_API_KEY", "meta/llama-3.3-70b-instruct", opts), nil
	case "claude-code":
		return newCLIProvider("claude-code", "claude", []string{"-p"}, opts), nil
	case "codex":
		return newCLIProvider("codex", "codex", []string{"exec"}, opts), nil
	case "opencode":
		return newCLIProvider("opencode", "opencode", []string{"run"}, opts), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", name)
	}
}
