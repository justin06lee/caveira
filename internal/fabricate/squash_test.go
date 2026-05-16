package fabricate

import (
	"strconv"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
)

// fileRef builds a FileRef for a path with the given content. The blob hash
// matches plumbing.ComputeHash(BlobObject, content); callers that hand the
// plan to WriteToRepo must ensure a source repo holds a blob with that hash.
func fileRef(path, content string) FileRef {
	b := []byte(content)
	return FileRef{
		Path: path,
		Blob: plumbing.ComputeHash(plumbing.BlobObject, b),
		Mode: filemode.Regular,
	}
}

// linearPlanContent returns the path->content of the files a linearPlan(n)
// references, so a source fixture repo can be built to hold those blobs.
func linearPlanContent(n int) map[string]string {
	files := map[string]string{}
	for i := 0; i < n; i++ {
		files["file"+strconv.Itoa(i)+".txt"] = "content " + strconv.Itoa(i) + "\n"
	}
	return files
}

// linearPlan builds a purely additive linear Plan with IDs 0..n-1, each
// commit adding one distinct file. master/HEAD point at the last commit.
func linearPlan(n int) *Plan {
	commits := make([]SynthCommit, n)
	for i := 0; i < n; i++ {
		var parents []int
		if i > 0 {
			parents = []int{i - 1}
		}
		commits[i] = SynthCommit{
			ID:      i,
			Parents: parents,
			Author:  Identity{Name: "A" + strconv.Itoa(i), Email: "a" + strconv.Itoa(i) + "@x.com"},
			Message: "commit " + strconv.Itoa(i),
			Added:   []FileRef{fileRef("file"+strconv.Itoa(i)+".txt", "content "+strconv.Itoa(i)+"\n")},
		}
	}
	return &Plan{
		Commits: commits,
		Refs:    map[string]int{"refs/heads/master": n - 1},
		HEAD:    n - 1,
		HeadRef: "refs/heads/master",
	}
}

func TestSquashPlan_LinearEdge(t *testing.T) {
	plan := linearPlan(4)
	durations := map[string]int{
		SyntheticOID(0): 5, SyntheticOID(1): 5,
		SyntheticOID(2): 5, SyntheticOID(3): 5,
	}

	err := SquashPlan(plan, []SquashOp{{
		Parent: SyntheticOID(1),
		Child:  SyntheticOID(2),
	}}, durations)
	if err != nil {
		t.Fatalf("SquashPlan: %v", err)
	}

	if findCommit(plan, 1) != nil {
		t.Errorf("commit 1 should be gone after squash")
	}
	c2 := findCommit(plan, 2)
	if c2 == nil {
		t.Fatalf("commit 2 should survive")
	}
	if len(c2.Parents) != 1 || c2.Parents[0] != 0 {
		t.Errorf("commit 2 parents = %v, want [0]", c2.Parents)
	}
	paths := map[string]bool{}
	for _, f := range c2.Added {
		paths[f.Path] = true
	}
	if !paths["file1.txt"] || !paths["file2.txt"] {
		t.Errorf("commit 2 Added = %v, want file1.txt and file2.txt", paths)
	}
	c3 := findCommit(plan, 3)
	if c3 == nil || len(c3.Parents) != 1 || c3.Parents[0] != 2 {
		t.Errorf("commit 3 parents = %v, want [2]", c3.Parents)
	}
	if plan.Refs["refs/heads/master"] != 3 {
		t.Errorf("master ref = %d, want 3", plan.Refs["refs/heads/master"])
	}
	if plan.HEAD != 3 {
		t.Errorf("HEAD = %d, want 3", plan.HEAD)
	}
}

