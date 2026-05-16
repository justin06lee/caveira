package fabricate

import (
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/justin06lee/caveira/internal/walk"
)

// PlanToDAG converts a Plan into a walk.DAG. Each SynthCommit becomes a
// walk.Commit with OID = SyntheticOID(id). Diff stats are computed from the
// Added FileRefs by reading the source repo's blob sizes.
func PlanToDAG(srcRepo *git.Repository, plan *Plan) (*walk.DAG, error) {
	dag := walk.NewDAG()
	for _, sc := range plan.Commits {
		parents := make([]string, 0, len(sc.Parents))
		for _, p := range sc.Parents {
			parents = append(parents, SyntheticOID(p))
		}
		lines, files, newFiles := 0, 0, 0
		for _, fr := range sc.Added {
			blob, err := srcRepo.BlobObject(fr.Blob)
			if err != nil {
				return nil, err
			}
			lines += countBlobLines(blob)
			files++
			newFiles++
		}
		dag.Add(&walk.Commit{
			OID:          SyntheticOID(sc.ID),
			Parents:      parents,
			Author:       walk.Person{Name: sc.Author.Name, Email: sc.Author.Email},
			Committer:    walk.Person{Name: sc.Committer.Name, Email: sc.Committer.Email},
			Message:      sc.Message,
			AuthorDate:   time.Time{}, // synthetic, the scheduler assigns timestamps later
			IsMerge:      sc.IsMerge,
			IsRoot:       len(sc.Parents) == 0,
			LinesChanged: lines,
			FilesTouched: files,
			NewFiles:     newFiles,
		})
	}
	return dag, nil
}

func countBlobLines(blob *object.Blob) int {
	r, err := blob.Reader()
	if err != nil {
		return 0
	}
	defer r.Close()
	buf := make([]byte, 4096)
	count := 0
	hadContent := false
	for {
		n, err := r.Read(buf)
		for i := 0; i < n; i++ {
			if buf[i] == '\n' {
				count++
			}
			hadContent = true
		}
		if err != nil {
			break
		}
	}
	if hadContent && count == 0 {
		count = 1
	}
	return count
}
