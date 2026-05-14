package schedule

import (
	"fmt"
	"time"

	"github.com/justin06lee/caveira/internal/walk"
)

// Result is the output of Schedule.
type Result struct {
	NewTimes map[string]time.Time // oid -> new author/committer time
	Scale    float64              // 1.0 if no scaling applied
	Squashes []Squash             // empty if no squashes applied
}

// Squash describes a single squash operation: child c is merged into parent p.
// The surviving commit retains c's children and p's parents; metadata comes
// from whichever of p or c had the larger original duration.
type Squash struct {
	Parent string
	Child  string
}

// Schedule produces new timestamps for every commit so that the entire DAG
// fits within [windowStart, windowEnd]. May scale durations and squash.
// Implements §5 of the design spec.
func Schedule(dag *walk.DAG, durations map[string]int, windowStart, windowEnd time.Time) (*Result, error) {
	if !windowStart.Before(windowEnd) {
		return nil, fmt.Errorf("window start must precede end")
	}

	res, span, err := runSchedule(dag, durations, windowStart, 1.0)
	if err != nil {
		return nil, err
	}
	windowSize := windowEnd.Sub(windowStart)
	if span <= windowSize {
		res.Scale = 1.0
		return res, nil
	}
	// scaling / squashing added in Tasks 10/11
	return res, fmt.Errorf("schedule does not fit; scaling not implemented yet")
}

// runSchedule computes one pass given a fixed scale factor.
// Returns new times and the achieved span.
func runSchedule(dag *walk.DAG, durations map[string]int, windowStart time.Time, scale float64) (*Result, time.Duration, error) {
	order, err := dag.TopologicalOrder()
	if err != nil {
		return nil, 0, err
	}

	end := map[string]time.Time{}
	for _, oid := range order {
		c := dag.Get(oid)
		start := windowStart
		for _, p := range c.Parents {
			if t, ok := end[p]; ok && t.After(start) {
				start = t
			}
		}
		d := scaledDuration(durations[oid], scale)
		end[oid] = start.Add(time.Duration(d) * time.Minute)
	}

	var maxEnd time.Time
	for _, t := range end {
		if t.After(maxEnd) {
			maxEnd = t
		}
	}
	span := maxEnd.Sub(windowStart)

	return &Result{
		NewTimes: end,
		Scale:    scale,
	}, span, nil
}

func scaledDuration(d int, scale float64) int {
	v := int(float64(d)*scale + 0.5)
	if v < 1 {
		v = 1
	}
	return v
}
