package schedule

import (
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/walk"
)

func makeLinearDAG(durations []int) *walk.DAG {
	d := walk.NewDAG()
	prev := ""
	for i := range durations {
		oid := string(rune('A' + i))
		c := &walk.Commit{OID: oid}
		if prev != "" {
			c.Parents = []string{prev}
		} else {
			c.IsRoot = true
		}
		d.Add(c)
		prev = oid
	}
	return d
}

func TestScheduleLinearFits(t *testing.T) {
	dag := makeLinearDAG([]int{10, 20, 30})
	durations := map[string]int{"A": 10, "B": 20, "C": 30}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(2 * time.Hour)

	res, err := Schedule(dag, durations, windowStart, windowEnd, false)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	wantA := windowStart.Add(10 * time.Minute)
	wantB := wantA.Add(20 * time.Minute)
	wantC := wantB.Add(30 * time.Minute)
	if !res.NewTimes["A"].Equal(wantA) {
		t.Errorf("A: got %v, want %v", res.NewTimes["A"], wantA)
	}
	if !res.NewTimes["B"].Equal(wantB) {
		t.Errorf("B: got %v, want %v", res.NewTimes["B"], wantB)
	}
	if !res.NewTimes["C"].Equal(wantC) {
		t.Errorf("C: got %v, want %v", res.NewTimes["C"], wantC)
	}
	if res.Scale != 1.0 {
		t.Errorf("Scale: got %v, want 1.0", res.Scale)
	}
	if len(res.Squashes) != 0 {
		t.Errorf("Squashes: got %d, want 0", len(res.Squashes))
	}
}

func TestScheduleScalesToFit(t *testing.T) {
	dag := makeLinearDAG([]int{60, 60, 60}) // 180 unscaled
	durations := map[string]int{"A": 60, "B": 60, "C": 60}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(90 * time.Minute)

	res, err := Schedule(dag, durations, windowStart, windowEnd, false)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if res.Scale >= 1.0 {
		t.Errorf("expected scaling, got Scale=%v", res.Scale)
	}
	last := res.NewTimes["C"]
	if last.After(windowEnd) {
		t.Errorf("schedule overran window: last=%v end=%v", last, windowEnd)
	}
}

func TestScheduleScalingFloorIsHalf(t *testing.T) {
	// 600 unscaled minutes; window 60. Required scale 0.1, but floor is 0.5.
	dag := makeLinearDAG([]int{60, 60, 60, 60, 60, 60, 60, 60, 60, 60})
	durations := map[string]int{}
	for i := 0; i < 10; i++ {
		oid := string(rune('A' + i))
		durations[oid] = 60
	}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(60 * time.Minute)

	_, err := Schedule(dag, durations, windowStart, windowEnd, false)
	// At floor s=0.5, span is still 300 minutes > 60. With squashing now in
	// place (Task 11), the schedule fits, so success is expected.
	if err != nil {
		t.Fatalf("after squashing should fit, got %v", err)
	}
}

func TestScheduleSquashesLinearEdges(t *testing.T) {
	dag := makeLinearDAG([]int{60, 60, 60, 60, 60, 60, 60, 60, 60, 60})
	durations := map[string]int{}
	for i := 0; i < 10; i++ {
		oid := string(rune('A' + i))
		durations[oid] = 60
	}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(60 * time.Minute)

	res, err := Schedule(dag, durations, windowStart, windowEnd, false)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	if len(res.Squashes) == 0 {
		t.Fatalf("expected at least one squash, got 0")
	}
	// All surviving commits' last time must be within the window.
	for oid, tt := range res.NewTimes {
		if tt.After(windowEnd) {
			t.Errorf("commit %s ends at %v, past window end %v", oid, tt, windowEnd)
		}
	}
}

