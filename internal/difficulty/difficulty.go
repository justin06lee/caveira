package difficulty

import "math/rand"

// Difficulty is a coarse rating of a commit's expected duration.
type Difficulty int

const (
	Trivial Difficulty = iota
	Easy
	Medium
	Hard
	Substantial
)

func (d Difficulty) String() string {
	switch d {
	case Trivial:
		return "trivial"
	case Easy:
		return "easy"
	case Medium:
		return "medium"
	case Hard:
		return "hard"
	case Substantial:
		return "substantial"
	}
	return "unknown"
}

// Base returns the unscaled mean duration in minutes for this difficulty.
func (d Difficulty) Base() int {
	switch d {
	case Trivial:
		return 5
	case Easy:
		return 15
	case Medium:
		return 30
	case Hard:
		return 60
	case Substantial:
		return 90
	}
	return 0
}

// Deviation returns the unscaled half-width of the uniform draw in minutes.
func (d Difficulty) Deviation() int {
	switch d {
	case Trivial:
		return 3
	case Easy:
		return 7
	case Medium:
		return 13
	case Hard:
		return 17
	case Substantial:
		return 23
	}
	return 0
}

// Score combines diff-stat signals into a single integer.
func Score(linesChanged, filesTouched, newFiles int) int {
	return linesChanged + filesTouched*10 + newFiles*25
}

// Bucket maps a score to a Difficulty. Merge commits are forced to Trivial.
func Bucket(score int, isMerge bool) Difficulty {
	if isMerge {
		return Trivial
	}
	switch {
	case score <= 10:
		return Trivial
	case score <= 50:
		return Easy
	case score <= 200:
		return Medium
	case score <= 600:
		return Hard
	default:
		return Substantial
	}
}

// Draw returns a duration in minutes for the given difficulty, sampling
// uniformly from [base-deviation, base+deviation] (inclusive on both ends).
func Draw(d Difficulty, rng *rand.Rand) int {
	dev := d.Deviation()
	// uniform_int_inclusive(-dev, +dev)
	offset := rng.Intn(2*dev+1) - dev
	return d.Base() + offset
}
