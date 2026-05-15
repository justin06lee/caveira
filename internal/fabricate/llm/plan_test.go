package llm

import "testing"

func TestParsePlan_PlainJSON(t *testing.T) {
	raw := `{"commits":[{"message":"chore: init","type":"chore","changes":[{"path":"go.mod","segments":"all"}]}]}`
	p, err := ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if len(p.Commits) != 1 || p.Commits[0].Message != "chore: init" {
		t.Fatalf("unexpected plan: %+v", p)
	}
	if !p.Commits[0].Changes[0].AllSegments {
		t.Fatal("expected AllSegments true for \"all\"")
	}
}

func TestParsePlan_FencedAndPrefixed(t *testing.T) {
	raw := "Here is the plan:\n```json\n{\"commits\":[{\"message\":\"feat: x\",\"type\":\"feat\"," +
		"\"changes\":[{\"path\":\"x.go\",\"segments\":[0,2]}]}]}\n```\nDone."
	p, err := ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if got := p.Commits[0].Changes[0].Segments; len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("segments = %v, want [0 2]", got)
	}
}

func TestParsePlan_MalformedRejected(t *testing.T) {
	if _, err := ParsePlan("no json here at all"); err == nil {
		t.Fatal("expected error for response with no JSON object")
	}
	if _, err := ParsePlan(`{"commits": [}`); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParsePlan_EmptyCommitsRejected(t *testing.T) {
	if _, err := ParsePlan(`{"commits":[]}`); err == nil {
		t.Fatal("expected error for a plan with no commits")
	}
}

func TestParsePlan_MissingSegmentsDefaultsToAll(t *testing.T) {
	raw := `{"commits":[{"message":"chore: x","type":"chore","changes":[{"path":"go.mod"}]}]}`
	p, err := ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if !p.Commits[0].Changes[0].AllSegments {
		t.Fatal("expected AllSegments true when segments key is absent")
	}
}

func TestParsePlan_BracesInsideStringLiteral(t *testing.T) {
	const message = "fix: handle } and { in parser"
	jsonObject := `{"commits":[{"message":"` + message +
		`","type":"fix","changes":[{"path":"x.go","segments":"all"}]}]}`
	raw := "Here:\n" + jsonObject + "\nthanks"
	p, err := ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if got := p.Commits[0].Message; got != message {
		t.Fatalf("message = %q, want %q", got, message)
	}
}