func TestScheduleLinearizesDAGWhenNoLinearEdges(t *testing.T) {
	// Diamond: A -> B, A -> C, B -> D, C -> D. No purely linear edges.
	d := walk.NewDAG()
	d.Add(&walk.Commit{OID: "A", IsRoot: true})
	d.Add(&walk.Commit{OID: "B", Parents: []string{"A"}})
	d.Add(&walk.Commit{OID: "C", Parents: []string{"A"}})
	d.Add(&walk.Commit{OID: "D", Parents: []string{"B", "C"}})

	durations := map[string]int{"A": 60, "B": 60, "C": 60, "D": 60}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(30 * time.Minute)

	res, err := Schedule(d, durations, windowStart, windowEnd, false)
	if err != nil {
		t.Fatalf("Schedule: %v", err)
	}
	for oid, tt := range res.NewTimes {
		if tt.After(windowEnd) {
			t.Errorf("commit %s ends %v past window end %v", oid, tt, windowEnd)
		}
	}
}

func TestSchedulePreserveKeepsAllCommitsAndScalesToFit(t *testing.T) {
	// 10 linear commits, 600 unscaled minutes; window 60. The normal path
	// squashes to fit. With --preserve nothing may be squashed: spacing scales
	// down instead and every commit survives inside the window.
	dag := makeLinearDAG([]int{60, 60, 60, 60, 60, 60, 60, 60, 60, 60})
	durations := map[string]int{}
	for i := 0; i < 10; i++ {
		durations[string(rune('A'+i))] = 60
	}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(60 * time.Minute)

	res, err := Schedule(dag, durations, windowStart, windowEnd, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	if len(res.Squashes) != 0 {
		t.Errorf("preserve must not squash, got %d squashes", len(res.Squashes))
	}
	if len(res.NewTimes) != 10 {
		t.Errorf("expected all 10 commits preserved, got %d", len(res.NewTimes))
	}
	for oid, tt := range res.NewTimes {
		if tt.After(windowEnd) {
			t.Errorf("commit %s ends %v past window end %v", oid, tt, windowEnd)
		}
		if tt.Before(windowStart) {
			t.Errorf("commit %s starts %v before window start %v", oid, tt, windowStart)
		}
	}
	// Spacing stays proportional, so commits remain strictly ordered A<B<...<J.
	prev := windowStart
	for i := 0; i < 10; i++ {
		got := res.NewTimes[string(rune('A'+i))]
		if !got.After(prev) {
			t.Errorf("commit %d not after previous: %v <= %v", i, got, prev)
		}
		prev = got
	}
}

func TestSchedulePreserveDoesNotScaleWhenItAlreadyFits(t *testing.T) {
	dag := makeLinearDAG([]int{10, 20, 30})
	durations := map[string]int{"A": 10, "B": 20, "C": 30}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(2 * time.Hour)

	res, err := Schedule(dag, durations, windowStart, windowEnd, true)
	if err != nil {
		t.Fatalf("Schedule preserve: %v", err)
	}
	if res.Scale != 1.0 {
		t.Errorf("Scale: got %v, want 1.0 (no compression needed)", res.Scale)
	}
	wantC := windowStart.Add(60 * time.Minute)
	if !res.NewTimes["C"].Equal(wantC) {
		t.Errorf("C: got %v, want %v", res.NewTimes["C"], wantC)
	}
}

func TestSchedulePreserveFailsWhenNarrowerThanOneSecondPerCommit(t *testing.T) {
	// 5 linear commits need at least 5 seconds; a 3-second window can't fit
	// them even at minimum spacing, and preserve refuses to merge.
	dag := makeLinearDAG([]int{60, 60, 60, 60, 60})
	durations := map[string]int{"A": 60, "B": 60, "C": 60, "D": 60, "E": 60}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(3 * time.Second)

	if _, err := Schedule(dag, durations, windowStart, windowEnd, true); err == nil {
		t.Fatal("expected error fitting 5 commits into a 3-second window")
	}
}

func TestScheduleHardFailsWhenWindowImpossiblyNarrow(t *testing.T) {
	d := walk.NewDAG()
	d.Add(&walk.Commit{OID: "A", IsRoot: true})
	durations := map[string]int{"A": 60}
	windowStart := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	windowEnd := windowStart.Add(1 * time.Second)

	_, err := Schedule(d, durations, windowStart, windowEnd, false)
	if err == nil {
		t.Fatal("expected hard-fail error")
	}
}
