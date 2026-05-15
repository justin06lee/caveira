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
	// DAG is the (possibly mutated) work DAG reflecting any squashes or
	// linearizations applied during scheduling. Callers that need to walk
	// the surviving commits (e.g. rewrite.Apply) should iterate this DAG
	// rather than the original caller-supplied one.
	DAG *walk.DAG
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

	work := cloneDAG(dag)
	d := cloneInts(durations)

	res, span, err := runSchedule(work, d, windowStart, 1.0)
	if err != nil {
		return nil, err
	}
	if span <= windowSize {
		res.DAG = work
		return res, nil
	}

	s := scaleFor(span, windowSize)
	res, span, err = scaleLoop(work, d, windowStart, windowSize, s)
	if err != nil {
		return nil, err
	}
	if span <= windowSize {
		res.DAG = work
		return res, nil
	}

	// Squash linear edges until it fits; fall back to linearizing branch points.
	var squashes []Squash
	for span > windowSize {
		edge, ok := pickSquashEdge(work, d)
		if !ok {
			if !linearizeOnce(work, d) {
				return nil, fmt.Errorf("cannot fit history into window even after maximum merging; widen the window (span=%v window=%v)", span, windowSize)
			}
		} else {
			applySquash(work, d, edge)
			squashes = append(squashes, edge)
		}
		res, span, err = runSchedule(work, d, windowStart, 0.5)
		if err != nil {
			return nil, err
		}
	}
	res.Squashes = squashes
	res.DAG = work
	return res, nil
}

func scaleLoop(dag *walk.DAG, durations map[string]int, windowStart time.Time, windowSize time.Duration, startScale float64) (*Result, time.Duration, error) {
	s := startScale
	var res *Result
	var span time.Duration
	var err error
	for attempt := 0; attempt < 5; attempt++ {
		res, span, err = runSchedule(dag, durations, windowStart, s)
		if err != nil {
			return nil, 0, err
		}
		if span <= windowSize {
			return res, span, nil
		}
		if s <= 0.5 {
			break
		}
		s -= 0.01
		if s < 0.5 {
			s = 0.5
		}
	}
	return res, span, nil
}

func scaleFor(span, windowSize time.Duration) float64 {
	s := float64(windowSize) / float64(span)
	if s < 0.5 {
		s = 0.5
	}
	if s > 1.0 {
		s = 1.0
	}
	return s
}

func cloneDAG(d *walk.DAG) *walk.DAG {
	out := walk.NewDAG()
	for _, c := range d.All() {
		cp := *c
		cp.Parents = append([]string(nil), c.Parents...)
		out.Add(&cp)
	}
	return out
}

func cloneInts(m map[string]int) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// pickSquashEdge selects the linear parent->child edge with the smallest
// min(d_p, d_c). Tiebreaker: earlier AuthorDate of the parent.
func pickSquashEdge(d *walk.DAG, durations map[string]int) (Squash, bool) {
	type cand struct {
		parent, child string
		score         int
		pDate         time.Time
	}
	var cands []cand
	for _, p := range d.All() {
		kids := d.Children(p.OID)
		if len(kids) != 1 {
			continue
		}
		child := d.Get(kids[0])
		if len(child.Parents) != 1 {
			continue
		}
		minDur := durations[p.OID]
		if durations[child.OID] < minDur {
			minDur = durations[child.OID]
		}
		cands = append(cands, cand{parent: p.OID, child: child.OID, score: minDur, pDate: p.AuthorDate})
	}
	if len(cands) == 0 {
		return Squash{}, false
	}
	best := cands[0]
	for _, c := range cands[1:] {
		if c.score < best.score || (c.score == best.score && c.pDate.Before(best.pDate)) {
			best = c
		}
	}
	return Squash{Parent: best.parent, Child: best.child}, true
}

// applySquash collapses parent and child: their roles in the DAG are taken by
// a single node keyed by child.OID (it survives). The surviving duration is
// max(d_p, d_c). The parent is removed.
func applySquash(d *walk.DAG, durations map[string]int, s Squash) {
	p := d.Get(s.Parent)
	c := d.Get(s.Child)
	if p == nil || c == nil {
		return
	}
	dur := durations[p.OID]
	if durations[c.OID] > dur {
		dur = durations[c.OID]
	}
	// Survivor metadata = whichever had the higher duration.
	survivor := c
	if durations[p.OID] > durations[c.OID] {
		copyMetadataFrom(c, p)
	}
	// Rewire: child's parents become parent's parents.
	survivor.Parents = append([]string(nil), p.Parents...)
	if p.IsRoot {
		survivor.IsRoot = true
	}
	durations[c.OID] = dur

	// Remove the parent; this also strips p.OID from any commits that had it
	// as a parent and rebuilds the children index. Grandchildren retain the
	// survivor (c.OID) as their parent, so the chain stays intact.
	d.Remove(p.OID)
}

func copyMetadataFrom(dst, src *walk.Commit) {
	dst.Author = src.Author
	dst.Committer = src.Committer
	dst.Message = src.Message
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

// linearizeOnce finds a branch point and collapses its smallest sibling
// branch into the larger one, producing one fewer parallel branch. Returns
// false if no such operation is possible.
func linearizeOnce(d *walk.DAG, durations map[string]int) bool {
	for _, p := range d.All() {
		kids := d.Children(p.OID)
		if len(kids) < 2 {
			continue
		}
		type weighted struct {
			oid    string
			weight int
		}
		var ws []weighted
		for _, k := range kids {
			ws = append(ws, weighted{oid: k, weight: branchWeight(d, durations, k)})
		}
		smallest := ws[0]
		for _, w := range ws[1:] {
			if w.weight < smallest.weight {
				smallest = w
			}
		}
		var absorber string
		for _, w := range ws {
			if w.oid != smallest.oid {
				absorber = w.oid
				break
			}
		}
		small := d.Get(smallest.oid)
		abs := d.Get(absorber)
		if small == nil || abs == nil {
			continue
		}
		mergedDur := durations[abs.OID]
		if durations[small.OID] > mergedDur {
			mergedDur = durations[small.OID]
			copyMetadataFrom(abs, small)
		}
		// abs survives. abs's parents and children stay; small's
		// children are reattached as children of abs, deduping in case
		// they already had abs as a parent (e.g. diamond shapes).
		for _, child := range d.Children(small.OID) {
			c := d.Get(child)
			seen := map[string]bool{}
			var np []string
			for _, par := range c.Parents {
				if par == small.OID {
					par = abs.OID
				}
				if seen[par] {
					continue
				}
				seen[par] = true
				np = append(np, par)
			}
			c.Parents = np
		}
		durations[abs.OID] = mergedDur
		d.Remove(small.OID)
		return true
	}
	return false
}

func branchWeight(d *walk.DAG, durations map[string]int, start string) int {
	weight := 0
	cur := start
	for cur != "" {
		weight += durations[cur]
		kids := d.Children(cur)
		if len(kids) != 1 {
			break
		}
		next := d.Get(kids[0])
		if len(next.Parents) != 1 {
			break
		}
		cur = kids[0]
	}
	return weight
}
