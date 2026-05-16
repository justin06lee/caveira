package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func codeFile(path string) []FileRef { return []FileRef{{Path: path}} }

func TestInjectCoAuthors_AppendsTrailer(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: alice, Message: "feat(walk): add walk", Added: codeFile("internal/walk/load.go")},
	}}
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 1.0, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	if !strings.Contains(plan.Commits[0].Message, "Co-Authored-By: Claude <noreply@anthropic.com>") {
		t.Fatalf("expected co-author trailer, got: %q", plan.Commits[0].Message)
	}
	if !strings.HasPrefix(plan.Commits[0].Message, "feat(walk): add walk") {
		t.Fatalf("original message not preserved: %q", plan.Commits[0].Message)
	}
}

func TestInjectCoAuthors_NoModels_NoOp(t *testing.T) {
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: Identity{Name: "Alice", Email: "alice@example.com"},
			Message: "feat: x", Added: codeFile("a.go")},
	}}
	report := &ModelReport{Profiles: map[string]PlayerProfile{}}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	if plan.Commits[0].Message != "feat: x" {
		t.Fatalf("message changed with no models: %q", plan.Commits[0].Message)
	}
}

func TestInjectCoAuthors_SkipsMergeAndEmpty(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: alice, Message: "Merge branch 'x'", IsMerge: true, Added: codeFile("a.go")},
		{ID: 1, Author: alice, Message: "wip"}, // empty Added
	}}
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 1.0, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	for _, c := range plan.Commits {
		if strings.Contains(c.Message, "Co-Authored-By") {
			t.Fatalf("merge/empty commit got a trailer: %q", c.Message)
		}
	}
}

func TestInjectCoAuthors_ZeroRatePlayerSkipped(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: alice, Message: "feat: x", Added: codeFile("a.go")},
	}}
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 0, Mix: map[string]float64{}},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	if strings.Contains(plan.Commits[0].Message, "Co-Authored-By") {
		t.Fatalf("zero-rate player got a trailer: %q", plan.Commits[0].Message)
	}
}

func TestInjectCoAuthors_Deterministic(t *testing.T) {
	build := func() *Plan {
		alice := Identity{Name: "Alice", Email: "alice@example.com"}
		return &Plan{Commits: []SynthCommit{
			{ID: 0, Author: alice, Message: "feat: a", Added: codeFile("a.go")},
			{ID: 1, Author: alice, Message: "feat: b", Added: codeFile("b.go")},
			{ID: 2, Author: alice, Message: "feat: c", Added: codeFile("c.go")},
		}}
	}
	report := &ModelReport{
		Models: []Identity{{Name: "Claude", Email: "noreply@anthropic.com"}},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 0.5, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	p1 := build()
	InjectCoAuthors(p1, report, rand.New(rand.NewSource(42)))
	p2 := build()
	InjectCoAuthors(p2, report, rand.New(rand.NewSource(42)))
	for i := range p1.Commits {
		if p1.Commits[i].Message != p2.Commits[i].Message {
			t.Fatalf("commit %d differs across seeded runs", i)
		}
	}
}
