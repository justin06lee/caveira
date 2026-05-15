package rewrite

import (
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/justin06lee/caveira/internal/schedule"
	"github.com/justin06lee/caveira/internal/walk"
)

func TestRewrite_LinearPreservesTreesAndMessages(t *testing.T) {
	src, oids := walk.MakeFixtureLinear(t, 3, []int{1, 5, 5})
	dag, err := walk.Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	durations := map[string]int{oids[0]: 5, oids[1]: 5, oids[2]: 5}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	res, err := schedule.Schedule(dag, durations, windowStart, windowEnd)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	dst, err := InMemoryClone(src)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, err := Apply(src, dst, dag, res); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	// Verify destination has 3 commits with the same messages and new times.
	commits := allCommits(t, dst)
	if len(commits) != 3 {
		t.Fatalf("expected 3 commits in destination, got %d", len(commits))
	}
	for i, c := range commits {
		want := windowStart.Add(time.Duration(5*(i+1)) * time.Minute)
		if !c.Author.When.Equal(want) {
			t.Errorf("commit %d author time = %v, want %v", i, c.Author.When, want)
		}
	}
}

func allCommits(t *testing.T, repo *git.Repository) []*object.Commit {
	t.Helper()
	head, err := repo.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	var out []*object.Commit
	_ = iter.ForEach(func(c *object.Commit) error {
		out = append(out, c)
		return nil
	})
	// Reverse to chronological order.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
