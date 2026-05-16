package fabricate

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
)

// Identity is one person's git identity.
type Identity struct {
	Name  string
	Email string
}

// FileRef describes a single file's content set by a SynthCommit. It always
// refers to a blob in the source repo, identified by its content hash Blob.
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
	// Added holds files this commit creates or updates.
	Added   []FileRef
	IsMerge bool
	Feature string // feature/scope name; "" for chore or non-feature commits
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

// parseSyntheticOID parses "synth-N" into the integer N.
func parseSyntheticOID(oid string) (int, error) {
	const prefix = "synth-"
	if !strings.HasPrefix(oid, prefix) {
		return 0, fmt.Errorf("not a synthetic OID")
	}
	n, err := strconv.Atoi(strings.TrimPrefix(oid, prefix))
	if err != nil {
		return 0, err
	}
	return n, nil
}
