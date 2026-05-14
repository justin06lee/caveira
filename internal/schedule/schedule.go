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
	windowSize := windowEnd.Sub(windowStart)

	res, span, err := runSchedule(dag, durations, windowStart, 1.0)
	if err != nil {
		return nil, err
	}
	if span <= windowSize {
		return res, nil
	}

	// Linear scale candidate. Span scales near-linearly with s.
	s := float64(windowSize) / float64(span)
	if s < 0.5 {
		s = 0.5
	}
	// Try a few decrements to absorb rounding error.
	for attempt := 0; attempt < 5; attempt++ {
		res, span, err = runSchedule(dag, durations, windowStart, s)
		if err != nil {
			return nil, err
		}
		if span <= windowSize {
			return res, nil
		}
		if s <= 0.5 {
			break
		}
		s -= 0.01
		if s < 0.5 {
			s = 0.5
		}
	}
	if span <= windowSize {
		return res, nil
	}
	// Squashing implemented in Task 11.
	return nil, fmt.Errorf("schedule does not fit even at scale=0.5; squashing not implemented yet")
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
