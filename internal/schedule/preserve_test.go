package schedule

import (
	"sort"
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/walk"
)

// assertChronoPreserved checks the invariants every --preserve schedule must
// hold under the chronological contract: every commit survives, no commit is
// squashed, the schedule stays inside [start, end], and — the heart of preserve
// — taking commits in their ORIGINAL author-date order yields strictly
// increasing new times. Topology is deliberately NOT required to be monotonic:
// a rebased child authored before its parent keeps that shape.
func assertChronoPreserved(t *testing.T, dag *walk.DAG, res *Result, start, end time.Time) {
	t.Helper()
	if len(res.Squashes) != 0 {
		t.Errorf("preserve must not squash, got %d squashes", len(res.Squashes))
	}
	all := dag.All()
	if len(res.NewTimes) != len(all) {
		t.Errorf("expected all %d commits preserved, got %d times", len(all), len(res.NewTimes))
	}
	if res.Scale > 1.0 {
		t.Errorf("scale must never exceed 1.0 (preserve never expands), got %v", res.Scale)
	}

	byOrig := append([]*walk.Commit(nil), all...)
	sort.Slice(byOrig, func(i, j int) bool {
		if !byOrig[i].AuthorDate.Equal(byOrig[j].AuthorDate) {
			return byOrig[i].AuthorDate.Before(byOrig[j].AuthorDate)
		}
		return byOrig[i].OID < byOrig[j].OID
	})

	var prev time.Time
	for i, c := range byOrig {
		nt, ok := res.NewTimes[c.OID]
		if !ok {
			t.Errorf("commit %s missing from schedule", c.OID)
			continue
		}
		if nt.Before(start) {
			t.Errorf("commit %s at %v starts before window start %v", c.OID, nt, start)
		}
		if nt.After(end) {
			t.Errorf("commit %s at %v ends after window end %v", c.OID, nt, end)
		}
		if i > 0 && !nt.After(prev) {
			t.Errorf("new times not strictly increasing in original author-date order at %s: %v <= %v", c.OID, nt, prev)
		}
		prev = nt
	}
}

