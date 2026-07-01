package fabricate

import (
	"fmt"
)

// SquashOp is one parent->child squash, by synthetic OID ("synth-N").
type SquashOp struct {
	Parent string
	Child  string
}

// SquashPlan replays squash operations onto plan, mutating it in place. Each
// SquashOp collapses a linear parent->child edge: the child survives and the
// parent is removed. The result is a Plan whose surviving commits, refs, HEAD
// and parent links are consistent and whose cumulative tree at every surviving
// commit is unchanged from before squashing.
//
// durations maps synthetic OID -> duration in minutes; it determines which of
// parent/child supplies the survivor's metadata. SquashPlan does NOT mutate the
// caller's durations map: it clones it internally.
//
// squashes must be supplied in the order the scheduler produced them: each op
// references nodes that still exist at that point in the replay, so an
// out-of-order slice can reference an already-removed commit.
func SquashPlan(plan *Plan, squashes []SquashOp, durations map[string]int) error {
	durs := make(map[string]int, len(durations))
	for k, v := range durations {
		durs[k] = v
	}

	for _, sq := range squashes {
		parentID, err := parseSyntheticOID(sq.Parent)
		if err != nil {
			return fmt.Errorf("squash parent %q: %w", sq.Parent, err)
		}
		childID, err := parseSyntheticOID(sq.Child)
		if err != nil {
			return fmt.Errorf("squash child %q: %w", sq.Child, err)
		}

		parent := findCommit(plan, parentID)
		if parent == nil {
			return fmt.Errorf("squash parent commit %d not found in plan", parentID)
		}
		child := findCommit(plan, childID)
		if child == nil {
			return fmt.Errorf("squash child commit %d not found in plan", childID)
		}

		// Survivor = child. Metadata comes from whichever of parent/child had
		// the larger duration (matching schedule.applySquash/copyMetadataFrom).
		if durs[sq.Parent] > durs[sq.Child] {
			child.Author = parent.Author
			child.Committer = parent.Committer
			child.Message = parent.Message
		}

		// Added: parent's entries first, child's second, so the child's wins
		// on any path collision (preserves the cumulative tree).
		merged := append(append([]FileRef{}, parent.Added...), child.Added...)
		child.Added = merged

		// Parents: child inherits the parent's parents. If the parent was a
		// root (no parents), the child therefore becomes a root too — SynthCommit
		// represents rootness purely as an empty Parents slice.
		child.Parents = append([]int(nil), parent.Parents...)

		// Remove the parent commit from the plan.
		removeCommit(plan, parentID)

		// Reparent / repoint: every reference to parentID becomes childID.
		for i := range plan.Commits {
			ps := plan.Commits[i].Parents
			for j := range ps {
				if ps[j] == parentID {
					ps[j] = childID
				}
			}
		}
		for ref, id := range plan.Refs {
			if id == parentID {
				plan.Refs[ref] = childID
			}
		}
		for i := range plan.Tags {
			if plan.Tags[i].CommitID == parentID {
				plan.Tags[i].CommitID = childID
			}
		}
		if plan.HEAD == parentID {
			plan.HEAD = childID
		}

		// Update durations so a later squash involving the survivor compares
		// against the merged duration (matching schedule.applySquash).
		merge := durs[sq.Parent]
		if durs[sq.Child] > merge {
			merge = durs[sq.Child]
		}
		durs[sq.Child] = merge
	}

	return nil
}

// findCommit returns a pointer to the SynthCommit with the given ID, or nil.
func findCommit(plan *Plan, id int) *SynthCommit {
	for i := range plan.Commits {
		if plan.Commits[i].ID == id {
			return &plan.Commits[i]
		}
	}
	return nil
}

// removeCommit deletes the SynthCommit with the given ID from plan.Commits.
func removeCommit(plan *Plan, id int) {
	for i := range plan.Commits {
		if plan.Commits[i].ID == id {
			plan.Commits = append(plan.Commits[:i], plan.Commits[i+1:]...)
			return
		}
	}
}
