package walk

import (
	"fmt"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
)

// Load walks every ref of repo, collects all reachable commits, computes
// per-commit diff stats (insertions+deletions, files touched, new files),
// and returns the assembled DAG.
func Load(repo *git.Repository) (*DAG, error) {
	dag := NewDAG()

	visited := map[plumbing.Hash]bool{}
	var seeds []plumbing.Hash

	refs, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("refs: %w", err)
	}
	err = refs.ForEach(func(ref *plumbing.Reference) error {
		if ref.Type() != plumbing.HashReference {
			return nil
		}
		h := ref.Hash()
		obj, err := repo.Object(plumbing.AnyObject, h)
		if err != nil {
			return nil
		}
		switch o := obj.(type) {
		case *object.Commit:
			seeds = append(seeds, o.Hash)
		case *object.Tag:
			if t, err := o.Commit(); err == nil {
				seeds = append(seeds, t.Hash)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, seed := range seeds {
		c, err := repo.CommitObject(seed)
		if err != nil {
			continue
		}
		if err := walkFrom(repo, c, dag, visited); err != nil {
			return nil, err
		}
	}

	return dag, nil
}

func walkFrom(repo *git.Repository, start *object.Commit, dag *DAG, visited map[plumbing.Hash]bool) error {
	stack := []*object.Commit{start}
	for len(stack) > 0 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if visited[c.Hash] {
			continue
		}
		visited[c.Hash] = true

		parents := make([]string, 0, c.NumParents())
		_ = c.Parents().ForEach(func(p *object.Commit) error {
			parents = append(parents, p.Hash.String())
			stack = append(stack, p)
			return nil
		})

		lines, files, newFiles, err := diffStats(c)
		if err != nil {
			return fmt.Errorf("diff stats for %s: %w", c.Hash, err)
		}

		dag.Add(&Commit{
			OID:          c.Hash.String(),
			Parents:      parents,
			Author:       Person{Name: c.Author.Name, Email: c.Author.Email},
			Committer:    Person{Name: c.Committer.Name, Email: c.Committer.Email},
			Message:      c.Message,
			AuthorDate:   c.Author.When,
			IsMerge:      c.NumParents() > 1,
			IsRoot:       c.NumParents() == 0,
			LinesChanged: lines,
			FilesTouched: files,
			NewFiles:     newFiles,
		})
	}
	_ = storer.ErrStop // silence unused import in some setups
	return nil
}

func diffStats(c *object.Commit) (lines, files, newFiles int, err error) {
	if c.NumParents() > 1 {
		return 0, 0, 0, nil
	}

	tree, err := c.Tree()
	if err != nil {
		return 0, 0, 0, err
	}

	var parentTree *object.Tree
	if c.NumParents() == 1 {
		p, err := c.Parent(0)
		if err != nil {
			return 0, 0, 0, err
		}
		parentTree, err = p.Tree()
		if err != nil {
			return 0, 0, 0, err
		}
	}

	changes, err := object.DiffTree(parentTree, tree)
	if err != nil {
		return 0, 0, 0, err
	}

	seen := map[string]bool{}
	for _, ch := range changes {
		path := ch.To.Name
		if path == "" {
			path = ch.From.Name
		}
		if !seen[path] {
			files++
			seen[path] = true
		}
		fromExists := ch.From.Name != ""
		toExists := ch.To.Name != ""
		if toExists && !fromExists {
			newFiles++
		}
		patch, err := ch.Patch()
		if err != nil {
			return 0, 0, 0, err
		}
		for _, fp := range patch.FilePatches() {
			for _, chk := range fp.Chunks() {
				switch chk.Type() {
				case 1, 2: // Add, Delete (diff.Add=1, diff.Delete=2 in go-git)
					lines += countLines(chk.Content())
				}
			}
		}
	}
	return lines, files, newFiles, nil
}

func countLines(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	if len(s) > 0 && s[len(s)-1] != '\n' {
		n++
	}
	return n
}
