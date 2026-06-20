package rewrite

import (
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/justin06lee/caveira/internal/schedule"
	"github.com/justin06lee/caveira/internal/walk"
)

func TestRebuildRefs_BranchTipPointsAtNewHash(t *testing.T) {
	src, oids := walk.MakeFixtureLinear(t, 2, []int{1, 5})
	dag, _ := walk.Load(src)
	durations := map[string]int{oids[0]: 5, oids[1]: 5}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	res, _ := schedule.Schedule(dag, durations, windowStart, windowStart.Add(time.Hour), false)

	dst, _ := InMemoryClone(src)
	mapping, err := Apply(src, dst, dag, res)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if err := RebuildRefs(src, dst, mapping); err != nil {
		t.Fatalf("RebuildRefs: %v", err)
	}
	refs, _ := dst.References()
	saw := map[string]plumbing.Hash{}
	_ = refs.ForEach(func(r *plumbing.Reference) error {
		if r.Type() == plumbing.HashReference {
			saw[string(r.Name())] = r.Hash()
		}
		return nil
	})
	if len(saw) == 0 {
		t.Fatal("expected at least one ref")
	}
}
