package repo

import (
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
)

// IsProtectedBranch returns true if name (full ref or short) refers to
// "main" or "master".
func IsProtectedBranch(name string) bool {
	short := name
	if strings.HasPrefix(name, "refs/heads/") {
		short = strings.TrimPrefix(name, "refs/heads/")
	}
	return short == "main" || short == "master"
}

// Push force-pushes (with lease) all branches and tags to origin.
// Refuses to touch main/master unless allowProtected is true.
func Push(repoPath string, allowProtected bool) error {
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		return err
	}

	refs, err := repo.References()
	if err != nil {
		return err
	}
	if !allowProtected {
		var blocked []string
		_ = refs.ForEach(func(r *plumbing.Reference) error {
			if r.Type() == plumbing.HashReference && IsProtectedBranch(string(r.Name())) {
				blocked = append(blocked, string(r.Name()))
			}
			return nil
		})
		if len(blocked) > 0 {
			return fmt.Errorf("refusing to push protected branches without --push-protected: %v", blocked)
		}
	}

	// Push all branches with force-with-lease semantics.
	if err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Force:      true,
		RefSpecs:   []config.RefSpec{"+refs/heads/*:refs/heads/*"},
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("push branches: %w", err)
	}
	if err := repo.Push(&git.PushOptions{
		RemoteName: "origin",
		Force:      true,
		RefSpecs:   []config.RefSpec{"+refs/tags/*:refs/tags/*"},
	}); err != nil && err != git.NoErrAlreadyUpToDate {
		return fmt.Errorf("push tags: %w", err)
	}
	return nil
}
