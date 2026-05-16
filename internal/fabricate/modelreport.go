package fabricate

import (
	"sort"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// coAuthorPrefix is the case-insensitive git trailer key for co-author lines.
const coAuthorPrefix = "co-authored-by:"

// parseCoAuthors extracts the identities named in "Co-Authored-By: Name <email>"
// trailer lines anywhere in a commit message. Lines that are not valid
// identities are skipped.
func parseCoAuthors(message string) []Identity {
	var out []Identity
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < len(coAuthorPrefix) {
			continue
		}
		if !strings.EqualFold(trimmed[:len(coAuthorPrefix)], coAuthorPrefix) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(coAuthorPrefix):])
		id, err := ParseIdentity(rest)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}

// PlayerProfile captures how much one human author used AI coding models.
type PlayerProfile struct {
	// Rate is the fraction of the player's commits that had at least one model.
	Rate float64
	// Mix maps a lowercased model email to the fraction of the player's
	// model-assisted commits in which that model appeared.
	Mix map[string]float64
}

// ModelReport is the result of scanning a repo for AI-model usage.
type ModelReport struct {
	// Models is the set of distinct model identities found anywhere in history,
	// sorted by lowercased email.
	Models []Identity
	// Profiles maps a lowercased human-author email to that player's profile.
	Profiles map[string]PlayerProfile
}

// ScanModelReport walks every reachable commit in repo and builds a ModelReport:
// the set of AI models present in history, and a per-human-author profile of
// how much each used those models (as committers or Co-Authored-By trailers).
func ScanModelReport(repo *git.Repository) (*ModelReport, error) {
	type acc struct {
		total      int
		withModel  int
		modelCount map[string]int // lowercased model email -> count
	}
	players := map[string]*acc{}
	modelsByEmail := map[string]Identity{}

	lc := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	noteModel := func(id Identity) {
		if IsModel(id) {
			modelsByEmail[lc(id.Email)] = id
		}
	}

	refs, err := repo.References()
	if err != nil {
		return nil, err
	}
	var heads []plumbing.Hash
	err = refs.ForEach(func(r *plumbing.Reference) error {
		if r.Type() != plumbing.HashReference {
			return nil
		}
		obj, err := repo.Object(plumbing.AnyObject, r.Hash())
		if err != nil {
			return nil
		}
		switch o := obj.(type) {
		case *object.Commit:
			heads = append(heads, o.Hash)
		case *object.Tag:
			if c, err := o.Commit(); err == nil {
				heads = append(heads, c.Hash)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	visited := map[plumbing.Hash]bool{}
	for _, h := range heads {
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		stack := []*object.Commit{c}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[cur.Hash] {
				continue
			}
			visited[cur.Hash] = true

			author := Identity{Name: cur.Author.Name, Email: cur.Author.Email}
			committer := Identity{Name: cur.Committer.Name, Email: cur.Committer.Email}
			coAuthors := parseCoAuthors(cur.Message)

			noteModel(author)
			noteModel(committer)
			for _, ca := range coAuthors {
				noteModel(ca)
			}

			if !IsModel(author) && lc(author.Email) != "" {
				key := lc(author.Email)
				a := players[key]
				if a == nil {
					a = &acc{modelCount: map[string]int{}}
					players[key] = a
				}
				a.total++
				onCommit := map[string]bool{}
				if IsModel(committer) {
					onCommit[lc(committer.Email)] = true
				}
				for _, ca := range coAuthors {
					if IsModel(ca) {
						onCommit[lc(ca.Email)] = true
					}
				}
				if len(onCommit) > 0 {
					a.withModel++
					for email := range onCommit {
						a.modelCount[email]++
					}
				}
			}

			_ = cur.Parents().ForEach(func(p *object.Commit) error {
				stack = append(stack, p)
				return nil
			})
		}
	}

	report := &ModelReport{Profiles: map[string]PlayerProfile{}}
	for _, m := range modelsByEmail {
		report.Models = append(report.Models, m)
	}
	sort.SliceStable(report.Models, func(i, j int) bool {
		return lc(report.Models[i].Email) < lc(report.Models[j].Email)
	})
	for key, a := range players {
		if a.total == 0 {
			continue
		}
		prof := PlayerProfile{
			Rate: float64(a.withModel) / float64(a.total),
			Mix:  map[string]float64{},
		}
		sum := 0
		for _, n := range a.modelCount {
			sum += n
		}
		if sum > 0 {
			for email, n := range a.modelCount {
				prof.Mix[email] = float64(n) / float64(sum)
			}
		}
		report.Profiles[key] = prof
	}
	return report, nil
}
