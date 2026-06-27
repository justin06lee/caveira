package schedule

import (
	"fmt"
	"sort"
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
//
// When preserve is true, no commit is ever squashed or linearized and the
// original commit chronology is kept exactly: new timestamps follow each
// commit's original author-date order and proportional spacing, compressed
// into the window. See scheduleChronologicalPreserve.
func Schedule(dag *walk.DAG, durations map[string]int, windowStart, windowEnd time.Time, preserve bool) (*Result, error) {
	if !windowStart.Before(windowEnd) {
		return nil, fmt.Errorf("window start must precede end")
	}
	windowSize := windowEnd.Sub(windowStart)

	work := cloneDAG(dag)

	// An empty history has nothing to schedule. Return an empty result rather
	// than letting the preserve path index commits[0] (panic) or the default
	// path compute a negative span from a zero maxEnd. Callers in the CLI guard
	// this already, but direct library callers may not.
	if len(work.All()) == 0 {
		return &Result{NewTimes: map[string]time.Time{}, Scale: 1.0, DAG: work}, nil
	}

	if preserve {
		return scheduleChronologicalPreserve(work, windowStart, windowSize)
	}

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

// scheduleChronologicalPreserve fits every commit into the window WITHOUT
// squashing, preserving the original commit chronology: new timestamps follow
// the order and proportional spacing of each commit's ORIGINAL author date,
// linearly compressed into the window. Unlike the default scheduler this does
// NOT re-derive times from graph topology — so a rebased history (a child
// authored earlier than its parent) keeps that exact shape, and the rewritten
// repo looks like the source, just shifted into the new window.
//
// Spacing is scaled by min(1, window/originalSpan): a window wider than the
// original author-date span keeps the real gaps (the history sits at the start)
// rather than stretching them. Commits that share an author-second, or that
// compression would collapse together, are nudged at least one second apart in
// chronological order so every commit stays distinct and ordered. The only
// unfittable case is a window shorter than one second per commit.
func scheduleChronologicalPreserve(work *walk.DAG, windowStart time.Time, windowSize time.Duration) (*Result, error) {
	commits := work.All()
	sort.Slice(commits, func(i, j int) bool {
		a, b := commits[i], commits[j]
		if !a.AuthorDate.Equal(b.AuthorDate) {
			return a.AuthorDate.Before(b.AuthorDate)
		}
		return a.OID < b.OID
	})
	n := len(commits)

	// Even at one second per commit the timeline needs this much room.
	if minSpan := time.Duration(n-1) * time.Second; windowSize < minSpan {
		return nil, fmt.Errorf("cannot fit %d commits into the window even at one second apart; widen --start/--end (need at least %v)", n, minSpan)
	}

	tMin := commits[0].AuthorDate
	origSpan := commits[n-1].AuthorDate.Sub(tMin)

	// Compress (never expand) so the original span fits inside the window.
	scale := 1.0
	if origSpan > 0 && windowSize < origSpan {
		scale = float64(windowSize) / float64(origSpan)
	}

	newTimes := make(map[string]time.Time, n)
	var prev time.Time
	overflow := false
	for i, c := range commits {
		var nt time.Time
		if origSpan == 0 {
			nt = windowStart.Add(time.Duration(i) * time.Second)
		} else {
			nt = windowStart.Add(time.Duration(float64(c.AuthorDate.Sub(tMin)) * scale))
		}
		// Keep strictly increasing in chronological order so equal or
		// compression-collapsed timestamps stay distinct and ordered.
		if i > 0 && !nt.After(prev) {
			nt = prev.Add(time.Second)
		}
		newTimes[c.OID] = nt
		prev = nt
		if nt.Sub(windowStart) > windowSize {
			overflow = true
		}
	}

	// Pathological tight windows (dense author-date clusters whose one-second
	// nudges cumulatively overflow the window) fall back to even spacing: this
	// preserves the chronological ORDER exactly and is guaranteed to fit,
	// trading away proportional spacing only when it genuinely cannot fit.
	if overflow && n > 1 {
		step := windowSize / time.Duration(n-1)
		for i, c := range commits {
			newTimes[c.OID] = windowStart.Add(time.Duration(i) * step)
		}
		if origSpan > 0 {
			scale = float64(windowSize) / float64(origSpan)
		}
	}

	return &Result{NewTimes: newTimes, Scale: scale, DAG: work}, nil
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
		// score, then earlier parent date, then OID — the final OID tiebreak
		// keeps the choice stable regardless of map-iteration order.
		switch {
		case c.score != best.score:
			if c.score < best.score {
				best = c
			}
		case !c.pDate.Equal(best.pDate):
			if c.pDate.Before(best.pDate) {
				best = c
			}
		case c.parent < best.parent:
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
	// Keep IsMerge consistent with the rewired parent count: a survivor that
	// inherited a merge parent's multiple parents is itself now a merge.
	survivor.IsMerge = len(survivor.Parents) > 1
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
	// Iterate in a stable OID order. d.All() walks the underlying map in random
	// order, so without this the branch point chosen for collapsing — and thus
	// the set of surviving commits — would vary run to run for the same --seed.
	all := d.All()
	sort.Slice(all, func(i, j int) bool { return all[i].OID < all[j].OID })
	for _, p := range all {
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
