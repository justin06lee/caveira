package fabricate

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/go-git/go-git/v5"
)

const offBranchForkProb = 0.30

const (
	conflictFixProb       = 0.20 // probability of a conflict-fix scar after a merge
	conflictFixBranchProb = 0.40 // probability the scar is a fix-branch (given a scar)
)

// BuildRatsPlan produces a Plan for rats mode from the flurry base sequence.
func BuildRatsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildRatsPlan: at least one identity required")
	}
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}
	return reshapeRats(base, ids, rng)
}

// featureRun is a contiguous run of base commits sharing one Feature.
type featureRun struct {
	feature string
	commits []SynthCommit
}

// splitBase partitions a linear base sequence into a leading chore run (commits
// with Feature == "") and one featureRun per contiguous same-Feature run.
func splitBase(base []SynthCommit) (chore []SynthCommit, runs []featureRun) {
	for _, c := range base {
		if c.Feature == "" {
			if len(runs) == 0 {
				chore = append(chore, c)
				continue
			}
		}
		if len(runs) > 0 && runs[len(runs)-1].feature == c.Feature && c.Feature != "" {
			runs[len(runs)-1].commits = append(runs[len(runs)-1].commits, c)
			continue
		}
		if c.Feature == "" {
			// A non-feature commit after features begin: attach to the last run.
			runs[len(runs)-1].commits = append(runs[len(runs)-1].commits, c)
			continue
		}
		runs = append(runs, featureRun{feature: c.Feature, commits: []SynthCommit{c}})
	}
	return chore, runs
}

// reshapeRats reshapes a linear base sequence into the rats emergent topology:
// each featureRun becomes a branch (forking from master or another open
// branch), branches merge back into master, and merges may leave conflict-fix
// scars. Commit IDs and parents are reassigned by this function.
func reshapeRats(base []SynthCommit, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(base) == 0 {
		return nil, errors.New("reshapeRats: empty base sequence")
	}
	choreCommits, runs := splitBase(base)

	var commits []SynthCommit
	refs := map[string]int{}
	next := func() int { return len(commits) }

	// Chore commit(s) on master.
	masterTip := -1
	for _, cc := range choreCommits {
		id := next()
		cc.ID = id
		cc.Author = ids[0]
		cc.Committer = ids[0]
		if masterTip >= 0 {
			cc.Parents = []int{masterTip}
		} else {
			cc.Parents = nil
		}
		commits = append(commits, cc)
		masterTip = id
	}
	if masterTip < 0 {
		// No chore commit: synthesize an empty root so feature branches have a base.
		commits = append(commits, SynthCommit{
			ID: 0, Author: ids[0], Committer: ids[0], Message: "chore: initial commit",
		})
		masterTip = 0
	}

	// Phase 1: build every feature branch.
	type branch struct {
		rat        Identity
		branchName string
		tip        int
	}
	var branches []branch
	var openBranchTips []int
	for fi, run := range runs {
		rat := ids[fi%len(ids)]
		branchName := fmt.Sprintf("refs/heads/feat/%s", run.feature)
		parent := pickForkParent(masterTip, openBranchTips, rng)
		tip := parent
		for _, rc := range run.commits {
			id := next()
			rc.ID = id
			rc.Author = rat
			rc.Committer = rat
			rc.Parents = []int{tip}
			commits = append(commits, rc)
			tip = id
		}
		openBranchTips = append(openBranchTips, tip)
		refs[branchName] = tip
		branches = append(branches, branch{rat: rat, branchName: branchName, tip: tip})
	}

	// Phase 2: merge each branch into master, with optional conflict-fix scars.
	for _, b := range branches {
		mergeID := next()
		commits = append(commits, SynthCommit{
			ID:        mergeID,
			Parents:   []int{masterTip, b.tip},
			Author:    b.rat,
			Committer: b.rat,
			Message:   fmt.Sprintf("Merge branch '%s' into master", trimRefsHeads(b.branchName)),
			IsMerge:   true,
		})
		masterTip = mergeID

		if rng.Float64() < conflictFixProb {
			feat := b.branchName[len("refs/heads/feat/"):]
			if rng.Float64() < conflictFixBranchProb {
				fixID := next()
				commits = append(commits, SynthCommit{
					ID: fixID, Parents: []int{masterTip}, Author: b.rat, Committer: b.rat,
					Message: fmt.Sprintf("fix: resolve conflict in %s", feat),
				})
				refs[fmt.Sprintf("refs/heads/fix/%s", feat)] = fixID
				mergeFixID := next()
				commits = append(commits, SynthCommit{
					ID: mergeFixID, Parents: []int{masterTip, fixID}, Author: b.rat,
					Committer: b.rat, IsMerge: true,
					Message: fmt.Sprintf("Merge branch 'fix/%s' into master", feat),
				})
				masterTip = mergeFixID
			} else {
				fixID := next()
				commits = append(commits, SynthCommit{
					ID: fixID, Parents: []int{masterTip}, Author: b.rat, Committer: b.rat,
					Message: fmt.Sprintf("fix: resolve conflict in %s", feat),
				})
				masterTip = fixID
			}
		}
	}

	refs[defaultBranch] = masterTip
	return &Plan{Commits: commits, Refs: refs, HEAD: masterTip, HeadRef: defaultBranch}, nil
}

func trimRefsHeads(ref string) string {
	const p = "refs/heads/"
	if len(ref) > len(p) && ref[:len(p)] == p {
		return ref[len(p):]
	}
	return ref
}

// pickForkParent returns the parent commit ID for a new feature branch's first
// commit: an open branch tip with probability offBranchForkProb, else master.
func pickForkParent(masterTip int, openBranchTips []int, rng *rand.Rand) int {
	if len(openBranchTips) > 0 && rng.Float64() < offBranchForkProb {
		return openBranchTips[rng.Intn(len(openBranchTips))]
	}
	return masterTip
}
