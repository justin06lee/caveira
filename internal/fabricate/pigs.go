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

// BuildPigsPlan produces a Plan for pigs mode from the flurry base sequence.
func BuildPigsPlan(repo *git.Repository, ids []Identity, weights []int, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildPigsPlan: at least one identity required")
	}
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}
	return reshapePigs(base, ids, weights, rng), nil
}

// reshapePigs reshapes a linear base sequence for pigs mode: randomly drawn
// authors across real commits, typos on every message, and ~noiseRate noise
// commits injected between adjacent real commits. The base sequence's commit
// IDs and parents are reassigned; callers need not pre-link them. It mutates
// the caller's base slice elements in place.
func reshapePigs(base []SynthCommit, ids []Identity, weights []int, rng *rand.Rand) *Plan {
	for i := range base {
		id := pickAuthor(ids, weights, rng)
		base[i].Author = id
		base[i].Committer = id
	}
	for i := range base {
		base[i].Message = ApplyTypos(base[i].Message, rng)
	}

	var out []SynthCommit
	base[0].ID = 0
	base[0].Parents = nil
	out = append(out, base[0])
	for i := 1; i < len(base); i++ {
		if rng.Float64() < noiseRate {
			noise := SynthCommit{
				Author:    pickAuthor(ids, weights, rng),
				Committer: pickAuthor(ids, weights, rng),
				Message:   ApplyTypos(noiseMessages[rng.Intn(len(noiseMessages))], rng),
			}
			noise.ID = len(out)
			noise.Parents = []int{out[len(out)-1].ID}
			out = append(out, noise)
		}
		base[i].ID = len(out)
		base[i].Parents = []int{out[len(out)-1].ID}
		out = append(out, base[i])
	}

	return &Plan{
		Commits: out,
		Refs:    map[string]int{defaultBranch: out[len(out)-1].ID},
		HEAD:    out[len(out)-1].ID,
		HeadRef: defaultBranch,
	}
}
