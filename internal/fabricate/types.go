package fabricate

import (
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
)

// Identity is one person's git identity.
type Identity struct {
	Name  string
	Email string
}

// FileRef describes a single file's content set by a SynthCommit.
// If Content is non-nil the file is synthetic: Blob is the precomputed
// content hash and Content holds the bytes to write into the dst repo.
// If Content is nil the file is copied from the source repo blob Blob.
type FileRef struct {
	Path    string
	Blob    plumbing.Hash
	Mode    filemode.FileMode
	Content []byte
}

// DiffStat is an explicit per-commit edit-volume override. When a SynthCommit
// sets Stats, PlanToDAG uses it instead of counting bytes in Added (used by
// layered LLM commits whose Added holds cumulative, not delta, content).
type DiffStat struct {
	Lines    int
	Files    int
	NewFiles int
}

// SynthCommit is a single fabricated commit's metadata + payload.
type SynthCommit struct {
	ID        int
	Parents   []int
	Author    Identity
	Committer Identity
	Message   string
	// Added holds files this commit creates or updates.
	Added   []FileRef
	IsMerge bool
	Feature string    // feature/scope name; "" for chore or non-feature commits
	Stats   *DiffStat // optional explicit edit-volume; nil = count from Added
}

// Plan is the full fabricated history.
type Plan struct {
	Commits []SynthCommit
	Refs    map[string]int
	HEAD    int
	HeadRef string
}

// SyntheticOID converts an int ID to the string OID used in walk.DAG.
func SyntheticOID(id int) string {
	return fmt.Sprintf("synth-%d", id)
}
