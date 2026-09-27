package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/justin06lee/caveira/core/agent"
	"github.com/justin06lee/caveira/core/llm"
	"github.com/justin06lee/caveira/core/tools"
)

// Saved sessions keep the conversation as the model saw it, not as it was
// drawn, so opening one rebuilds the items from the messages: previews
// from each call's arguments, output from its tool message, and the diff
// an edit reported back to the model.

const (
	compactedPrefix   = "This session's earlier conversation was compacted."
	interruptedSuffix = "\n\n[interrupted by user]"
)

func (c *chat) load(msgs []llm.Message, ag *agent.Agent) {
	for _, m := range msgs {
		switch m.Role {
		case llm.RoleUser:
			if strings.HasPrefix(m.Content, compactedPrefix) {
				c.push(Item{Kind: "notice", Tone: "info", Text: "Earlier messages were summarized to make room."})
				continue
			}
			c.push(Item{Kind: "user", Text: m.Content})

		case llm.RoleAssistant:
			text, stopped := strings.CutSuffix(m.Content, interruptedSuffix)
			if text != "" || m.Reasoning != "" {
				c.push(Item{Kind: "assistant", Text: text, Reasoning: m.Reasoning})
			}
			if stopped {
				c.push(Item{Kind: "notice", Tone: "info", Text: "Stopped."})
			}
			for _, call := range m.ToolCalls {
				tv := &ToolView{Call: call.ID, Name: call.Function.Name, Label: toolLabel(call.Function.Name), Status: "done", Kind: tools.KindRead.String()}
				if t, ok := ag.Tools.Get(call.Function.Name); ok {
					tv.Preview = t.Preview(json.RawMessage(call.Function.Arguments))
					tv.Kind = t.Kind().String()
				}
				e := c.push(Item{Kind: "tool", Tool: tv})
				c.byCall[call.ID] = e.Item.ID
			}

		case llm.RoleTool:
			it := c.find(c.byCall[m.ToolCallID])
			if it == nil || it.Tool == nil {
				continue
			}
			t := it.Tool
			t.Output = bound(m.Content)
			if failed(m.Content) {
				t.Status = "error"
				t.Summary = strings.TrimPrefix(firstLine(m.Content), "Error: ")
			} else if m.Name == "read_file" {
				t.Summary = fmt.Sprintf("%d lines", strings.Count(strings.TrimRight(m.Content, "\n"), "\n")+1)
			}
			if m.Name == "edit_file" && strings.HasPrefix(m.Content, "Edited ") {
				if _, diff, ok := strings.Cut(m.Content, "\n\n"); ok {
					t.Diff = bound(diff)
				}
			}
		}
	}
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}

// failed spots the results tools and the agent write for a call that did
// not work, including a command that exited non-zero.
func failed(out string) bool {
	for _, p := range []string{"Error:", "The user declined", "Cancelled by the user"} {
		if strings.HasPrefix(out, p) {
			return true
		}
	}
	// bash only writes this line when the exit code is not zero.
	return strings.HasPrefix(out, "[exit code ") || strings.Contains(out, "\n[exit code ")
}
