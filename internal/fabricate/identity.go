package fabricate

import (
	"bufio"
	"fmt"
	"io"
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
// (used for the picker UI).
type DiscoveredIdentity struct {
	Identity
	Commits int
}

// walkCommits visits every commit reachable from any ref in repo exactly once.
func walkCommits(repo *git.Repository, visit func(*object.Commit)) error {
	refs, err := repo.References()
	if err != nil {
		return err
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
		return err
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
			visit(cur)
			_ = cur.Parents().ForEach(func(p *object.Commit) error {
				stack = append(stack, p)
				return nil
			})
		}
	}
	return nil
}

// DiscoverIdentities scans every reachable commit in repo and returns the
// unique author identities (keyed by lowercased email, canonicalized through
// mm), sorted by commit count descending then by name ascending. Models are
// excluded. A nil mm applies no canonicalization.
func DiscoverIdentities(repo *git.Repository, mm *Mailmap) ([]DiscoveredIdentity, error) {
	counts := map[string]*DiscoveredIdentity{}

	err := walkCommits(repo, func(cur *object.Commit) {
		id := mm.Canonical(Identity{Name: cur.Author.Name, Email: cur.Author.Email})
		key := strings.ToLower(strings.TrimSpace(id.Email))
		if key == "" {
			return
		}
		d, ok := counts[key]
		if !ok {
			d = &DiscoveredIdentity{Identity: id}
			counts[key] = d
		}
		d.Commits++
	})
	if err != nil {
		return nil, err
	}

	out := make([]DiscoveredIdentity, 0, len(counts))
	for _, d := range counts {
		if IsModel(d.Identity) {
			continue // AI coding agents are never offered as players
		}
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

// ResolveIdentities returns exactly n identities, using:
//  1. flag-supplied identities first (as-is, in order)
//  2. then identities discovered in repo (excluding those already supplied)
//  3. then interactive prompts on stdin to fill any remaining slots
//
// If more identities are discovered than the remaining slots after flags, an
// interactive picker is shown to let the user choose which to use.
//
// When pick is true, the automatic discover/picker logic in step 2 is bypassed:
// resolution always runs an interactive curation step (curateIdentities over the
// discovered identities, where the user may select any subset including none),
// then prompts on stdin for any shortfall to reach n.
func ResolveIdentities(repo *git.Repository, flagIDs []string, n int, mm *Mailmap, pick bool, stdin io.Reader, stdout io.Writer) ([]Identity, error) {
	if n < 1 {
		return nil, fmt.Errorf("ResolveIdentities: n must be >= 1, got %d", n)
	}

	out := make([]Identity, 0, n)
	for _, s := range flagIDs {
		id, err := ParseIdentity(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if len(out) > n {
		return nil, fmt.Errorf("got %d --pig/--rat identities but only %d slots available", len(out), n)
	}
	if len(out) == n {
		return out, nil
	}

	remaining := n - len(out)
	supplied := map[string]bool{}
	for _, id := range out {
		supplied[strings.ToLower(id.Email)] = true
	}

	discovered, err := DiscoverIdentities(repo, mm)
	if err != nil {
		return nil, err
	}
	var fresh []DiscoveredIdentity
	for _, d := range discovered {
		if !supplied[strings.ToLower(d.Email)] {
			fresh = append(fresh, d)
		}
	}

	if pick {
		picked, err := curateIdentities(fresh, remaining, stdin, stdout, n, len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, picked...)
		if len(out) < n {
			prompted, err := promptIdentities(n-len(out), stdin, stdout)
			if err != nil {
				return nil, err
			}
			out = append(out, prompted...)
		}
		if len(out) != n {
			return nil, fmt.Errorf("resolver produced %d identities, expected %d", len(out), n)
		}
		return out, nil
	}

	switch {
	case len(fresh) == remaining:
		for _, d := range fresh {
			out = append(out, d.Identity)
		}
	case len(fresh) > remaining:
		picked, err := pickIdentities(fresh, remaining, stdin, stdout, n, len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, picked...)
	case len(fresh) < remaining:
		for _, d := range fresh {
			out = append(out, d.Identity)
		}
		need := remaining - len(fresh)
		prompted, err := promptIdentities(need, stdin, stdout)
		if err != nil {
			return nil, err
		}
		out = append(out, prompted...)
	}

	if len(out) != n {
		return nil, fmt.Errorf("resolver produced %d identities, expected %d", len(out), n)
	}
	return out, nil
}

// readLineUnbuffered reads a single line (up to and including '\n') from r one
// byte at a time, so it never consumes input past the newline. Returns the line
// without the trailing newline. A trailing io.EOF is returned alongside any
// partial line read before the stream ended.
func readLineUnbuffered(r io.Reader) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 1)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if buf[0] == '\n' {
				return sb.String(), nil
			}
			sb.WriteByte(buf[0])
		}
		if err != nil {
			return sb.String(), err
		}
	}
}

