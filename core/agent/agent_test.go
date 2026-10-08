package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/justin06lee/caveira/core/config"
	"github.com/justin06lee/caveira/core/llm"
)

// sse builds a streamed response body from text and an optional tool call.
func sse(text string, call *llm.ToolCall) string {
	var sb strings.Builder
	frame := func(v any) {
		b, _ := json.Marshal(v)
		sb.WriteString("data: ")
		sb.Write(b)
		sb.WriteString("\n\n")
	}
	if text != "" {
		frame(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{"content": text}}}})
	}
	finish := "stop"
	if call != nil {
		finish = "tool_calls"
		frame(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{
			"tool_calls": []any{map[string]any{"index": 0, "id": call.ID, "type": "function", "function": map[string]any{"name": call.Function.Name, "arguments": call.Function.Arguments}}},
		}}}})
	}
	frame(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": map[string]any{}, "finish_reason": finish}}})
	frame(map[string]any{"choices": []any{}, "usage": map[string]any{"prompt_tokens": 100, "completion_tokens": 10, "total_tokens": 110}})
	sb.WriteString("data: [DONE]\n\n")
	return sb.String()
}

type scripted struct {
	mu        sync.Mutex
	responses []func(w http.ResponseWriter)
	bodies    []string
}

func (s *scripted) handler(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	s.mu.Lock()
	s.bodies = append(s.bodies, string(body))
	n := len(s.bodies) - 1
	s.mu.Unlock()
	if n >= len(s.responses) {
		w.WriteHeader(500)
		fmt.Fprint(w, `{"error":{"message":"no more scripted responses"}}`)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	s.responses[n](w)
}

func writeSSE(body string) func(http.ResponseWriter) {
	return func(w http.ResponseWriter) { _, _ = io.WriteString(w, body) }
}

func newAgent(t *testing.T, srvURL, dir string, confirm bool) *Agent {
	t.Helper()
	cfg := config.Settings{Model: "test-model", Confirm: confirm}
	return New(llm.New(srvURL, ""), cfg, dir, "system prompt")
}

func collect(ag *Agent, ctx context.Context, input string, onApproval func(ApprovalEvent)) []Event {
	var events []Event
	ag.Run(ctx, input, func(ev Event) {
		events = append(events, ev)
		if a, ok := ev.(ApprovalEvent); ok && onApproval != nil {
			onApproval(a)
		}
	})
	return events
}

func TestToolLoopWritesFile(t *testing.T) {
	dir := t.TempDir()
	call := &llm.ToolCall{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "write_file", Arguments: `{"path":"hi.txt","content":"hello\n"}`}}
	s := &scripted{responses: []func(http.ResponseWriter){
		writeSSE(sse("Writing it.", call)),
		writeSSE(sse("Done.", nil)),
	}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()

	ag := newAgent(t, srv.URL, dir, false)
	events := collect(ag, context.Background(), "make hi.txt", nil)

	got, err := os.ReadFile(filepath.Join(dir, "hi.txt"))
	if err != nil || string(got) != "hello\n" {
		t.Fatalf("file not written: %v %q", err, got)
	}
	if _, ok := events[len(events)-1].(DoneEvent); !ok {
		t.Fatalf("last event %T", events[len(events)-1])
	}
	var sawStart, sawEnd bool
	for _, ev := range events {
		switch ev := ev.(type) {
		case ToolStartEvent:
			sawStart = ev.Name == "write_file" && ev.Preview == "hi.txt"
		case ToolEndEvent:
			sawEnd = !ev.Result.IsError
		}
	}
	if !sawStart || !sawEnd {
		t.Fatalf("tool events missing: start=%v end=%v", sawStart, sawEnd)
	}
	roles := []string{}
	for _, m := range ag.Messages {
		roles = append(roles, m.Role)
	}
	if strings.Join(roles, ",") != "user,assistant,tool,assistant" {
		t.Fatalf("roles %v", roles)
	}
	if ag.Messages[2].ToolCallID != "c1" {
		t.Fatalf("tool message not linked: %+v", ag.Messages[2])
	}
	if !strings.Contains(s.bodies[1], `"tool_call_id":"c1"`) || !strings.Contains(s.bodies[1], `"tools":[`) {
		t.Fatalf("second request missing tool result or tool defs: %s", s.bodies[1])
	}
	if ag.Totals.Requests != 2 || ag.Totals.InputTokens != 200 {
		t.Fatalf("totals %+v", ag.Totals)
	}
}

func TestConfirmDeny(t *testing.T) {
	dir := t.TempDir()
	call := &llm.ToolCall{ID: "c1", Type: "function", Function: llm.FunctionCall{Name: "bash", Arguments: `{"command":"touch ran"}`}}
	s := &scripted{responses: []func(http.ResponseWriter){
		writeSSE(sse("", call)),
		writeSSE(sse("Okay, not running it.", nil)),
	}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()

	ag := newAgent(t, srv.URL, dir, true)
	asked := false
	collect(ag, context.Background(), "run it", func(a ApprovalEvent) {
		asked = a.Name == "bash" && a.Preview == "touch ran"
		a.Reply <- Deny
	})
	if !asked {
		t.Fatal("no approval requested")
	}
	if _, err := os.Stat(filepath.Join(dir, "ran")); err == nil {
		t.Fatal("command ran despite denial")
	}
	if !strings.Contains(ag.Messages[2].Content, "declined") {
		t.Fatalf("tool message %q", ag.Messages[2].Content)
	}
}

func TestConfirmAllowAlways(t *testing.T) {
	dir := t.TempDir()
	mk := func(id, name string) *llm.ToolCall {
		return &llm.ToolCall{ID: id, Type: "function", Function: llm.FunctionCall{Name: "write_file", Arguments: fmt.Sprintf(`{"path":%q,"content":"x"}`, name)}}
	}
	s := &scripted{responses: []func(http.ResponseWriter){
		writeSSE(sse("", mk("c1", "a.txt"))),
		writeSSE(sse("", mk("c2", "b.txt"))),
		writeSSE(sse("Done.", nil)),
	}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()

	ag := newAgent(t, srv.URL, dir, true)
	asks := 0
	collect(ag, context.Background(), "two files", func(a ApprovalEvent) {
		asks++
		a.Reply <- AllowAlways
	})
	if asks != 1 {
		t.Fatalf("expected one approval, got %d", asks)
	}
	for _, f := range []string{"a.txt", "b.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("%s not written", f)
		}
	}
}

func TestInterruptMidStream(t *testing.T) {
	dir := t.TempDir()
	s := &scripted{responses: []func(http.ResponseWriter){
		func(w http.ResponseWriter) {
			fl, _ := w.(http.Flusher)
			for i := 0; i < 50; i++ {
				_, _ = io.WriteString(w, `data: {"choices":[{"index":0,"delta":{"content":"word "}}]}`+"\n\n")
				if fl != nil {
					fl.Flush()
				}
				time.Sleep(20 * time.Millisecond)
			}
		},
	}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()

	ag := newAgent(t, srv.URL, dir, false)
	ctx, cancel := context.WithCancel(context.Background())
	var events []Event
	ag.Run(ctx, "go", func(ev Event) {
		events = append(events, ev)
		if _, ok := ev.(TextEvent); ok && len(events) > 3 {
			cancel()
		}
	})
	done, ok := events[len(events)-1].(DoneEvent)
	if !ok || !done.Interrupted {
		t.Fatalf("expected interrupted DoneEvent, got %#v", events[len(events)-1])
	}
	last := ag.Messages[len(ag.Messages)-1]
	if last.Role != llm.RoleAssistant || !strings.Contains(last.Content, "[interrupted by user]") {
		t.Fatalf("partial reply not recorded: %+v", last)
	}
}

func TestContextOverflowCompacts(t *testing.T) {
	dir := t.TempDir()
	s := &scripted{responses: []func(http.ResponseWriter){
		func(w http.ResponseWriter) {
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":{"message":"This model's maximum context length is 1000 tokens","type":"invalid_request_error","code":"context_length_exceeded"}}`)
		},
		writeSSE(sse("SUMMARY OF EVERYTHING", nil)),
		writeSSE(sse("Continuing.", nil)),
	}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()

	ag := newAgent(t, srv.URL, dir, false)
	ag.Messages = []llm.Message{
		{Role: llm.RoleUser, Content: "earlier"},
		{Role: llm.RoleAssistant, Content: "earlier reply"},
	}
	events := collect(ag, context.Background(), "next", nil)
	var compacted bool
	for _, ev := range events {
		if c, ok := ev.(CompactEvent); ok {
			compacted = strings.Contains(c.Summary, "SUMMARY")
		}
	}
	if !compacted {
		t.Fatalf("no compaction event in %v", events)
	}
	if !strings.Contains(ag.Messages[0].Content, "SUMMARY OF EVERYTHING") {
		t.Fatalf("history not replaced: %+v", ag.Messages[0])
	}
	if last := ag.Messages[len(ag.Messages)-1]; last.Content != "Continuing." {
		t.Fatalf("turn did not continue after compaction: %+v", last)
	}
	// The compaction request carried no tools and the instruction.
	if strings.Contains(s.bodies[1], `"tools"`) || !strings.Contains(s.bodies[1], "handoff note") {
		t.Fatalf("compaction request wrong: %s", s.bodies[1][:200])
	}
}

func TestSessionRoundTrip(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	s := &scripted{responses: []func(http.ResponseWriter){writeSSE(sse("Hi there.", nil))}}
	srv := httptest.NewServer(http.HandlerFunc(s.handler))
	defer srv.Close()

	ag := newAgent(t, srv.URL, dir, false)
	ag.NewSession()
	collect(ag, context.Background(), "hello", nil)

	latest, err := LatestSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	if latest.ID != ag.Session.ID || len(latest.Messages) != 2 || latest.Title != "hello" {
		t.Fatalf("session %+v", latest)
	}
	ag2 := newAgent(t, srv.URL, dir, false)
	ag2.Attach(latest)
	if len(ag2.Messages) != 2 || ag2.Totals.Requests != 1 {
		t.Fatalf("attach: %d messages, totals %+v", len(ag2.Messages), ag2.Totals)
	}
}
