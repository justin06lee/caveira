package fabricate

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// identityRe matches "Name <email>" with at least one '@' inside the angle brackets.
var identityRe = regexp.MustCompile(`^\s*(?P<name>.+?)\s*<\s*(?P<email>[^<>\s]+@[^<>\s]+)\s*>\s*$`)

// ParseIdentity parses a "Name <email>" string into an Identity.
func ParseIdentity(s string) (Identity, error) {
	m := identityRe.FindStringSubmatch(s)
	if m == nil {
		return Identity{}, fmt.Errorf("invalid identity %q: expected `Name <email>`", s)
	}
	name := strings.TrimSpace(m[identityRe.SubexpIndex("name")])
	email := strings.TrimSpace(m[identityRe.SubexpIndex("email")])
	if name == "" {
		return Identity{}, fmt.Errorf("invalid identity %q: name is empty", s)
	}
	return Identity{Name: name, Email: email}, nil
}

// DiscoveredIdentity is an Identity plus how many commits attributed to it
// (used for the picker UI in Task 4).
type DiscoveredIdentity struct {
	Identity
	Commits int
}

// DiscoverIdentities scans every reachable commit in repo and returns the
// unique author identities (keyed by lowercased email), sorted by commit count
// descending then by name ascending.
func DiscoverIdentities(repo *git.Repository) ([]DiscoveredIdentity, error) {
	visited := map[plumbing.Hash]bool{}
	counts := map[string]*DiscoveredIdentity{}

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

			key := strings.ToLower(strings.TrimSpace(cur.Author.Email))
			if key == "" {
				continue
			}
			d, ok := counts[key]
			if !ok {
				d = &DiscoveredIdentity{
					Identity: Identity{Name: cur.Author.Name, Email: cur.Author.Email},
				}
				counts[key] = d
			}
			d.Commits++

			_ = cur.Parents().ForEach(func(p *object.Commit) error {
				stack = append(stack, p)
				return nil
			})
		}
	}

	out := make([]DiscoveredIdentity, 0, len(counts))
	for _, d := range counts {
		out = append(out, *d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
