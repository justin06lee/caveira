package walk

import "testing"

func TestLoad_Linear(t *testing.T) {
	repo, oids := MakeFixtureLinear(t, 3, []int{1, 30, 100})
	dag, err := Load(repo)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(dag.All()) != 3 {
		t.Fatalf("expected 3 commits, got %d", len(dag.All()))
	}
	root := dag.Get(oids[0])
	if !root.IsRoot {
		t.Errorf("oid[0] should be root")
	}
	if root.LinesChanged != 1 || root.NewFiles != 1 || root.FilesTouched != 1 {
		t.Errorf("root stats: lines=%d files=%d new=%d", root.LinesChanged, root.FilesTouched, root.NewFiles)
	}

	mid := dag.Get(oids[1])
	if mid.LinesChanged != 30 || mid.NewFiles != 1 || mid.FilesTouched != 1 {
		t.Errorf("mid stats: lines=%d files=%d new=%d", mid.LinesChanged, mid.FilesTouched, mid.NewFiles)
	}
}
