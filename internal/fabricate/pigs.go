package fabricate

import (
	"errors"
	"math/rand"

	"github.com/go-git/go-git/v5"
)

const (
	noiseRate     = 0.15 // probability of a noise commit between any two real commits
	defaultBranch = "refs/heads/master"
)

var noiseMessages = []string{
	"wip", "fix", "fix typo", "revert", "more changes",
	"stuff", "todo", "wip2", "idk", "actually fix",
	"lint", "fmt",
}

// BuildPigsPlan produces a Plan for pigs mode: a linear chain of synthetic
// commits with authors round-robin'd through ids, noise commits sprinkled in,
// and every message run through ApplyTypos.
func BuildPigsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildPigsPlan: at least one identity required")
	}
	// Use the first identity as a placeholder for the base flurry sequence;
	// we overwrite authors below.
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}

	// Author round-robin across real commits.
	for i := range base {
		id := ids[i%len(ids)]
		base[i].Author = id
		base[i].Committer = id
	}

	// Apply typos to every message.
	for i := range base {
		base[i].Message = ApplyTypos(base[i].Message, rng)
	}

	// Inject noise commits between adjacent real commits.
	var out []SynthCommit
	out = append(out, base[0])
	for i := 1; i < len(base); i++ {
		if rng.Float64() < noiseRate {
			noise := SynthCommit{
				Author:    ids[rng.Intn(len(ids))],
				Committer: ids[rng.Intn(len(ids))],
				Message:   ApplyTypos(noiseMessages[rng.Intn(len(noiseMessages))], rng),
			}
			noise.ID = len(out)
			noise.Parents = []int{out[len(out)-1].ID}
			out = append(out, noise)
			base[i].ID = len(out)
			base[i].Parents = []int{noise.ID}
		} else {
			base[i].ID = len(out)
			base[i].Parents = []int{out[len(out)-1].ID}
		}
		out = append(out, base[i])
	}

	plan := &Plan{
		Commits: out,
		Refs:    map[string]int{defaultBranch: out[len(out)-1].ID},
		HEAD:    out[len(out)-1].ID,
		HeadRef: defaultBranch,
	}
	return plan, nil
}