// makeLinearDAGDated builds a linear chain whose commits carry explicit author
// dates (minutes past a fixed base). The chain order (parent->child) is the
// slice order; the author dates may run in any order, letting tests model
// rebased histories where author time disagrees with topology.
func makeLinearDAGDated(authorMinutes []int) *walk.DAG {
	base := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	d := walk.NewDAG()
	prev := ""
	for i, m := range authorMinutes {
		c := &walk.Commit{OID: oidFor(i), AuthorDate: base.Add(time.Duration(m) * time.Minute)}
		if prev == "" {
			c.IsRoot = true
		} else {
			c.Parents = []string{prev}
		}
		d.Add(c)
		prev = oidFor(i)
	}
	return d
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

// TestPreserve_RebasedHistoryKeepsAuthorChronology is the regression test for
// the reported bug: a rebased repo whose tip commit was authored EARLIER than
// its ancestors had its chronology inverted ("recent commits became oldest")
// because the old preserve re-dated strictly by graph topology. The
// chronological contract must instead keep the original author-date order, even
// though that means the topological tip is dated before the root.
func TestPreserve_RebasedHistoryKeepsAuthorChronology(t *testing.T) {
	// Chain A(root) -> B -> C. Authored OUT of topological order: the root A was
	// authored LAST (180m), the tip C FIRST (0m) — a feature replayed onto an
	// older base, exactly like the Etch.2 "Codex on top of working?" rebase.
	d := makeLinearDAGDated([]int{180, 60, 0}) // A=180m, B=60m, C=0m
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(4 * time.Hour)

	res, err := Schedule(d, nil, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertChronoPreserved(t, d, res, start, end)

	a, b, c := res.NewTimes[oidFor(0)], res.NewTimes[oidFor(1)], res.NewTimes[oidFor(2)]
	// Author order was C(0) < B(60) < A(180), so new times must follow it.
	if !(c.Before(b) && b.Before(a)) {
		t.Errorf("expected author chronology C<B<A preserved, got A=%v B=%v C=%v", a, b, c)
	}
	// The crux: the topological tip C stays dated BEFORE the root A. The old
	// topological preserve forced A<B<C, inverting the user's chronology.
	if !c.Before(a) {
		t.Errorf("rebased tip C (%v) must stay before root A (%v)", c, a)
	}
}

func TestPreserve_MonotonicHistoryKeepsOrder(t *testing.T) {
	// A normal (non-rebased) history: author dates increase with topology. The
	// output order must match and stay within the window.
	d := makeLinearDAGDated([]int{0, 30, 90, 150})
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(3 * time.Hour)

	res, err := Schedule(d, nil, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertChronoPreserved(t, d, res, start, end)
	// Window (180m) is wider than the original span (150m): no compression.
	if res.Scale != 1.0 {
		t.Errorf("scale: got %v, want 1.0 (window wider than history, no compression)", res.Scale)
	}
}

func TestPreserve_KeepsProportionalSpacing(t *testing.T) {
	// Original gaps: A..B = 10m, B..C = 40m (4x). After compression the ratio of
	// the gaps must be preserved (~4x), just uniformly scaled.
	d := makeLinearDAGDated([]int{0, 10, 50})
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(25 * time.Minute) // forces compression of the 50m span
	res, err := Schedule(d, nil, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertChronoPreserved(t, d, res, start, end)
	if res.Scale >= 1.0 {
		t.Errorf("expected compression for a narrow window, got scale %v", res.Scale)
	}
	gapA := res.NewTimes[oidFor(1)].Sub(res.NewTimes[oidFor(0)])
	gapB := res.NewTimes[oidFor(2)].Sub(res.NewTimes[oidFor(1)])
	ratio := float64(gapB) / float64(gapA)
	if ratio < 3.5 || ratio > 4.5 {
		t.Errorf("expected ~4x spacing ratio, got %.2f (gapA=%v gapB=%v)", ratio, gapA, gapB)
	}
}

func TestPreserve_DiamondKeepsAllCommits(t *testing.T) {
	// A diamond with no author dates: chronological order falls back to OID, and
	// every commit must still survive and stay ordered/distinct.
	dag := makeDiamondDAG()
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)

	res, err := Schedule(dag, nil, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertChronoPreserved(t, dag, res, start, end)
	if len(res.NewTimes) != 4 || len(res.Squashes) != 0 {
		t.Errorf("preserve should keep all 4 commits with 0 squashes, got %d / %d squashes",
			len(res.NewTimes), len(res.Squashes))
	}
}

func TestPreserve_WindowWiderThanSpanKeepsRealGaps(t *testing.T) {
	// When the window dwarfs the original span, gaps are NOT stretched to fill
	// it: the history keeps its real author-date spacing and sits at the start.
	d := makeLinearDAGDated([]int{0, 20, 50}) // 50m span
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(10 * time.Hour)
	res, err := Schedule(d, nil, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertChronoPreserved(t, d, res, start, end)
	if res.Scale != 1.0 {
		t.Errorf("scale should be 1.0 for an over-wide window, got %v", res.Scale)
	}
	// Last commit lands at start+50m (real span), well short of the window end.
	if got, want := res.NewTimes[oidFor(2)], start.Add(50*time.Minute); !got.Equal(want) {
		t.Errorf("tip at %v, want real-gap position %v", got, want)
	}
}

func TestPreserve_FitsManyCommitsInTinyButFeasibleWindow(t *testing.T) {
	const n = 100
	mins := make([]int, n)
	for i := range mins {
		mins[i] = i * 90 // 90-minute author gaps, ~6188m total span
	}
	dag := makeLinearDAGDated(mins)
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(5 * time.Minute) // 300s >= 100 commits * 1s, so it must fit

	res, err := Schedule(dag, nil, start, end, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	assertChronoPreserved(t, dag, res, start, end)
	if len(res.NewTimes) != n {
		t.Errorf("expected %d commits, got %d", n, len(res.NewTimes))
	}
}

func TestPreserve_FailsWindowShorterThanOneSecondPerCommit(t *testing.T) {
	const n = 60
	mins := make([]int, n)
	for i := range mins {
		mins[i] = i
	}
	dag := makeLinearDAGDated(mins)
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(time.Duration(n-2) * time.Second) // one short of feasible

	if _, err := Schedule(dag, nil, start, end, true); err == nil {
		t.Fatal("expected error when window is shorter than one second per commit")
	}
}

func TestPreserve_RejectsInvertedWindow(t *testing.T) {
	dag := makeLinearDAGDated([]int{0})
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(-time.Hour)
	if _, err := Schedule(dag, nil, start, end, true); err == nil {
		t.Fatal("expected error when start is not before end")
	}
}

func TestPreserve_VersusDefault_KeepsMoreCommits(t *testing.T) {
	// Same DAG and window: default mode squashes to fit; preserve keeps all.
	const n = 12
	durations := map[string]int{}
	mins := make([]int, n)
	for i := 0; i < n; i++ {
		durations[oidFor(i)] = 60
		mins[i] = i * 60
	}
	start := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	end := start.Add(30 * time.Minute)

	defRes, err := Schedule(makeLinearDAGOID(n), durations, start, end, false)
	if err != nil {
		t.Fatalf("default schedule: %v", err)
	}
	presRes, err := Schedule(makeLinearDAGDated(mins), nil, start, end, true)
	if err != nil {
		t.Fatalf("preserve schedule: %v", err)
	}

	if len(defRes.Squashes) == 0 {
		t.Fatalf("expected default mode to squash this narrow window")
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
