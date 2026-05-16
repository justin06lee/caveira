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

func TestInjectCoAuthors_WeightedModelChoice(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	codex := Identity{Name: "Codex", Email: "codex@openai.com"}
	const n = 200
	commits := make([]SynthCommit, n)
	for i := 0; i < n; i++ {
		commits[i] = SynthCommit{
			ID:      i,
			Author:  alice,
			Message: "feat: change",
			Added:   codeFile("internal/walk/load.go"),
		}
	}
	plan := &Plan{Commits: commits}
	report := &ModelReport{
		Models: []Identity{claude, codex},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {
				Rate: 1.0,
				Mix: map[string]float64{
					"noreply@anthropic.com": 0.8,
					"codex@openai.com":      0.2,
				},
			},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))

	claudeCount, codexCount := 0, 0
	for _, c := range plan.Commits {
		switch {
		case strings.Contains(c.Message, "Co-Authored-By: Claude <noreply@anthropic.com>"):
			claudeCount++
		case strings.Contains(c.Message, "Co-Authored-By: Codex <codex@openai.com>"):
			codexCount++
		default:
			t.Fatalf("commit %d got no co-author trailer: %q", c.ID, c.Message)
		}
	}
	if claudeCount+codexCount != n {
		t.Fatalf("expected all %d commits trailered, got claude=%d codex=%d", n, claudeCount, codexCount)
	}
	if claudeCount == 0 || codexCount == 0 {
		t.Fatalf("expected both models picked at least once, got claude=%d codex=%d", claudeCount, codexCount)
	}
	if claudeCount <= codexCount {
		t.Fatalf("expected Claude (weight 0.8) > Codex (weight 0.2), got claude=%d codex=%d", claudeCount, codexCount)
	}
	if claudeCount*100 <= n*60 {
		t.Fatalf("expected Claude to be a clear majority (>60%% of %d), got claude=%d", n, claudeCount)
	}
	t.Logf("weighted pick: claude=%d codex=%d", claudeCount, codexCount)
}

func TestInjectCoAuthors_ChoreTypeFactor(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	const n = 200
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			// Rate*1.5 = 1.05 (clamped to 1.0) for chore; Rate*1.0 = 0.7 for code.
			"alice@example.com": {Rate: 0.7, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	buildPlan := func(filePath string) *Plan {
		commits := make([]SynthCommit, n)
		for i := 0; i < n; i++ {
			commits[i] = SynthCommit{
				ID:      i,
				Author:  alice,
				Message: "change",
				Added:   codeFile(filePath),
			}
		}
		return &Plan{Commits: commits}
	}
	countTrailers := func(plan *Plan) int {
		c := 0
		for _, sc := range plan.Commits {
			if strings.Contains(sc.Message, "Co-Authored-By") {
				c++
			}
		}
		return c
	}

	if Classify("README.md") != Chore {
		t.Fatalf("expected README.md to classify as Chore")
	}
	if Classify("internal/walk/load.go") != Code {
		t.Fatalf("expected internal/walk/load.go to classify as Code")
	}

	chorePlan := buildPlan("README.md")
	InjectCoAuthors(chorePlan, report, rand.New(rand.NewSource(1)))
	choreCount := countTrailers(chorePlan)
	if choreCount != n {
		t.Fatalf("chore p clamps to 1.0; expected all %d commits trailered, got %d", n, choreCount)
	}

	codePlan := buildPlan("internal/walk/load.go")
	InjectCoAuthors(codePlan, report, rand.New(rand.NewSource(1)))
	codeCount := countTrailers(codePlan)
	if codeCount >= choreCount {
		t.Fatalf("code p=0.7 should skip some commits; expected codeCount < choreCount, got code=%d chore=%d", codeCount, choreCount)
	}
	t.Logf("type factor: chore trailers=%d code trailers=%d", choreCount, codeCount)
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
