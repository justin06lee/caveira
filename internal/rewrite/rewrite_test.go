package rewrite

import (
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
	res, err := schedule.Schedule(dag, durations, windowStart, windowEnd, false)
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

func TestRewrite_AppliesSquash(t *testing.T) {
	src, _ := walk.MakeFixtureLinear(t, 4, []int{1, 5, 5, 5})
	dag, err := walk.Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// 4 commits of 60 mins each; window 90 mins -> must squash at least 2.
	durations := map[string]int{}
	for _, c := range dag.All() {
		durations[c.OID] = 60
	}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(90 * time.Minute)

	res, err := schedule.Schedule(dag, durations, windowStart, windowEnd, false)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if len(res.Squashes) == 0 {
		t.Fatal("expected squashes")
	}

	dst, err := InMemoryClone(src)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, err := Apply(src, dst, res.DAG, res); err != nil {
		t.Fatalf("Apply: %v", err)
	}
	commits := allCommits(t, dst)
	if len(commits) != len(res.DAG.All()) {
		t.Fatalf("destination commit count = %d, want %d", len(commits), len(res.DAG.All()))
	}
	// Sanity: squashes did reduce the effective commit count.
	if len(res.DAG.All()) != len(dag.All())-len(res.Squashes) {
		t.Fatalf("post-squash DAG size = %d, want %d (orig %d - squashes %d)",
			len(res.DAG.All()), len(dag.All())-len(res.Squashes), len(dag.All()), len(res.Squashes))
	}
}

// TestRewrite_NormalizesMixedTimezones guards against the bug where rewritten
// commits were re-projected into each commit's ORIGINAL timezone. A source
// whose commits carry different UTC offsets (e.g. one straddling a DST
// boundary, or authors in different zones) would then emit a rewritten history
// with mixed offsets, making git log render the commits out of order even
// though the underlying instants were correct. Every rewritten commit must
// share the window's zone offset and be strictly monotonic.
func TestRewrite_NormalizesMixedTimezones(t *testing.T) {
	// The fixture authors its commits in UTC (offset 0). The window below is
	// in a non-UTC fixed zone, so any code that re-projects new times into the
	// source commit's zone (the bug) would leak offset 0 into the output. A
	// correct rewrite emits every commit in the window's zone instead.
	src, oids := walk.MakeFixtureLinear(t, 4, []int{1, 5, 5, 5})

	dag, err := walk.Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	durations := map[string]int{}
	for _, o := range oids {
		durations[o] = 5
	}

	// Window expressed in a single fixed zone — this is the zone every
	// rewritten commit must adopt.
	windowZone := time.FixedZone("WIN", -7*3600)
	windowStart := time.Date(2026, 5, 14, 13, 0, 0, 0, windowZone)
	windowEnd := windowStart.Add(2 * time.Hour)

	res, err := schedule.Schedule(dag, durations, windowStart, windowEnd, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}

	dst, err := InMemoryClone(src)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, err := Apply(src, dst, dag, res); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	commits := allCommits(t, dst) // chronological (oldest first)
	if len(commits) != 4 {
		t.Fatalf("expected 4 commits, got %d", len(commits))
	}

	wantOffset := offsetOf(windowStart)
	var prev time.Time
	for i, c := range commits {
		if got := offsetOf(c.Author.When); got != wantOffset {
			t.Errorf("commit %d author offset = %ds, want %ds (mixed zones leak through)", i, got, wantOffset)
		}
		if got := offsetOf(c.Committer.When); got != wantOffset {
			t.Errorf("commit %d committer offset = %ds, want %ds", i, got, wantOffset)
		}
		if i > 0 && !c.Author.When.After(prev) {
			t.Errorf("commit %d time %v is not after previous %v (out of order)", i, c.Author.When, prev)
		}
		prev = c.Author.When
	}
}

// TestRewrite_SquashSurvivorUsesDAGMetadata guards against the bug where Apply
// re-read author/committer/message from the SOURCE commit keyed by the
// survivor's OID, discarding the metadata the scheduler had copied onto the
// surviving DAG node. When a squash picks the parent's identity/message for the
// survivor (because the parent had the larger original duration), that choice
// must reach the output. Here the DAG node for the survivor carries "Parent"
// metadata while the source commit at the same OID carries "Child" metadata;
// the rewritten commit must reflect the DAG node, not the source object.
func TestRewrite_SquashSurvivorUsesDAGMetadata(t *testing.T) {
	src, oids := walk.MakeFixtureLinear(t, 1, []int{3})
	survivorOID := oids[0]

	srcCommit, err := src.CommitObject(plumbing.NewHash(survivorOID))
	if err != nil {
		t.Fatalf("read source commit: %v", err)
	}

	// Post-squash DAG: a single survivor node keyed by the source OID but
	// carrying the parent's metadata (what schedule.copyMetadataFrom produces).
	dag := walk.NewDAG()
	dag.Add(&walk.Commit{
		OID:        survivorOID,
		Author:     walk.Person{Name: "Parent Author", Email: "parent@example.com"},
		Committer:  walk.Person{Name: "Parent Author", Email: "parent@example.com"},
		Message:    "parent message survived the squash\n",
		AuthorDate: srcCommit.Author.When,
		IsRoot:     true,
	})

	newTime := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	res := &schedule.Result{
		NewTimes: map[string]time.Time{survivorOID: newTime},
		DAG:      dag,
		Scale:    1,
	}

	dst, err := InMemoryClone(src)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	if _, err := Apply(src, dst, dag, res); err != nil {
		t.Fatalf("Apply: %v", err)
	}

	commits := allCommits(t, dst)
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	got := commits[0]
	if got.Author.Name != "Parent Author" || got.Author.Email != "parent@example.com" {
		t.Errorf("author = %s <%s>, want Parent Author <parent@example.com> (source metadata leaked through)",
			got.Author.Name, got.Author.Email)
	}
	if got.Message != "parent message survived the squash\n" {
		t.Errorf("message = %q, want the parent's message", got.Message)
	}
	// The tree must still come from the source commit at this OID.
	if got.TreeHash != srcCommit.TreeHash {
		t.Errorf("tree = %s, want source tree %s", got.TreeHash, srcCommit.TreeHash)
	}
}

// TestRewrite_PreservesNonStandardHEADBranch guards against the bug where Apply
// hardcoded the rewritten HEAD to refs/heads/master (or main). For a repo whose
// branch was named anything else, RebuildRefs recreated only the real branch and
// left HEAD pointing at a branch that never existed — a dangling HEAD that makes
// the rewritten repo report "current branch has no commits" even though every
// commit was written correctly. HEAD must track the source's actual branch.
func TestRewrite_PreservesNonStandardHEADBranch(t *testing.T) {
	src, oids := walk.MakeFixtureLinear(t, 3, []int{1, 5, 5})

	// Move the source onto a branch named neither "main" nor "master".
	trunk := plumbing.NewBranchReferenceName("trunk")
	head, err := src.Head()
	if err != nil {
		t.Fatalf("head: %v", err)
	}
	origBranch := head.Name()
	if err := src.Storer.SetReference(plumbing.NewHashReference(trunk, head.Hash())); err != nil {
		t.Fatalf("set trunk: %v", err)
	}
	if err := src.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, trunk)); err != nil {
		t.Fatalf("set HEAD: %v", err)
	}
	if origBranch != trunk {
		if err := src.Storer.RemoveReference(origBranch); err != nil {
			t.Fatalf("remove %s: %v", origBranch, err)
		}
	}

	dag, err := walk.Load(src)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	durations := map[string]int{}
	for _, o := range oids {
		durations[o] = 5
	}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(time.Hour)
	res, err := schedule.Schedule(dag, durations, windowStart, windowEnd, true)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}

	dst, err := InMemoryClone(src)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	mapping, err := Apply(src, dst, dag, res)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if err := RebuildRefs(src, dst, mapping); err != nil {
		t.Fatalf("RebuildRefs: %v", err)
	}

	// HEAD must be symbolic and target the source's real branch.
	dh, err := dst.Reference(plumbing.HEAD, false)
	if err != nil {
		t.Fatalf("read dst HEAD: %v", err)
	}
	if dh.Type() != plumbing.SymbolicReference || dh.Target() != trunk {
		t.Fatalf("dst HEAD = %v (type %v), want symbolic -> %s", dh.Target(), dh.Type(), trunk)
	}
	// And it must resolve — a dangling HEAD (the bug) errors here.
	resolved, err := dst.Head()
	if err != nil {
		t.Fatalf("dst HEAD does not resolve (dangling branch): %v", err)
	}
	if want := mapping[oids[len(oids)-1]]; resolved.Hash() != want {
		t.Errorf("HEAD tip = %s, want rewritten newest commit %s", resolved.Hash(), want)
	}
}

func offsetOf(t time.Time) int {
	_, off := t.Zone()
	return off
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
