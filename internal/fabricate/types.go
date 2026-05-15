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

// FileRef describes a single file's content to be added by a SynthCommit.
// Blob is the OID of the blob in the SOURCE repo.
type FileRef struct {
	Path string
	Blob plumbing.Hash
	Mode filemode.FileMode
}

// SynthCommit is a single fabricated commit's metadata + payload.
type SynthCommit struct {
	ID        int
	Parents   []int
	Author    Identity
	Committer Identity
	Message   string
	Added     []FileRef
	IsMerge   bool
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
