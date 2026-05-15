package schedule

import (
	"testing"
	"time"

	"github.com/justin06lee/caveira/internal/walk"
)

func makeLinearDAG(durations []int) *walk.DAG {
	d := walk.NewDAG()
	prev := ""
	for i, _ := range durations {
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

	res, err := Schedule(dag, durations, windowStart, windowEnd)
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

	res, err := Schedule(dag, durations, windowStart, windowEnd)
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

	_, err := Schedule(dag, durations, windowStart, windowEnd)
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

	res, err := Schedule(dag, durations, windowStart, windowEnd)
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
