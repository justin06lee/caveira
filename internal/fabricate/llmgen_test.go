package fabricate

import (
	"context"
	"math/rand"
	"testing"
)

// stubProvider returns a fixed raw response, recording how many times it ran.
// It structurally satisfies llm.Provider, so no llm import is needed here.
type stubProvider struct {
	responses []string
	calls     int
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) GeneratePlan(_ context.Context, _ string) (string, error) {
	r := s.responses[s.calls%len(s.responses)]
	s.calls++
	return r, nil
}

// fixtureFiles is a small source tree shared by the GenerateLLM tests.
func fixtureFiles() map[string]string {
	return map[string]string{
		"go.mod":                     "module x\n",
		"internal/walk/load.go":      "package walk\n\nfunc Load() {}\n",
		"internal/walk/load_test.go": "package walk\n",
	}
}

func TestGenerateLLM_SingleAuthorEndsAtSourceTree(t *testing.T) {
	repo := newFixtureRepo(t, fixtureFiles())
	files, err := WalkHead(repo)
	if err != nil {
		t.Fatalf("WalkHead: %v", err)
	}
	// Build an "all segments" plan covering every file.
	plan := `{"commits":[`
	for i, f := range files {
		if i > 0 {
			plan += ","
		}
		plan += `{"message":"feat: add ` + f.Path + `","type":"feat","changes":[{"path":"` +
			f.Path + `","segments":"all"}]}`
	}
	plan += `]}`

	prov := &stubProvider{responses: []string{plan}}
	p, dag, err := GenerateLLM(repo, []Identity{{Name: "A", Email: "a@x.com"}}, "single",
		prov, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("GenerateLLM: %v", err)
	}
	if len(p.Commits) == 0 || len(dag.All()) == 0 {
		t.Fatal("expected a non-empty plan and DAG")
	}
	if prov.calls != 1 {
		t.Fatalf("expected 1 provider call, got %d", prov.calls)
	}
}

func TestGenerateLLM_RetriesOnBadJSON(t *testing.T) {
	repo := newFixtureRepo(t, fixtureFiles())
	files, _ := WalkHead(repo)
	good := `{"commits":[{"message":"chore: all","type":"chore","changes":[`
	for i, f := range files {
		if i > 0 {
			good += ","
		}
		good += `{"path":"` + f.Path + `","segments":"all"}`
	}
	good += `]}]}`

	prov := &stubProvider{responses: []string{"garbage, not json", good}}
	_, _, err := GenerateLLM(repo, []Identity{{Name: "A", Email: "a@x.com"}}, "single",
		prov, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("GenerateLLM should have recovered on retry: %v", err)
	}
	if prov.calls != 2 {
		t.Fatalf("expected 2 provider calls (1 bad + 1 good), got %d", prov.calls)
	}
}

func TestGenerateLLM_HardErrorAfterRetries(t *testing.T) {
	repo := newFixtureRepo(t, fixtureFiles())
	prov := &stubProvider{responses: []string{"still not json"}}
	_, _, err := GenerateLLM(repo, []Identity{{Name: "A", Email: "a@x.com"}}, "single",
		prov, rand.New(rand.NewSource(1)))
	if err == nil {
		t.Fatal("expected a hard error after exhausting retries")
	}
	if prov.calls != maxLLMAttempts {
		t.Fatalf("expected %d provider calls, got %d", maxLLMAttempts, prov.calls)
	}
}
