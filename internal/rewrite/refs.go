package rewrite

import (
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// RebuildRefs deletes any existing refs on dst (other than HEAD) and creates
// new refs that mirror src's refs, retargeted via mapping. Annotated tags are
// rebuilt as new tag objects retargeted at the new commit.
func RebuildRefs(src, dst *git.Repository, mapping map[string]plumbing.Hash) error {
	// Delete existing dst refs except HEAD.
	dstRefs, err := dst.References()
	if err != nil {
		return err
	}
	_ = dstRefs.ForEach(func(r *plumbing.Reference) error {
		if r.Name() == plumbing.HEAD {
			return nil
		}
		return dst.Storer.RemoveReference(r.Name())
	})

	// Walk src refs and recreate on dst.
	srcRefs, err := src.References()
	if err != nil {
		return err
	}
	return srcRefs.ForEach(func(r *plumbing.Reference) error {
		if r.Type() != plumbing.HashReference {
			return nil
		}
		obj, err := src.Object(plumbing.AnyObject, r.Hash())
		if err != nil {
			return nil
		}
		switch o := obj.(type) {
		case *object.Commit:
			if newHash, ok := mapping[o.Hash.String()]; ok {
				return dst.Storer.SetReference(plumbing.NewHashReference(r.Name(), newHash))
			}
		case *object.Tag:
			targetCommit, err := o.Commit()
			if err != nil {
				return nil
			}
			newTarget, ok := mapping[targetCommit.Hash.String()]
			if !ok {
				return nil
			}
			// Retime the tagger date onto the target commit's new time so the
			// tag moves into the window with its commit instead of keeping its
			// original (now out-of-window) timestamp. The tagger's name/email
			// are preserved, mirroring how commit identities are kept.
			tagger := o.Tagger
			if nc, err := dst.CommitObject(newTarget); err == nil {
				tagger.When = nc.Committer.When
			}
			nt := &object.Tag{
				Name:       o.Name,
				Tagger:     tagger,
				Message:    o.Message,
				TargetType: plumbing.CommitObject,
				Target:     newTarget,
			}
			ne := dst.Storer.NewEncodedObject()
			if err := nt.Encode(ne); err != nil {
				return err
			}
			nh, err := dst.Storer.SetEncodedObject(ne)
			if err != nil {
				return err
			}
			return dst.Storer.SetReference(plumbing.NewHashReference(r.Name(), nh))
		}
		return nil
	})
}
