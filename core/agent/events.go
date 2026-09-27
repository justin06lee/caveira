package agent

import (
	"encoding/json"
	"time"

	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/tools"
)

// Event is something the UI wants to know about while a turn runs. Every
// event type is a value; the UI switches on the concrete type.
type Event interface{ isEvent() }

// TextEvent is a fragment of the assistant's visible reply.
type TextEvent struct{ Delta string }

// ReasoningEvent is a fragment of the model's thinking.
type ReasoningEvent struct{ Delta string }

// AssistantDoneEvent marks one complete assistant message. Tool calls, if
// any, follow as ToolStart/ToolEnd pairs before the next assistant message.
type AssistantDoneEvent struct{ Message llm.Message }

// ToolStartEvent fires when a tool call is about to run.
type ToolStartEvent struct {
	ID      string
	Name    string
	Preview string
	Kind    tools.Kind
	Args    json.RawMessage
}

// ToolEndEvent fires when it has finished.
type ToolEndEvent struct {
	ID       string
	Name     string
	Result   tools.Result
	Duration time.Duration
}

// Decision is the answer to an approval request.
type Decision int

const (
	Deny Decision = iota
	Allow
	AllowAlways // allow, and stop asking for this tool this session
)

// ApprovalEvent asks the UI whether a tool call may run. The agent blocks
// until a Decision is sent on Reply (or the context is cancelled).
type ApprovalEvent struct {
	ID      string
	Name    string
	Preview string
	Kind    tools.Kind
	Reply   chan<- Decision
}

// UsageEvent reports token usage after each model response.
type UsageEvent struct {
	Turn   llm.Usage
	Totals Totals
	// ContextTokens is the prompt size of the last request: how full the
	// context window is right now.
	ContextTokens int
}

// CompactEvent reports that the history was summarized.
type CompactEvent struct {
	BeforeMessages int
	BeforeTokens   int
	Summary        string
}

// ErrorEvent is a failure the turn could not recover from.
type ErrorEvent struct{ Err error }

// DoneEvent ends a turn.
type DoneEvent struct{ Interrupted bool }

func (TextEvent) isEvent()          {}
func (ReasoningEvent) isEvent()     {}
func (AssistantDoneEvent) isEvent() {}
func (ToolStartEvent) isEvent()     {}
func (ToolEndEvent) isEvent()       {}
func (ApprovalEvent) isEvent()      {}
func (UsageEvent) isEvent()         {}
func (CompactEvent) isEvent()       {}
func (ErrorEvent) isEvent()         {}
func (DoneEvent) isEvent()          {}
