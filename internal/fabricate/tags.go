package fabricate

import (
	"fmt"
	"math/rand"
)

const (
	// tagReleaseInterval is the target number of mainline commits between
	// release tags, before per-release jitter is applied.
	tagReleaseInterval = 6
	tagReleaseJitter   = 2
	// tagPatchProb / tagMajorProb steer semver bumps; the remainder are minor
	// bumps. First release is always v0.1.0.
	tagPatchProb = 0.25
	tagMajorProb = 0.10
)

// releaseMessages are natural annotation templates; %s is the version string
// (e.g. "v1.2.0").
var releaseMessages = []string{
	"Release %s",
	"Release version %s",
	"%s",
	"Tag release %s",
	"Cut %s",
}

// generateReleaseTags places annotated semver release tags along the first-parent
// mainline of plan.HEAD and appends them to plan.Tags. Placement, version bumps,
// and message choice are drawn from rng, so a fixed seed yields a fixed tag set.
// A tagger is the tagged commit's author — the person who cut the release. Short
// histories (fewer than one release interval of mainline commits) get no tags.
// Returns the number of tags added.
func generateReleaseTags(plan *Plan, rng *rand.Rand) int {
	mainline := mainlineFromHead(plan)
	if len(mainline) < tagReleaseInterval {
		return 0
	}
	byID := make(map[int]*SynthCommit, len(plan.Commits))
	for i := range plan.Commits {
		byID[plan.Commits[i].ID] = &plan.Commits[i]
	}

	major, minor, patch := 0, 0, 0
	first := true
	counter := 0
	interval := nextInterval(rng)
	added := 0
	for _, id := range mainline {
		counter++
		if counter < interval {
			continue
		}
		if first {
			minor, first = 1, false
		} else {
			switch r := rng.Float64(); {
			case r < tagMajorProb:
				major, minor, patch = major+1, 0, 0
			case r < tagMajorProb+tagPatchProb:
				patch++
			default:
				minor, patch = minor+1, 0
			}
		}
		version := fmt.Sprintf("v%d.%d.%d", major, minor, patch)
		tagger := byID[id].Author
		msg := fmt.Sprintf(releaseMessages[rng.Intn(len(releaseMessages))], version)
		plan.Tags = append(plan.Tags, SynthTag{
			Name:     version,
			CommitID: id,
			Tagger:   tagger,
			Message:  msg,
		})
		added++
		counter = 0
		interval = nextInterval(rng)
	}
	return added
}

// nextInterval returns the next release spacing, jittered around
// tagReleaseInterval and clamped to at least 1.
func nextInterval(rng *rand.Rand) int {
	n := tagReleaseInterval + rng.Intn(tagReleaseJitter*2+1) - tagReleaseJitter
	if n < 1 {
		n = 1
	}
	return n
}

// mainlineFromHead returns the plan's first-parent chain from the root up to
// HEAD (oldest first). This is the release line: for pigs/single it is the whole
// linear history; for rats it is master's first-parent spine, skipping the
// merged-in feature-branch commits.
func mainlineFromHead(plan *Plan) []int {
	byID := make(map[int]*SynthCommit, len(plan.Commits))
	for i := range plan.Commits {
		byID[plan.Commits[i].ID] = &plan.Commits[i]
	}
	var chain []int
	id := plan.HEAD
	seen := map[int]bool{}
	for {
		sc := byID[id]
		if sc == nil || seen[id] {
			break
		}
		seen[id] = true
		chain = append(chain, id)
		if len(sc.Parents) == 0 {
			break
		}
		id = sc.Parents[0]
	}
	for i, j := 0, len(chain)-1; i < j; i, j = i+1, j-1 {
		chain[i], chain[j] = chain[j], chain[i]
	}
	return chain
}
