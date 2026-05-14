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
