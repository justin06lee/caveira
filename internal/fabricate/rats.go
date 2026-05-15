package fabricate

import (
	"errors"
	"fmt"
	"math/rand"
	"strings"

	"github.com/go-git/go-git/v5"
)

const offBranchForkProb = 0.30

// BuildRatsPlan produces a Plan for rats mode. In this initial version each
// feature gets its own branch off master, branches merge in feature order.
// Tasks 12 and 13 add emergent off-branch forking and conflict scars.
func BuildRatsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildRatsPlan: at least one identity required")
	}

	files, err := WalkHead(repo)
	if err != nil {
		return nil, err
	}
	chore, features := GroupFiles(files)

	var commits []SynthCommit
	refs := map[string]int{}

	// Chore commit on master.
	chairman := ids[0]
	commits = append(commits, SynthCommit{
		ID:        0,
		Author:    chairman,
		Committer: chairman,
		Message:   ChoreMessage(rng),
		Added:     chore,
	})
	masterTip := 0

	// Phase 1: create every feature branch first, so that each branch is
	// "open" (started but not yet merged) while later branches are built.
	// This lets a later branch fork off an earlier still-open branch,
	// producing the criss-crossing emergent topology.
	type featureBranch struct {
		rat        Identity
		branchName string
		tip        int
	}
	branches := make([]featureBranch, 0, len(features))
	var openBranchTips []int
	for fi, feat := range features {
		rat := ids[fi%len(ids)]
		branchName := fmt.Sprintf("refs/heads/feat/%s", basenameDir(feat.Dir))

		forkParent := pickForkParent(masterTip, openBranchTips, rng)

		var branchTip int
		if len(feat.Code) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{forkParent},
				Author:    rat,
				Committer: rat,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
			})
			branchTip = cid
		} else {
			branchTip = forkParent
		}

		if len(feat.Test) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{branchTip},
				Author:    rat,
				Committer: rat,
				Message:   TestMessage(feat.Dir, rng),
				Added:     feat.Test,
			})
			branchTip = cid
		}

		// Track this branch as "open" for later features to potentially fork off.
		openBranchTips = append(openBranchTips, branchTip)

		refs[branchName] = branchTip
		branches = append(branches, featureBranch{rat: rat, branchName: branchName, tip: branchTip})
	}

	// Phase 2: merge each branch into master in feature order. Once a branch
	// is merged it is no longer "open" (though phase 1 is already done, this
	// keeps the bookkeeping honest for future tasks).
	for _, b := range branches {
		mergeID := len(commits)
		feat := strings.TrimPrefix(b.branchName, "refs/heads/feat/")
		commits = append(commits, SynthCommit{
			ID:        mergeID,
			Parents:   []int{masterTip, b.tip},
			Author:    b.rat,
			Committer: b.rat,
			Message:   fmt.Sprintf("Merge branch 'feat/%s' into master", feat),
			IsMerge:   true,
		})
		masterTip = mergeID

		newOpen := openBranchTips[:0]
		for _, t := range openBranchTips {
			if t != b.tip {
				newOpen = append(newOpen, t)
			}
		}
		openBranchTips = newOpen
	}

	refs[defaultBranch] = masterTip
	plan := &Plan{
		Commits: commits,
		Refs:    refs,
		HEAD:    masterTip,
		HeadRef: defaultBranch,
	}
	return plan, nil
}

// pickForkParent returns the parent commit ID for a new feature branch's first
// commit. With probability offBranchForkProb (when at least one open branch
// exists), it picks an open branch's current tip; otherwise master's tip.
func pickForkParent(masterTip int, openBranchTips []int, rng *rand.Rand) int {
	if len(openBranchTips) > 0 && rng.Float64() < offBranchForkProb {
		return openBranchTips[rng.Intn(len(openBranchTips))]
	}
	return masterTip
}