// curateIdentities shows found and reads a comma-separated selection of 0..max
// entries by 1-based index. An empty line selects none. Returns the chosen
// identities. Errors on out-of-range indices, duplicates, or more than max.
func curateIdentities(found []DiscoveredIdentity, max int, stdin io.Reader, stdout io.Writer, total, alreadyHave int) ([]Identity, error) {
	fmt.Fprintf(stdout, "Caveira needs %d identities. %d supplied via flag. Found %d in .git:\n", total, alreadyHave, len(found))
	for i, d := range found {
		fmt.Fprintf(stdout, "  [%d] %s <%s>     (%d commits)\n", i+1, d.Name, d.Email, d.Commits)
	}
	fmt.Fprintf(stdout, "Pick up to %d (comma-separated, e.g. `1,3`; empty to pick none): ", max)

	// Read exactly one line without buffering ahead: in the pick path the
	// caller may hand the same stdin to promptIdentities afterwards, and a
	// bufio.Reader would swallow that follow-up input.
	line, err := readLineUnbuffered(stdin)
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	parts := strings.Split(line, ",")
	if len(parts) > max {
		return nil, fmt.Errorf("picked %d but only %d slot(s) available", len(parts), max)
	}
	seen := map[int]bool{}
	out := make([]Identity, 0, len(parts))
	for _, p := range parts {
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx); err != nil {
			return nil, fmt.Errorf("invalid pick %q", p)
		}
		if idx < 1 || idx > len(found) {
			return nil, fmt.Errorf("pick %d out of range", idx)
		}
		if seen[idx] {
			return nil, fmt.Errorf("duplicate pick %d", idx)
		}
		seen[idx] = true
		out = append(out, found[idx-1].Identity)
	}
	return out, nil
}

func pickIdentities(found []DiscoveredIdentity, k int, stdin io.Reader, stdout io.Writer, total, alreadyHave int) ([]Identity, error) {
	fmt.Fprintf(stdout, "Caveira needs %d identities. %d supplied via flag. Found %d in .git:\n", total, alreadyHave, len(found))
	for i, d := range found {
		fmt.Fprintf(stdout, "  [%d] %s <%s>     (%d commits)\n", i+1, d.Name, d.Email, d.Commits)
	}
	fmt.Fprintf(stdout, "Pick %d (comma-separated, e.g. `1,3`): ", k)

	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("no selection provided")
	}
	parts := strings.Split(line, ",")
	if len(parts) != k {
		return nil, fmt.Errorf("expected %d picks, got %d", k, len(parts))
	}
	out := make([]Identity, 0, k)
	for _, p := range parts {
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx); err != nil {
			return nil, fmt.Errorf("invalid pick %q", p)
		}
		if idx < 1 || idx > len(found) {
			return nil, fmt.Errorf("pick %d out of range", idx)
		}
		out = append(out, found[idx-1].Identity)
	}
	return out, nil
}

func promptIdentities(k int, stdin io.Reader, stdout io.Writer) ([]Identity, error) {
	reader := bufio.NewReader(stdin)
	out := make([]Identity, 0, k)
	for i := 0; i < k; i++ {
		var name, email string
		for name == "" {
			fmt.Fprintf(stdout, "Identity %d — Name: ", i+1)
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}
			name = strings.TrimSpace(line)
			if name == "" && err == io.EOF {
				return nil, fmt.Errorf("stdin exhausted while prompting for identity name")
			}
		}
		for email == "" {
			fmt.Fprintf(stdout, "Identity %d — Email: ", i+1)
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}
			email = strings.TrimSpace(line)
			if email == "" && err == io.EOF {
				return nil, fmt.Errorf("stdin exhausted while prompting for identity email")
			}
			if email != "" && !strings.Contains(email, "@") {
				fmt.Fprintf(stdout, "Email needs an @\n")
				email = ""
			}
		}
		out = append(out, Identity{Name: name, Email: email})
	}
	return out, nil
}
