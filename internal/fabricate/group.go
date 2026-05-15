package fabricate

import (
	"path"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// Feature is a top-level directory grouping with its code and test files.
type Feature struct {
	Dir  string // e.g., "internal/walk", "." for root
	Code []FileRef
	Test []FileRef
}

// WalkHead returns all files in the repo's HEAD tree as FileRefs.
func WalkHead(repo *git.Repository) ([]FileRef, error) {
	head, err := repo.Head()
	if err != nil {
		return nil, err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return nil, err
	}
	tree, err := commit.Tree()
	if err != nil {
		return nil, err
	}

	var out []FileRef
	err = tree.Files().ForEach(func(f *object.File) error {
		out = append(out, FileRef{
			Path: f.Name,
			Blob: f.Blob.Hash,
			Mode: f.Mode,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// GroupFiles splits files into a chore set and per-feature groups.
// Returns chore files and a sorted slice of Features.
func GroupFiles(files []FileRef) ([]FileRef, []Feature) {
	var chore []FileRef
	byDir := map[string]*Feature{}

	for _, f := range files {
		switch Classify(f.Path) {
		case Chore:
			chore = append(chore, f)
		case Test:
			dir := featureDir(f.Path)
			feat, ok := byDir[dir]
			if !ok {
				feat = &Feature{Dir: dir}
				byDir[dir] = feat
			}
			feat.Test = append(feat.Test, f)
		case Code:
			dir := featureDir(f.Path)
			feat, ok := byDir[dir]
			if !ok {
				feat = &Feature{Dir: dir}
				byDir[dir] = feat
			}
			feat.Code = append(feat.Code, f)
		}
	}

	features := make([]Feature, 0, len(byDir))
	for _, f := range byDir {
		sort.SliceStable(f.Code, func(i, j int) bool { return f.Code[i].Path < f.Code[j].Path })
		sort.SliceStable(f.Test, func(i, j int) bool { return f.Test[i].Path < f.Test[j].Path })
		features = append(features, *f)
	}
	sort.SliceStable(features, func(i, j int) bool { return features[i].Dir < features[j].Dir })
	return chore, features
}

// featureDir returns the top-level directory of p, or "." if p is at root.
func featureDir(p string) string {
	clean := path.Clean(p)
	parts := strings.SplitN(clean, "/", 3)
	if len(parts) == 1 {
		return "."
	}
	if len(parts) == 2 {
		return parts[0]
	}
	return parts[0] + "/" + parts[1]
}
