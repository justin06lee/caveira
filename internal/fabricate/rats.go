package fabricate

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/go-git/go-git/v5"
)

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

	for fi, feat := range features {
		rat := ids[fi%len(ids)]
		branchName := fmt.Sprintf("refs/heads/feat/%s", basenameDir(feat.Dir))

		// Code commit on branch (parent = current master tip)
		var branchTip int
		if len(feat.Code) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{masterTip},
				Author:    rat,
				Committer: rat,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
			})
			branchTip = cid
		} else {
			branchTip = masterTip
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

		// Record branch ref at its tip.
		refs[branchName] = branchTip

		// Merge commit on master.
		mergeID := len(commits)
		commits = append(commits, SynthCommit{
			ID:        mergeID,
			Parents:   []int{masterTip, branchTip},
			Author:    rat,
			Committer: rat,
			Message:   fmt.Sprintf("Merge branch 'feat/%s' into master", basenameDir(feat.Dir)),
			IsMerge:   true,
		})
		masterTip = mergeID
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
