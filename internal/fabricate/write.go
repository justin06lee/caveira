package fabricate

import (
	"fmt"
	"io"
	"sort"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// WriteToRepo writes the Plan to dst:
//   - Copies needed blobs from src to dst (idempotent).
//   - For each commit in topological order, computes its tree state
//     (cumulative files from this commit and all ancestors) and writes the
//     tree object.
//   - Writes each commit object with the supplied times[oid] and rewritten
//     parent hashes (via the old->new map being built).
//   - Creates refs from plan.Refs and HEAD from plan.HeadRef.
//
// Returns the old (synthetic OID) -> new (real plumbing.Hash) mapping.
func WriteToRepo(src, dst *git.Repository, plan *Plan, times map[string]time.Time) (map[string]plumbing.Hash, error) {
	byID := make(map[int]*SynthCommit, len(plan.Commits))
	for i := range plan.Commits {
		byID[plan.Commits[i].ID] = &plan.Commits[i]
	}
	order, err := topoOrder(plan)
	if err != nil {
		return nil, err
	}

	// Copy source blobs into dst.
	seenBlobs := map[plumbing.Hash]bool{}
	for _, c := range plan.Commits {
		for _, fr := range c.Added {
			if seenBlobs[fr.Blob] {
				continue
			}
			seenBlobs[fr.Blob] = true
			if err := copyBlob(src, dst, fr.Blob); err != nil {
				return nil, fmt.Errorf("copy blob %s: %w", fr.Blob, err)
			}
		}
	}

	// Compute cumulative file state per commit ID.
	stateByID := map[int]map[string]FileRef{}
	for _, id := range order {
		sc := byID[id]
		var state map[string]FileRef
		switch len(sc.Parents) {
		case 0:
			state = map[string]FileRef{}
		case 1:
			state = cloneState(stateByID[sc.Parents[0]])
		default:
			state = map[string]FileRef{}
			for _, pid := range sc.Parents {
				for p, f := range stateByID[pid] {
					state[p] = f
				}
			}
		}
		for _, fr := range sc.Added {
			state[fr.Path] = fr
		}
		stateByID[id] = state
	}

	mapping := map[string]plumbing.Hash{}
	for _, id := range order {
		sc := byID[id]
		treeHash, err := writeTreeFromState(dst, stateByID[id])
		if err != nil {
			return nil, fmt.Errorf("write tree for commit %d: %w", id, err)
		}
		when := times[SyntheticOID(id)]
		if when.IsZero() {
			return nil, fmt.Errorf("no scheduled time for commit %d", id)
		}
		var parents []plumbing.Hash
		for _, p := range sc.Parents {
			parents = append(parents, mapping[SyntheticOID(p)])
		}
		commit := &object.Commit{
			Author: object.Signature{
				Name:  sc.Author.Name,
				Email: sc.Author.Email,
				When:  when,
			},
			Committer: object.Signature{
				Name:  sc.Committer.Name,
				Email: sc.Committer.Email,
				When:  when,
			},
			Message:      sc.Message,
			TreeHash:     treeHash,
			ParentHashes: parents,
		}
		ne := dst.Storer.NewEncodedObject()
		if err := commit.Encode(ne); err != nil {
			return nil, err
		}
		newHash, err := dst.Storer.SetEncodedObject(ne)
		if err != nil {
			return nil, err
		}
		mapping[SyntheticOID(id)] = newHash
	}

	// Drop every pre-existing ref. Duplicate copied the source repo's
	// branches, tags and remote-tracking refs verbatim; without removing
	// them the original commits stay reachable and the fabricated history
	// would be grafted alongside the real one instead of replacing it.
	existingRefs, err := dst.References()
	if err != nil {
		return nil, err
	}
	_ = existingRefs.ForEach(func(r *plumbing.Reference) error {
		if r.Name() == plumbing.HEAD {
			return nil
		}
		return dst.Storer.RemoveReference(r.Name())
	})

	for refName, commitID := range plan.Refs {
		if err := dst.Storer.SetReference(plumbing.NewHashReference(plumbing.ReferenceName(refName), mapping[SyntheticOID(commitID)])); err != nil {
			return nil, err
		}
	}
	if plan.HeadRef != "" {
		if err := dst.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, plumbing.ReferenceName(plan.HeadRef))); err != nil {
			return nil, err
		}
	}

	return mapping, nil
}

