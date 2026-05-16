package fabricate

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// mmEntry is one commit-email -> canonical mapping from a .mailmap line.
type mmEntry struct {
	commitName  string // lowercased; "" matches any commit name
	properName  string // "" = no name override
	properEmail string // canonical email
}

// Mailmap canonicalizes git identities per a repository's .mailmap file.
// The zero value and a nil *Mailmap are valid passthrough (no-op) maps.
type Mailmap struct {
	byCommitEmail map[string][]mmEntry // lowercased commit email -> entries
	nameByEmail   map[string]string    // lowercased email -> canonical name
}

var mailmapAngleRe = regexp.MustCompile(`<([^<>]*)>`)

// ParseMailmap parses .mailmap content into a Mailmap. It handles the four
// standard line forms and `#` comments; unparseable lines are skipped.
func ParseMailmap(content []byte) *Mailmap {
	mm := &Mailmap{
		byCommitEmail: map[string][]mmEntry{},
		nameByEmail:   map[string]string{},
	}
	for _, raw := range strings.Split(string(content), "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		locs := mailmapAngleRe.FindAllStringSubmatchIndex(line, -1)
		switch {
		case len(locs) == 1:
			// Form 1: Proper Name <proper@email>
			loc := locs[0]
			email := strings.TrimSpace(line[loc[2]:loc[3]])
			name := strings.TrimSpace(line[:loc[0]])
			if email != "" && name != "" {
				mm.nameByEmail[strings.ToLower(email)] = name
			}
		case len(locs) >= 2:
			// Forms 2/3/4: [Proper Name] <proper@email> [Commit Name] <commit@email>
			l1, l2 := locs[0], locs[1]
			properEmail := strings.TrimSpace(line[l1[2]:l1[3]])
			commitEmail := strings.TrimSpace(line[l2[2]:l2[3]])
			properName := strings.TrimSpace(line[:l1[0]])
			commitName := strings.TrimSpace(line[l1[1]:l2[0]])
			if properEmail == "" || commitEmail == "" {
				continue
			}
			key := strings.ToLower(commitEmail)
			mm.byCommitEmail[key] = append(mm.byCommitEmail[key], mmEntry{
				commitName:  strings.ToLower(commitName),
				properName:  properName,
				properEmail: properEmail,
			})
			if properName != "" {
				mm.nameByEmail[strings.ToLower(properEmail)] = properName
			}
		}
	}
	return mm
}

// Canonical returns the canonical identity for id per the mailmap. A nil
// *Mailmap returns id unchanged.
func (mm *Mailmap) Canonical(id Identity) Identity {
	if mm == nil {
		return id
	}
	lcEmail := strings.ToLower(strings.TrimSpace(id.Email))
	lcName := strings.ToLower(strings.TrimSpace(id.Name))

	if entries := mm.byCommitEmail[lcEmail]; len(entries) > 0 {
		var best *mmEntry
		for i := range entries {
			if entries[i].commitName != "" && entries[i].commitName == lcName {
				best = &entries[i]
				break
			}
		}
		if best == nil {
			for i := range entries {
				if entries[i].commitName == "" {
					best = &entries[i]
					break
				}
			}
		}
		if best != nil {
			name := best.properName
			if name == "" {
				if n := mm.nameByEmail[strings.ToLower(best.properEmail)]; n != "" {
					name = n
				} else {
					name = id.Name
				}
			}
			return Identity{Name: name, Email: best.properEmail}
		}
	}
	if n := mm.nameByEmail[lcEmail]; n != "" {
		return Identity{Name: n, Email: id.Email}
	}
	return id
}

// LoadMailmap reads and parses <repoPath>/.mailmap. An absent file yields a
// nil *Mailmap (a valid passthrough); a read error other than not-exist is
// returned.
func LoadMailmap(repoPath string) (*Mailmap, error) {
	content, err := os.ReadFile(filepath.Join(repoPath, ".mailmap"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseMailmap(content), nil
}
