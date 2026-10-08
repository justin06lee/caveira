// Package agent runs the loop: send the conversation to the model, stream
// the reply, run the tools it asked for, feed the results back, repeat until
// the model answers without tool calls.
package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/tools"
)

// Totals is cumulative usage for the session.
type Totals struct {
	Requests     int     `json:"requests"`
	InputTokens  int     `json:"input_tokens"`
	CachedTokens int     `json:"cached_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
}

// Agent holds one conversation and the means to continue it.
type Agent struct {
	Client  *llm.Client
	Model   string
	Effort  string
	MaxTok  int
	Tools   *tools.Registry
	WorkDir string
	// Confirm makes write and execute tools wait for approval.
	Confirm bool
	// ContextWindow is the model's context size in tokens; compaction kicks
	// in as the conversation approaches it.
	ContextWindow int
	// MaxSteps caps model calls per user turn, so a model stuck in a loop
	// cannot spend forever.
	MaxSteps int

	System   string
	Messages []llm.Message
	Totals   Totals
	// LastPromptTokens is how many tokens the last request occupied.
	LastPromptTokens int

	Session *Session

	// mu guards always, and Model and ContextWindow against a window that
	// reads them (ModelInfo) while a turn switches them.
	mu     sync.Mutex
	always map[string]bool
}

// New builds an agent for one working directory.
func New(client *llm.Client, cfg config.Settings, workDir, system string) *Agent {
	spec := config.Spec(cfg.Model)
	window := cfg.ContextWindow
	if window <= 0 {
		window = spec.ContextWindow
	}
	return &Agent{
		Client:        client,
		Model:         cfg.Model,
		Effort:        cfg.ReasoningEffort,
		MaxTok:        cfg.MaxTokens,
		Tools:         tools.Default(workDir),
		WorkDir:       workDir,
		Confirm:       cfg.Confirm,
		ContextWindow: window,
		MaxSteps:      200,
		System:        system,
		always:        map[string]bool{},
	}
}

// SetModel switches models mid-session and refreshes the context window.
func (a *Agent) SetModel(model string, window int) {
	if window <= 0 {
		window = config.Spec(model).ContextWindow
	}
	a.mu.Lock()
	a.Model, a.ContextWindow = model, window
	a.mu.Unlock()
}

// ModelInfo is the model and context window, safe to call while a turn
// runs in another goroutine.
func (a *Agent) ModelInfo() (model string, window int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.Model, a.ContextWindow
}

// Reset forgets the conversation but keeps the configuration.
func (a *Agent) Reset() {
	a.Messages = nil
	a.LastPromptTokens = 0
	a.Session = nil
}

func (a *Agent) request(withTools bool) llm.Request {
	msgs := make([]llm.Message, 0, len(a.Messages)+1)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: a.System})
	msgs = append(msgs, a.Messages...)
	req := llm.Request{
		Model:           a.Model,
		Messages:        msgs,
		MaxTokens:       a.MaxTok,
		ReasoningEffort: a.Effort,
	}
	if a.Session != nil {
		req.PromptCacheKey = a.Session.ID
	}
	if withTools {
		req.Tools = a.Tools.Definitions()
	}
	return req
}

// Run performs one user turn, emitting events until DoneEvent.
func (a *Agent) Run(ctx context.Context, input string, emit func(Event)) {
	a.Messages = append(a.Messages, llm.Message{Role: llm.RoleUser, Content: input})
	defer a.save()

	steps := a.MaxSteps
	if steps <= 0 {
		steps = 200
	}
	retriedContext := false

	for step := 0; step < steps; step++ {
		if a.shouldCompact() {
			if err := a.Compact(ctx, emit); err != nil && ctx.Err() != nil {
				emit(DoneEvent{Interrupted: true})
				return
			}
		}

		comp, err := a.Client.Stream(ctx, a.request(true), func(d llm.Delta) error {
			if d.Content != "" {
				emit(TextEvent{Delta: d.Content})
			}
			if d.Reasoning != "" {
				emit(ReasoningEvent{Delta: d.Reasoning})
			}
			return nil
		})

		if ctx.Err() != nil {
			// Interrupted. Keep whatever text arrived so the transcript and
			// the history agree, but drop half-parsed tool calls.
			if comp != nil && strings.TrimSpace(comp.Message.Content) != "" {
				a.Messages = append(a.Messages, llm.Message{Role: llm.RoleAssistant, Content: comp.Message.Content + "\n\n[interrupted by user]"})
			}
			emit(DoneEvent{Interrupted: true})
			return
		}
		if err != nil {
			if isContextOverflow(err) && !retriedContext && len(a.Messages) > 2 {
				retriedContext = true
				if cerr := a.Compact(ctx, emit); cerr == nil {
					continue
				}
			}
			emit(ErrorEvent{Err: err})
			emit(DoneEvent{})
			return
		}

		a.account(comp.Usage)
		emit(UsageEvent{Turn: comp.Usage, Totals: a.Totals, ContextTokens: a.LastPromptTokens})

		msg := comp.Message
		if len(msg.ToolCalls) == 0 {
			// A reply that is only tool calls written as text runs as
			// those calls; the text itself is dropped, which also takes it
			// off the screen when AssistantDone replaces what streamed.
			if calls := textToolCalls(msg.Content, a.hasTool); calls != nil {
				msg.ToolCalls, msg.Content = calls, ""
			}
		}
		if msg.Content == "" && len(msg.ToolCalls) == 0 {
			if comp.FinishReason == "length" {
				emit(ErrorEvent{Err: errors.New("the model hit its output limit before saying anything")})
			}
			// An empty reply ends the turn; nothing to feed back.
			a.Messages = append(a.Messages, msg)
			emit(AssistantDoneEvent{Message: msg})
			emit(DoneEvent{})
			return
		}
		a.Messages = append(a.Messages, msg)
		emit(AssistantDoneEvent{Message: msg})

		if len(msg.ToolCalls) == 0 {
			emit(DoneEvent{})
			return
		}

		for _, call := range msg.ToolCalls {
			a.Messages = append(a.Messages, a.runTool(ctx, call, emit))
		}
		if ctx.Err() != nil {
			emit(DoneEvent{Interrupted: true})
			return
		}
	}
	emit(ErrorEvent{Err: fmt.Errorf("stopped after %d model calls in one turn; send another message to continue", steps)})
	emit(DoneEvent{})
}

// runTool executes one call and returns the tool message for the history.
// Every call gets a result, even a cancelled one, because a tool_call without
// a matching tool message makes the whole history invalid.
func (a *Agent) runTool(ctx context.Context, call llm.ToolCall, emit func(Event)) llm.Message {
	name := call.Function.Name
	args := json.RawMessage(call.Function.Arguments)
	reply := func(r tools.Result) llm.Message {
		content := r.Output
		if strings.TrimSpace(content) == "" {
			content = "(no output)"
		}
		return llm.Message{Role: llm.RoleTool, ToolCallID: call.ID, Name: name, Content: content}
	}

	if ctx.Err() != nil {
		return reply(tools.Result{Output: "Cancelled by the user before this ran.", Summary: "cancelled", IsError: true})
	}

	tool, known := a.Tools.Get(name)
	preview := ""
	kind := tools.KindRead
	if known {
		preview = tool.Preview(args)
		kind = tool.Kind()
	}
	emit(ToolStartEvent{ID: call.ID, Name: name, Preview: preview, Kind: kind, Args: args})
	start := time.Now()

	if known && a.Confirm && kind != tools.KindRead && !a.always[name] {
		ch := make(chan Decision, 1)
		emit(ApprovalEvent{ID: call.ID, Name: name, Preview: preview, Kind: kind, Reply: ch})
		var d Decision
		select {
		case d = <-ch:
		case <-ctx.Done():
			r := tools.Result{Output: "Cancelled by the user.", Summary: "cancelled", IsError: true}
			emit(ToolEndEvent{ID: call.ID, Name: name, Result: r, Duration: time.Since(start)})
			return reply(r)
		}
		switch d {
		case AllowAlways:
			a.mu.Lock()
			a.always[name] = true
			a.mu.Unlock()
		case Deny:
			r := tools.Result{
				Output:  "The user declined this tool call. Do not retry it as-is; ask what they would prefer or take another approach.",
				Summary: "denied", IsError: true,
			}
			emit(ToolEndEvent{ID: call.ID, Name: name, Result: r, Duration: time.Since(start)})
			return reply(r)
		}
	}

	result := a.Tools.Run(ctx, name, args)
	emit(ToolEndEvent{ID: call.ID, Name: name, Result: result, Duration: time.Since(start)})
	return reply(result)
}

func (a *Agent) hasTool(name string) bool {
	_, ok := a.Tools.Get(name)
	return ok
}

func (a *Agent) account(u llm.Usage) {
	a.Totals.Requests++
	a.Totals.InputTokens += u.PromptTokens
	a.Totals.CachedTokens += u.Cached()
	a.Totals.OutputTokens += u.CompletionTokens
	spec := config.Spec(a.Model)
	fresh := u.PromptTokens - u.Cached()
	if fresh < 0 {
		fresh = 0
	}
	a.Totals.CostUSD += float64(fresh)*spec.InputPerM/1e6 +
		float64(u.Cached())*spec.CachedPerM/1e6 +
		float64(u.CompletionTokens)*spec.OutputPerM/1e6
	if u.PromptTokens > 0 {
		a.LastPromptTokens = u.PromptTokens + u.CompletionTokens
	}
}

func (a *Agent) shouldCompact() bool {
	if a.ContextWindow <= 0 || a.LastPromptTokens <= 0 || len(a.Messages) < 4 {
		return false
	}
	return float64(a.LastPromptTokens) > float64(a.ContextWindow)*0.85
}

func isContextOverflow(err error) bool {
	var apiErr *llm.APIError
	if !errors.As(err, &apiErr) || apiErr.Status != 400 {
		return false
	}
	m := strings.ToLower(apiErr.Message + " " + apiErr.Code)
	return strings.Contains(m, "context") && (strings.Contains(m, "length") || strings.Contains(m, "window") || strings.Contains(m, "exceed") || strings.Contains(m, "too long") || strings.Contains(m, "maximum"))
}

const compactInstruction = `Write a handoff note for a fresh instance of yourself that is about to take over this session with none of the conversation above. Cover, in this order and only from what actually happened:

1. What the user asked for, in their terms, including any preferences or constraints they stated.
2. What has been done: files created or changed (with paths), commands run, and their outcomes.
3. The current state: what works, what is broken, what was verified.
4. What remains to be done, and anything decided about how to do it.
5. Details worth keeping exactly: paths, identifiers, error messages, version numbers, and the user's own words where they matter.

Only include what appears in the conversation above. Never list a file, command, or outcome that is not there; if nothing has been done yet, say so in one line. Be specific and dense. Plain text with short sections; no preamble.`

// Compact replaces the history with a summary of it.
func (a *Agent) Compact(ctx context.Context, emit func(Event)) error {
	if len(a.Messages) == 0 {
		return nil
	}
	before := len(a.Messages)
	beforeTokens := a.LastPromptTokens

	msgs := make([]llm.Message, 0, len(a.Messages)+2)
	msgs = append(msgs, llm.Message{Role: llm.RoleSystem, Content: a.System})
	msgs = append(msgs, a.Messages...)
	msgs = append(msgs, llm.Message{Role: llm.RoleUser, Content: compactInstruction})
	req := llm.Request{Model: a.Model, Messages: msgs, ReasoningEffort: a.Effort}
	if a.Session != nil {
		req.PromptCacheKey = a.Session.ID
	}
	comp, err := a.Client.Stream(ctx, req, nil)
	if err != nil {
		return err
	}
	a.account(comp.Usage)
	summary := strings.TrimSpace(comp.Message.Content)
	if summary == "" {
		return errors.New("compaction produced an empty summary")
	}
	a.Messages = []llm.Message{
		{Role: llm.RoleUser, Content: "This session's earlier conversation was compacted. Here is the handoff note from the previous context:\n\n" + summary + "\n\nContinue from here."},
		{Role: llm.RoleAssistant, Content: "Understood. I have the state from the handoff note and will continue from there."},
	}
	a.LastPromptTokens = 0
	emit(CompactEvent{BeforeMessages: before, BeforeTokens: beforeTokens, Summary: summary})
	a.save()
	return nil
}

// Note appends a user-side note to the history without running a turn: the
// output of a shell command the user ran themselves, for instance.
func (a *Agent) Note(text string) {
	a.Messages = append(a.Messages, llm.Message{Role: llm.RoleUser, Content: text})
	a.save()
}
