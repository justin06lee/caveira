package schedule

import (
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/walk"
)

// assertPreserved checks the invariants every --preserve schedule must hold:
// every original commit survives, each commit ends strictly after all of its
// parents, and the whole schedule stays inside [start, end].
func assertPreserved(t *testing.T, dag *walk.DAG, res *Result, start, end time.Time) {
	t.Helper()
	if len(res.Squashes) != 0 {
		t.Errorf("preserve must not squash, got %d squashes", len(res.Squashes))
	}
	all := dag.All()
	if len(res.NewTimes) != len(all) {
		t.Errorf("expected all %d commits preserved, got %d times", len(all), len(res.NewTimes))
	}
	if res.Scale > 1.0 {
		t.Errorf("scale must never exceed 1.0, got %v", res.Scale)
	}
	for _, c := range all {
		ct, ok := res.NewTimes[c.OID]
		if !ok {
			t.Errorf("commit %s missing from schedule", c.OID)
			continue
		}
		if ct.Before(start) {
			t.Errorf("commit %s at %v starts before window start %v", c.OID, ct, start)
		}
		if ct.After(end) {
			t.Errorf("commit %s at %v ends after window end %v", c.OID, ct, end)
		}
		for _, p := range c.Parents {
			pt, ok := res.NewTimes[p]
			if !ok {
				t.Errorf("parent %s of %s missing from schedule", p, c.OID)
				continue
			}
			if !ct.After(pt) {
				t.Errorf("commit %s (%v) does not come after parent %s (%v)", c.OID, ct, p, pt)
			}
		}
	}
}

func makeDiamondDAG() *walk.DAG {
	// A -> B, A -> C, {B,C} -> D
	d := walk.NewDAG()
	d.Add(&walk.Commit{OID: "A", IsRoot: true})
	d.Add(&walk.Commit{OID: "B", Parents: []string{"A"}})
	d.Add(&walk.Commit{OID: "C", Parents: []string{"A"}})
	d.Add(&walk.Commit{OID: "D", Parents: []string{"B", "C"}})
	return d
}

func TestPreserve_DiamondKeepsAllCommits(t *testing.T) {
	dag := makeDiamondDAG()
	durations := map[string]int{"A": 60, "B": 60, "C": 30, "D": 60}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute) // far too narrow for ~180 unscaled minutes

	res, err := Schedule(dag, durations, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertPreserved(t, dag, res, start, end)
	if res.Scale >= 1.0 {
		t.Errorf("expected compression for a narrow window, got scale %v", res.Scale)
	}
}

