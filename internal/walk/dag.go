package walk

import (
	"fmt"
	"sort"
	"time"
)

// Commit is the in-memory representation of a source commit, plus the diff
// stats computed in Task 7. It does not hold tree contents.
type Commit struct {
	OID          string
	Parents      []string
	Author       Person
	Committer    Person
	Message      string
	AuthorDate   time.Time
	IsMerge      bool
	IsRoot       bool
	Signed       bool
	LinesChanged int
	FilesTouched int
	NewFiles     int
}

// Person is a git identity.
type Person struct {
	Name  string
	Email string
}

// DAG holds all reachable commits keyed by OID.
type DAG struct {
	commits  map[string]*Commit
	children map[string][]string
}

// NewDAG returns an empty DAG.
func NewDAG() *DAG {
	return &DAG{
		commits:  map[string]*Commit{},
		children: map[string][]string{},
	}
}

// Add inserts a commit. Idempotent.
func (d *DAG) Add(c *Commit) {
	if _, ok := d.commits[c.OID]; ok {
		return
	}
	d.commits[c.OID] = c
	for _, p := range c.Parents {
		d.children[p] = append(d.children[p], c.OID)
	}
}

// Get returns the commit with the given OID, or nil.
func (d *DAG) Get(oid string) *Commit {
	return d.commits[oid]
}

// All returns commits in unspecified order.
func (d *DAG) All() []*Commit {
	out := make([]*Commit, 0, len(d.commits))
	for _, c := range d.commits {
		out = append(out, c)
	}
	return out
}

// Children returns the OIDs of commits whose parents include oid.
func (d *DAG) Children(oid string) []string {
	return append([]string(nil), d.children[oid]...)
}

// TopologicalOrder returns OIDs sorted so that every parent precedes its
// children. Ties are broken by AuthorDate then OID for determinism.
func (d *DAG) TopologicalOrder() ([]string, error) {
	inDegree := map[string]int{}
	for oid, c := range d.commits {
		if _, ok := inDegree[oid]; !ok {
			inDegree[oid] = 0
		}
		for _, p := range c.Parents {
			if _, exists := d.commits[p]; exists {
				inDegree[oid]++
			}
		}
	}

	var ready []string
	for oid, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, oid)
		}
	}
	sortByAuthorDate(ready, d.commits)

	var order []string
	for len(ready) > 0 {
		head := ready[0]
		ready = ready[1:]
		order = append(order, head)
		for _, child := range d.children[head] {
			inDegree[child]--
			if inDegree[child] == 0 {
				ready = append(ready, child)
				sortByAuthorDate(ready, d.commits)
			}
		}
	}

	if len(order) != len(d.commits) {
		return nil, fmt.Errorf("cycle detected: produced %d/%d in topo order", len(order), len(d.commits))
	}
	return order, nil
}

func sortByAuthorDate(oids []string, commits map[string]*Commit) {
	sort.SliceStable(oids, func(i, j int) bool {
		a, b := commits[oids[i]], commits[oids[j]]
		if !a.AuthorDate.Equal(b.AuthorDate) {
			return a.AuthorDate.Before(b.AuthorDate)
		}
		return oids[i] < oids[j]
	})
}

// Remove deletes a commit and removes any references to it from other
// commits' parent lists. Used by the scheduler when squashing.
func (d *DAG) Remove(oid string) {
	delete(d.commits, oid)
	delete(d.children, oid)
	for _, c := range d.commits {
		var np []string
		for _, p := range c.Parents {
			if p != oid {
				np = append(np, p)
			}
		}
		c.Parents = np
	}
	// rebuild children index
	d.children = map[string][]string{}
	for _, c := range d.commits {
		for _, p := range c.Parents {
			d.children[p] = append(d.children[p], c.OID)
		}
	}
}
