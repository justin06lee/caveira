package fabricate

import "testing"

func TestIsModel(t *testing.T) {
	cases := []struct {
		id   Identity
		want bool
	}{
		{Identity{Name: "Claude Opus 4.7", Email: "noreply@anthropic.com"}, true},
		{Identity{Name: "Codex", Email: "codex@openai.com"}, true},
		{Identity{Name: "Cursor Agent", Email: "agent@example.com"}, true},
		{Identity{Name: "github-actions[bot]", Email: "actions@github.com"}, true},
		{Identity{Name: "Aider", Email: "aider@local"}, true},
		{Identity{Name: "GPT Agent", Email: "agent@example.com"}, true},
		{Identity{Name: "copilot", Email: "copilot@github.com"}, true},
		{Identity{Name: "Alice Cooper", Email: "alice@example.com"}, false},
		{Identity{Name: "Bob", Email: "bob@anthropic.example"}, false},
		{Identity{Name: "", Email: ""}, false},
	}
	for _, c := range cases {
		if got := IsModel(c.id); got != c.want {
			t.Errorf("IsModel(%q <%s>) = %v, want %v", c.id.Name, c.id.Email, got, c.want)
		}
	}
}