func TestPreserve_MergeCommitSurvives(t *testing.T) {
	// A diamond's tip D is a merge (two parents). Preserve must keep it and
	// schedule it after BOTH parents finish.
	dag := makeDiamondDAG()
	durations := map[string]int{"A": 30, "B": 30, "C": 30, "D": 1}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(20 * time.Minute)

	res, err := Schedule(dag, durations, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertPreserved(t, dag, res, start, end)
	dEnd := res.NewTimes["D"]
	if !dEnd.After(res.NewTimes["B"]) || !dEnd.After(res.NewTimes["C"]) {
		t.Errorf("merge D must follow both parents: D=%v B=%v C=%v", dEnd, res.NewTimes["B"], res.NewTimes["C"])
	}
}

func TestPreserve_SingleCommitNoScaleWhenFits(t *testing.T) {
	dag := makeLinearDAG([]int{60})
	durations := map[string]int{"A": 60}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(2 * time.Hour)

	res, err := Schedule(dag, durations, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertPreserved(t, dag, res, start, end)
	if res.Scale != 1.0 {
		t.Errorf("scale: got %v, want 1.0 (single commit fits)", res.Scale)
	}
	want := start.Add(60 * time.Minute)
	if !res.NewTimes["A"].Equal(want) {
		t.Errorf("A: got %v, want %v", res.NewTimes["A"], want)
	}
}

func TestPreserve_KeepsProportionalSpacing(t *testing.T) {
	// B's duration is 4x A's. After uniform compression the gap before B should
	// stay ~4x the gap before A — harder commits keep bigger gaps.
	dag := makeLinearDAG([]int{10, 40})
	durations := map[string]int{"A": 10, "B": 40}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(25 * time.Minute) // forces compression of the 50 unscaled minutes

	res, err := Schedule(dag, durations, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertPreserved(t, dag, res, start, end)
	gapA := res.NewTimes["A"].Sub(start)
	gapB := res.NewTimes["B"].Sub(res.NewTimes["A"])
	if gapB <= gapA {
		t.Fatalf("expected B's gap (%v) to exceed A's (%v)", gapB, gapA)
	}
	ratio := float64(gapB) / float64(gapA)
	if ratio < 3.5 || ratio > 4.5 {
		t.Errorf("expected ~4x spacing ratio, got %.2f (gapA=%v gapB=%v)", ratio, gapA, gapB)
	}
}

func TestPreserve_MonotonicSpacingShrinksWithWindow(t *testing.T) {
	// The narrower the window, the smaller the scale (never larger). Verify the
	// compressed span tracks the window across a range of widths.
	durations := map[string]int{}
	durs := make([]int, 8)
	for i := range durs {
		durs[i] = 30
		durations[string(rune('A'+i))] = 30
	}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)

	var prevScale float64 = 2.0
	for _, mins := range []int{200, 120, 60, 30, 10} {
		dag := makeLinearDAG(durs)
		end := start.Add(time.Duration(mins) * time.Minute)
		res, err := Schedule(dag, durations, start, end, true)
		if err != nil {
			t.Fatalf("window %dm: %v", mins, err)
		}
		assertPreserved(t, dag, res, start, end)
		if res.Scale > prevScale {
			t.Errorf("window %dm: scale %v should not exceed previous %v", mins, res.Scale, prevScale)
		}
		prevScale = res.Scale
	}
}

func TestPreserve_FitsManyCommitsInTinyButFeasibleWindow(t *testing.T) {
	const n = 100
	durs := make([]int, n)
	durations := map[string]int{}
	for i := range durs {
		durs[i] = 90
		durations[oidFor(i)] = 90
	}
	dag := makeLinearDAGOID(n)
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute) // 300s >= 100 commits * 1s, so it must fit

	res, err := Schedule(dag, durations, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertPreserved(t, dag, res, start, end)
	if len(res.NewTimes) != n {
		t.Errorf("expected %d commits, got %d", n, len(res.NewTimes))
	}
}

func TestPreserve_ExactBoundaryAtOneSecondPerCommit(t *testing.T) {
	const n = 60
	durations := map[string]int{}
	for i := 0; i < n; i++ {
		durations[oidFor(i)] = 30
	}
	dag := makeLinearDAGOID(n)
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Duration(n) * time.Second) // exactly 1s per commit

	res, err := Schedule(dag, durations, start, end, true)
	if err != nil {
		t.Fatalf("expected the exact-floor window to fit, got %v", err)
	}
	assertPreserved(t, dag, res, start, end)
	last := res.NewTimes[oidFor(n-1)]
	if !last.Equal(end) {
		t.Errorf("last commit should land exactly on the window end: got %v want %v", last, end)
	}
}

func TestPreserve_FailsOneSecondTooNarrow(t *testing.T) {
	const n = 60
	durations := map[string]int{}
	for i := 0; i < n; i++ {
		durations[oidFor(i)] = 30
	}
	dag := makeLinearDAGOID(n)
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Duration(n-1) * time.Second) // one second short of feasible

	if _, err := Schedule(dag, durations, start, end, true); err == nil {
		t.Fatal("expected error when window is one second short of one-second-per-commit")
	}
}

func TestPreserve_RejectsInvertedWindow(t *testing.T) {
	dag := makeLinearDAG([]int{10})
	durations := map[string]int{"A": 10}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(-time.Hour)
	if _, err := Schedule(dag, durations, start, end, true); err == nil {
		t.Fatal("expected error when start is not before end")
	}
}

func TestPreserve_VersusDefault_KeepsMoreCommits(t *testing.T) {
	// Same DAG and window: default mode squashes to fit; preserve keeps all.
	const n = 12
	durations := map[string]int{}
	for i := 0; i < n; i++ {
		durations[oidFor(i)] = 60
	}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)

	defRes, err := Schedule(makeLinearDAGOID(n), durations, start, end, false)
	if err != nil {
		t.Fatalf("default schedule: %v", err)
	}
	presRes, err := Schedule(makeLinearDAGOID(n), durations, start, end, true)
	if err != nil {
		t.Fatalf("preserve schedule: %v", err)
	}

	if len(defRes.Squashes) == 0 {
		t.Fatalf("expected default mode to squash this narrow window")
	}
	if len(defRes.DAG.All()) >= n {
		t.Errorf("default mode should drop commits, kept %d of %d", len(defRes.DAG.All()), n)
	}
	if len(presRes.NewTimes) != n || len(presRes.Squashes) != 0 {
		t.Errorf("preserve should keep all %d commits with 0 squashes, got %d commits / %d squashes",
			n, len(presRes.NewTimes), len(presRes.Squashes))
	}
}

// oidFor and makeLinearDAGOID build longer linear chains than the single-rune
// makeLinearDAG helper supports, using zero-padded numeric OIDs so >26 commits
// stay sortable and unambiguous.
func oidFor(i int) string {
	return string(rune('0'+(i/100)%10)) + string(rune('0'+(i/10)%10)) + string(rune('0'+i%10))
}

func makeLinearDAGOID(n int) *walk.DAG {
	d := walk.NewDAG()
	for i := 0; i < n; i++ {
		c := &walk.Commit{OID: oidFor(i)}
		if i == 0 {
			c.IsRoot = true
		} else {
			c.Parents = []string{oidFor(i - 1)}
		}
		d.Add(c)
	}
	return d
}
