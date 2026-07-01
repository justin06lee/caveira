package fabricate

import (
	"math/rand"
	"testing"
)

func TestSquashPlan_RepointsTagOffRemovedCommit(t *testing.T) {
	plan := linearPlan(4)
	// Tag sits on commit 1, which is the parent removed by the squash below.
	plan.Tags = []SynthTag{{Name: "v0.1.0", CommitID: 1, Message: "Release v0.1.0"}}
	durations := map[string]int{
		SyntheticOID(0): 5, SyntheticOID(1): 5,
		SyntheticOID(2): 5, SyntheticOID(3): 5,
	}
	if err := SquashPlan(plan, []SquashOp{{Parent: SyntheticOID(1), Child: SyntheticOID(2)}}, durations); err != nil {
		t.Fatalf("SquashPlan: %v", err)
	}
	if plan.Tags[0].CommitID != 2 {
		t.Fatalf("tag CommitID = %d after squash, want 2 (the surviving child)", plan.Tags[0].CommitID)
	}
	if findCommit(plan, plan.Tags[0].CommitID) == nil {
		t.Fatal("tag points at a commit no longer in the plan")
	}
}

func TestMainlineFromHead_LinearOldestFirst(t *testing.T) {
	plan := linearPlan(5)
	got := mainlineFromHead(plan)
	want := []int{0, 1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("mainline length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mainline[%d] = %d, want %d (full: %v)", i, got[i], want[i], got)
		}
	}
}

func TestMainlineFromHead_FollowsFirstParentOnly(t *testing.T) {
	// A merge commit (id 3) with a side branch parent (id 2) that is NOT on
	// master's first-parent spine. mainline must skip id 2.
	plan := &Plan{
		Commits: []SynthCommit{
			{ID: 0},
			{ID: 1, Parents: []int{0}},
			{ID: 2, Parents: []int{0}}, // side branch off root
			{ID: 3, Parents: []int{1, 2}, IsMerge: true},
		},
		HEAD: 3,
	}
	got := mainlineFromHead(plan)
	want := []int{0, 1, 3}
	if len(got) != len(want) {
		t.Fatalf("mainline = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("mainline = %v, want %v", got, want)
		}
	}
}

func TestGenerateReleaseTags_ShortHistoryNoTags(t *testing.T) {
	plan := linearPlan(tagReleaseInterval - 1)
	if n := generateReleaseTags(plan, rand.New(rand.NewSource(1))); n != 0 {
		t.Fatalf("added %d tags for short history, want 0", n)
	}
	if len(plan.Tags) != 0 {
		t.Fatalf("plan.Tags = %v, want empty", plan.Tags)
	}
}

func TestGenerateReleaseTags_Deterministic(t *testing.T) {
	run := func() []SynthTag {
		p := linearPlan(40)
		generateReleaseTags(p, rand.New(rand.NewSource(42)))
		return p.Tags
	}
	a, b := run(), run()
	if len(a) != len(b) {
		t.Fatalf("tag counts differ: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("tag %d differs: %+v vs %+v", i, a[i], b[i])
		}
	}
}

func TestGenerateReleaseTags_ValidPlacementAndVersions(t *testing.T) {
	plan := linearPlan(40)
	generateReleaseTags(plan, rand.New(rand.NewSource(7)))
	if len(plan.Tags) == 0 {
		t.Fatal("expected at least one tag for a 40-commit history")
	}

	mainline := map[int]bool{}
	byID := map[int]*SynthCommit{}
	for _, id := range mainlineFromHead(plan) {
		mainline[id] = true
	}
	for i := range plan.Commits {
		byID[plan.Commits[i].ID] = &plan.Commits[i]
	}

	if plan.Tags[0].Name != "v0.1.0" {
		t.Errorf("first tag = %q, want v0.1.0", plan.Tags[0].Name)
	}
	seen := map[string]bool{}
	for _, tag := range plan.Tags {
		if !mainline[tag.CommitID] {
			t.Errorf("tag %q points at off-mainline commit %d", tag.Name, tag.CommitID)
		}
		if seen[tag.Name] {
			t.Errorf("duplicate tag name %q", tag.Name)
		}
		seen[tag.Name] = true
		if tag.Message == "" {
			t.Errorf("tag %q has empty message", tag.Name)
		}
		if c := byID[tag.CommitID]; c != nil && tag.Tagger != c.Author {
			t.Errorf("tag %q tagger %+v != tagged commit author %+v", tag.Name, tag.Tagger, c.Author)
		}
	}
}
