package rewrite

import (
	"fmt"
	"io"

	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/justin06lee/caveira/internal/schedule"
	"github.com/justin06lee/caveira/internal/walk"
)

// InMemoryClone returns a fresh in-memory go-git repository that has all the
// object database of src copied over but no refs. Used for testing Apply
// without touching disk. Production code clones via repo.Clone.
func InMemoryClone(src *git.Repository) (*git.Repository, error) {
	dst, err := git.Init(memory.NewStorage(), memfs.New())
	if err != nil {
		return nil, err
	}
	// Copy all objects from src to dst.
	iter, err := src.Storer.IterEncodedObjects(plumbing.AnyObject)
	if err != nil {
		return nil, err
	}
	if err := iter.ForEach(func(o plumbing.EncodedObject) error {
		ne := dst.Storer.NewEncodedObject()
		ne.SetType(o.Type())
		w, err := ne.Writer()
		if err != nil {
			return err
		}
		r, err := o.Reader()
		if err != nil {
			return err
		}
		// io.Copy surfaces a real read error instead of the hand-rolled loop's
		// break-on-any-error, which silently truncated objects on transient I/O
		// failures and wrote a corrupt copy.
		if _, err := io.Copy(w, r); err != nil {
			return err
		}
		if err := w.Close(); err != nil {
			return err
		}
		_, err = dst.Storer.SetEncodedObject(ne)
		return err
	}); err != nil {
		return nil, err
	}
	return dst, nil
}

// Apply writes new commit objects to dst according to the schedule result.
// It rewrites parents to point at new OIDs, preserves tree hashes (taking
// the child's tree on a squash), and sets new author/committer timestamps.
func Apply(src, dst *git.Repository, dag *walk.DAG, res *schedule.Result) (map[string]plumbing.Hash, error) {
	// Apply needs the scheduled times; a nil result has nothing to apply.
	if res == nil {
		return nil, fmt.Errorf("rewrite.Apply: nil schedule result")
	}
	// Prefer the post-squash DAG from Schedule if available; otherwise fall
	// back to the caller-supplied DAG (e.g. when no scheduling was performed).
	if res.DAG != nil {
		dag = res.DAG
	}
	order, err := dag.TopologicalOrder()
	if err != nil {
		return nil, err
	}

	oldToNew := map[string]plumbing.Hash{}

	for _, oid := range order {
		c := dag.Get(oid)
		oldHash := plumbing.NewHash(oid)
		oldCommit, err := src.CommitObject(oldHash)
		if err != nil {
			return nil, fmt.Errorf("read old commit %s: %w", oid, err)
		}
		newTime, ok := res.NewTimes[oid]
		if !ok {
			return nil, fmt.Errorf("no scheduled time for %s", oid)
		}
		// Keep the scheduled instant in the window's timezone (the zone the
		// user chose for --start/--end). Re-projecting into each commit's
		// ORIGINAL zone would scatter offsets across the rewritten history
		// (e.g. a source spanning a DST boundary mixes -08:00 and -07:00),
		// making git log render the commits with inconsistent, out-of-order
		// local times even though the underlying instants are correct.
		newTimeInTZ := newTime

		var newParents []plumbing.Hash
		for _, p := range c.Parents {
			nh, ok := oldToNew[p]
			if !ok {
				return nil, fmt.Errorf("parent %s of %s has no new hash", p, oid)
			}
			newParents = append(newParents, nh)
		}

		newCommit := &object.Commit{
			Author: object.Signature{
				Name:  oldCommit.Author.Name,
				Email: oldCommit.Author.Email,
				When:  newTimeInTZ,
			},
			Committer: object.Signature{
				Name:  oldCommit.Committer.Name,
				Email: oldCommit.Committer.Email,
				When:  newTimeInTZ,
			},
			Message:      oldCommit.Message,
			TreeHash:     oldCommit.TreeHash,
			ParentHashes: newParents,
		}
		ne := dst.Storer.NewEncodedObject()
		if err := newCommit.Encode(ne); err != nil {
			return nil, err
		}
		nh, err := dst.Storer.SetEncodedObject(ne)
		if err != nil {
			return nil, err
		}
		oldToNew[oid] = nh
	}

	// Replace HEAD with the new tip (use the last commit in topo order as a
	// reasonable default; refs are rebuilt in Task 16).
	if len(order) > 0 {
		last := oldToNew[order[len(order)-1]]
		headRef := plumbing.NewBranchReferenceName("master")
		if _, err := src.Reference(plumbing.NewBranchReferenceName("main"), false); err == nil {
			headRef = plumbing.NewBranchReferenceName("main")
		}
		if err := dst.Storer.SetReference(plumbing.NewHashReference(headRef, last)); err != nil {
			return nil, err
		}
		if err := dst.Storer.SetReference(plumbing.NewSymbolicReference(plumbing.HEAD, headRef)); err != nil {
			return nil, err
		}
	}

	return oldToNew, nil
}