func cloneState(s map[string]FileRef) map[string]FileRef {
	out := make(map[string]FileRef, len(s))
	for k, v := range s {
		out[k] = v
	}
	return out
}

// writeTreeFromState builds nested tree objects from the path->FileRef state
// and returns the root tree hash.
func writeTreeFromState(dst *git.Repository, state map[string]FileRef) (plumbing.Hash, error) {
	paths := make([]string, 0, len(state))
	for p := range state {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return buildNestedTree(dst, paths, state, "")
}

// buildNestedTree builds a tree object for the subtree rooted at prefix.
func buildNestedTree(dst *git.Repository, paths []string, state map[string]FileRef, prefix string) (plumbing.Hash, error) {
	type entryRef struct {
		Name string
		Mode filemode.FileMode
		Hash plumbing.Hash
	}
	directChildren := map[string]bool{}
	subdirs := map[string][]string{}

	pre := prefix
	if pre != "" {
		pre += "/"
	}

	for _, p := range paths {
		if pre != "" && len(p) <= len(pre) {
			continue
		}
		rel := p
		if pre != "" {
			rel = p[len(pre):]
		}
		idx := indexOf(rel, '/')
		if idx == -1 {
			directChildren[rel] = true
		} else {
			subdir := rel[:idx]
			subdirs[subdir] = append(subdirs[subdir], p)
		}
	}

	var entries []entryRef
	for name := range directChildren {
		full := pre + name
		fr := state[full]
		entries = append(entries, entryRef{Name: name, Mode: fr.Mode, Hash: fr.Blob})
	}
	for name, kidPaths := range subdirs {
		subHash, err := buildNestedTree(dst, kidPaths, state, pre+name)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, entryRef{Name: name, Mode: filemode.Dir, Hash: subHash})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })

	tree := &object.Tree{}
	for _, e := range entries {
		tree.Entries = append(tree.Entries, object.TreeEntry{
			Name: e.Name,
			Mode: e.Mode,
			Hash: e.Hash,
		})
	}
	ne := dst.Storer.NewEncodedObject()
	if err := tree.Encode(ne); err != nil {
		return plumbing.ZeroHash, err
	}
	return dst.Storer.SetEncodedObject(ne)
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}

func copyBlob(src, dst *git.Repository, h plumbing.Hash) error {
	obj, err := src.Storer.EncodedObject(plumbing.BlobObject, h)
	if err != nil {
		return err
	}
	ne := dst.Storer.NewEncodedObject()
	ne.SetType(obj.Type())
	w, err := ne.Writer()
	if err != nil {
		return err
	}
	r, err := obj.Reader()
	if err != nil {
		return err
	}
	defer r.Close()
	// io.Copy propagates a real read error instead of silently truncating the
	// blob, and we close the writer so storers that flush on Close stay correct.
	if _, err := io.Copy(w, r); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	_, err = dst.Storer.SetEncodedObject(ne)
	return err
}

// topoOrder returns commit IDs in topological order (parents before children).
func topoOrder(plan *Plan) ([]int, error) {
	inDegree := make(map[int]int, len(plan.Commits))
	children := make(map[int][]int, len(plan.Commits))
	for _, c := range plan.Commits {
		if _, ok := inDegree[c.ID]; !ok {
			inDegree[c.ID] = 0
		}
		for _, p := range c.Parents {
			inDegree[c.ID]++
			children[p] = append(children[p], c.ID)
		}
	}
	var ready []int
	for id, deg := range inDegree {
		if deg == 0 {
			ready = append(ready, id)
		}
	}
	sort.Ints(ready)
	var order []int
	for len(ready) > 0 {
		head := ready[0]
		ready = ready[1:]
		order = append(order, head)
		for _, child := range children[head] {
			inDegree[child]--
			if inDegree[child] == 0 {
				ready = append(ready, child)
				sort.Ints(ready)
			}
		}
	}
	if len(order) != len(plan.Commits) {
		return nil, fmt.Errorf("cycle detected in plan: produced %d/%d", len(order), len(plan.Commits))
	}
	return order, nil
}