func TestSquashPlan_MetadataFromLargerDuration(t *testing.T) {
	// Case 1: parent has the larger duration -> survivor takes parent metadata.
	plan := linearPlan(2)
	durations := map[string]int{SyntheticOID(0): 10, SyntheticOID(1): 3}
	if err := SquashPlan(plan, []SquashOp{{
		Parent: SyntheticOID(0), Child: SyntheticOID(1),
	}}, durations); err != nil {
		t.Fatalf("SquashPlan: %v", err)
	}
	c1 := findCommit(plan, 1)
	if c1.Message != "commit 0" {
		t.Errorf("survivor Message = %q, want %q (parent's)", c1.Message, "commit 0")
	}
	if c1.Author.Email != "a0@x.com" {
		t.Errorf("survivor Author = %v, want parent's a0@x.com", c1.Author)
	}

	// Case 2: child has the larger duration -> survivor keeps its own metadata.
	plan2 := linearPlan(2)
	durations2 := map[string]int{SyntheticOID(0): 3, SyntheticOID(1): 10}
	if err := SquashPlan(plan2, []SquashOp{{
		Parent: SyntheticOID(0), Child: SyntheticOID(1),
	}}, durations2); err != nil {
		t.Fatalf("SquashPlan: %v", err)
	}
	c1b := findCommit(plan2, 1)
	if c1b.Message != "commit 1" {
		t.Errorf("survivor Message = %q, want %q (child's own)", c1b.Message, "commit 1")
	}
	if c1b.Author.Email != "a1@x.com" {
		t.Errorf("survivor Author = %v, want child's own a1@x.com", c1b.Author)
	}
}

func TestSquashPlan_RootEdge(t *testing.T) {
	plan := linearPlan(3)
	durations := map[string]int{
		SyntheticOID(0): 5, SyntheticOID(1): 5, SyntheticOID(2): 5,
	}
	if err := SquashPlan(plan, []SquashOp{{
		Parent: SyntheticOID(0), Child: SyntheticOID(1),
	}}, durations); err != nil {
		t.Fatalf("SquashPlan: %v", err)
	}
	c1 := findCommit(plan, 1)
	if c1 == nil {
		t.Fatalf("commit 1 should survive")
	}
	// SynthCommit represents rootness as an empty Parents slice; the parent
	// was the root, so the survivor must inherit empty parents.
	if len(c1.Parents) != 0 {
		t.Errorf("survivor Parents = %v, want empty (root)", c1.Parents)
	}
}

func TestSquashPlan_FinalTreeUnchanged(t *testing.T) {
	// The source repo must hold the blobs the plan's FileRefs reference.
	src := newFixtureRepo(t, linearPlanContent(4))

	// Write the un-squashed plan and record the HEAD tree hash.
	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	unsquashed := linearPlan(4)
	dst1 := newEmptyRepo(t)
	times1 := map[string]time.Time{}
	for _, c := range unsquashed.Commits {
		times1[SyntheticOID(c.ID)] = base.Add(time.Duration(c.ID*10) * time.Minute)
	}
	if _, err := WriteToRepo(src, dst1, unsquashed, times1); err != nil {
		t.Fatalf("WriteToRepo (unsquashed): %v", err)
	}
	wantTree, err := headTree(dst1)
	if err != nil {
		t.Fatalf("head tree (unsquashed): %v", err)
	}

	// Fresh copy: squash two edges, then write and compare the tree.
	squashed := linearPlan(4)
	durations := map[string]int{
		SyntheticOID(0): 5, SyntheticOID(1): 5,
		SyntheticOID(2): 5, SyntheticOID(3): 5,
	}
	ops := []SquashOp{
		{Parent: SyntheticOID(0), Child: SyntheticOID(1)},
		{Parent: SyntheticOID(2), Child: SyntheticOID(3)},
	}
	if err := SquashPlan(squashed, ops, durations); err != nil {
		t.Fatalf("SquashPlan: %v", err)
	}

	dst2 := newEmptyRepo(t)
	times2 := map[string]time.Time{}
	for _, c := range squashed.Commits {
		times2[SyntheticOID(c.ID)] = base.Add(time.Duration(c.ID*10) * time.Minute)
	}
	if _, err := WriteToRepo(src, dst2, squashed, times2); err != nil {
		t.Fatalf("WriteToRepo (squashed): %v", err)
	}
	gotTree, err := headTree(dst2)
	if err != nil {
		t.Fatalf("head tree (squashed): %v", err)
	}

	if gotTree != wantTree {
		t.Errorf("squashed HEAD tree %s != unsquashed HEAD tree %s", gotTree, wantTree)
	}
}

// headTree returns the tree hash of the repo's HEAD commit.
func headTree(r *git.Repository) (plumbing.Hash, error) {
	head, err := r.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return commit.TreeHash, nil
}
