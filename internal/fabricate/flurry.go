package fabricate

import (
	"math/rand"

	"github.com/go-git/go-git/v5"
)

// FlurrySequence returns the base sequence of synthetic commits (chore, then
// per-feature code + test) with the supplied identity as both author and
// committer. Parents form a linear chain (each commit's parent is the prior).
func FlurrySequence(repo *git.Repository, id Identity, rng *rand.Rand) ([]SynthCommit, error) {
	files, err := WalkHead(repo)
	if err != nil {
		return nil, err
	}
	chore, features := GroupFiles(files)

	var commits []SynthCommit
	idx := 0

	commits = append(commits, SynthCommit{
		ID:        idx,
		Author:    id,
		Committer: id,
		Message:   ChoreMessage(rng),
		Added:     chore,
	})
	idx++

	prev := commits[0].ID
	for _, feat := range features {
		if len(feat.Code) > 0 {
			c := SynthCommit{
				ID:        idx,
				Parents:   []int{prev},
				Author:    id,
				Committer: id,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
				Feature:   basenameDir(feat.Dir),
			}
			commits = append(commits, c)
			prev = idx
			idx++
		}
		if len(feat.Test) > 0 {
			c := SynthCommit{
				ID:        idx,
				Parents:   []int{prev},
				Author:    id,
				Committer: id,
				Message:   TestMessage(feat.Dir, rng),
				Added:     feat.Test,
				Feature:   basenameDir(feat.Dir),
			}
			commits = append(commits, c)
			prev = idx
			idx++
		}
	}

	return commits, nil
}
