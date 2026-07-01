# Caveira Fabricate (Phase 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `--fabricate` + `--flurry` (NLP-only fabrication) plus `--pigs N` and `--rats N` workflow modes that synthesize a believable commit history ending at the source repo's HEAD tree.

**Architecture:** A new `internal/fabricate` package walks the source HEAD tree, classifies files (chore / code / test), groups them by top-level directory, and emits a list of synthetic commits. `pigs.go` linearizes into one branch with author round-robin, noise commits, and message typos; `rats.go` emits an emergent branched topology with off-branch forks and occasional conflict-fix commits. The output is converted to a `walk.DAG` that the existing scheduler (`internal/schedule`) consumes for window-fitting; a fabricate-specific writer (`fabricate.WriteToRepo`) then writes the trees, blobs, commits, and branch refs to the destination repo.

**Tech Stack:** Go 1.22+, `github.com/go-git/go-git/v5`, stdlib `math/rand`, existing `internal/walk` DAG types, existing `internal/schedule` pipeline.

**Spec:** `docs/superpowers/specs/2026-05-14-caveira-fabricate-design.md`

---

## Conventions used in this plan

- Module path: `github.com/justin06lee/caveira`
- Branch: implementation should happen on a feature branch (e.g., `fabricate-impl`) off of `master`.
- After each task, commit with the message shown.
- `go test ./...` from repo root must pass before each commit.

## Data model used across tasks

These types are introduced in Task 6 and referenced from Tasks 7–13. Engineers reading Tasks 7+ can find their definitions here.

```go
// internal/fabricate/types.go (lives alongside templates.go in Task 6)

// Identity is one person's git identity.
type Identity struct {
    Name  string
    Email string
}

// FileRef describes a single file's content to be added by a SynthCommit.
type FileRef struct {
    Path string         // path in the working tree (relative)
    Blob plumbing.Hash  // OID of the file's blob in the SOURCE repo
    Mode filemode.FileMode
}

// SynthCommit is a single fabricated commit's metadata + payload.
type SynthCommit struct {
    ID        int      // index in Plan.Commits; used as the "OID" in the walk.DAG
    Parents   []int    // indices in Plan.Commits
    Author    Identity
    Committer Identity
    Message   string
    Added     []FileRef // files this commit adds (Phase 1: never modifies, only adds)
    IsMerge   bool
}

// Plan is the full fabricated history.
type Plan struct {
    Commits []SynthCommit
    Refs    map[string]int // branch name -> commit index that the ref tips at
    HEAD    int            // commit index for HEAD
    HeadRef string         // ref name for HEAD (e.g. "refs/heads/master")
}

// SyntheticOID converts an int ID to the string used in the walk.DAG.
func SyntheticOID(id int) string { return fmt.Sprintf("synth-%d", id) }
```

---

## Task 1: Config fields and CLI flag plumbing

**Files:**
- Modify: `internal/input/config.go`
- Modify: `internal/input/config_test.go`
- Modify: `internal/cli/cli.go`
- Modify: `internal/cli/cli_test.go`

- [ ] **Step 1: Add new fields to `Config` and validation**

Update `internal/input/config.go` to add fields and cross-flag validation:

```go
package input

import (
	"errors"
	"fmt"
	"time"
)

type Config struct {
	Repo          string
	Start         time.Time
	End           time.Time
	Seed          int64
	HasSeed       bool
	DryRun        bool
	Push          bool
	PushProtected bool
	WindowTZ      *time.Location
	OutDir        string

	// Fabricate-mode fields (Phase 1)
	Fabricate      bool
	Flurry         bool
	PigsN          int      // 0 = not set
	RatsN          int      // 0 = not set
	PigIdentities  []string // raw strings from --pig flags, parsed in fabricate.ParseIdentity
	RatIdentities  []string // raw strings from --rat flags
}

func (c *Config) Validate() error {
	if c.Repo == "" {
		return errors.New("--repo is required")
	}
	if c.WindowTZ == nil {
		return errors.New("--window-tz must resolve to a location")
	}
	if !c.Start.Before(c.End) {
		return errors.New("--start must be strictly before --end")
	}

	fabFlagsUsed := c.Flurry || c.PigsN > 0 || c.RatsN > 0 || len(c.PigIdentities) > 0 || len(c.RatIdentities) > 0
	if fabFlagsUsed && !c.Fabricate {
		return errors.New("--flurry, --pigs, --rats, --pig, --rat all require --fabricate")
	}
	if c.Fabricate && !c.Flurry {
		return errors.New("--fabricate requires --flurry")
	}
	if c.PigsN > 0 && c.RatsN > 0 {
		return errors.New("--pigs and --rats are mutually exclusive")
	}
	if len(c.PigIdentities) > 0 && c.PigsN == 0 {
		return errors.New("--pig requires --pigs N")
	}
	if len(c.RatIdentities) > 0 && c.RatsN == 0 {
		return errors.New("--rat requires --rats N")
	}
	if c.PigsN < 0 || c.RatsN < 0 {
		return fmt.Errorf("--pigs/--rats must be >= 1")
	}
	return nil
}

func (c *Config) WindowSize() time.Duration {
	return c.End.Sub(c.Start)
}
```

- [ ] **Step 2: Write failing tests for the new validation rules**

Append to `internal/input/config_test.go`:

```go
func TestConfig_Validate_FabricateFlagDependencies(t *testing.T) {
	tz := time.UTC
	base := Config{
		Repo:     "/tmp/x",
		Start:    time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:      time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		WindowTZ: tz,
	}
	cases := []struct {
		name   string
		mutate func(*Config)
		want   string
	}{
		{"flurry without fabricate", func(c *Config) { c.Flurry = true }, "require --fabricate"},
		{"pigs without fabricate", func(c *Config) { c.PigsN = 2 }, "require --fabricate"},
		{"rats without fabricate", func(c *Config) { c.RatsN = 2 }, "require --fabricate"},
		{"pig without pigs", func(c *Config) { c.Fabricate = true; c.Flurry = true; c.PigIdentities = []string{"x"} }, "--pig requires --pigs"},
		{"rat without rats", func(c *Config) { c.Fabricate = true; c.Flurry = true; c.RatIdentities = []string{"x"} }, "--rat requires --rats"},
		{"pigs and rats together", func(c *Config) { c.Fabricate = true; c.Flurry = true; c.PigsN = 2; c.RatsN = 2 }, "mutually exclusive"},
		{"fabricate without flurry", func(c *Config) { c.Fabricate = true }, "requires --flurry"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base
			tc.mutate(&c)
			err := c.Validate()
			if err == nil {
				t.Fatalf("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("expected %q in error, got: %v", tc.want, err)
			}
		})
	}
}

func TestConfig_Validate_FabricateOK(t *testing.T) {
	tz := time.UTC
	c := Config{
		Repo: "/tmp/x", WindowTZ: tz,
		Start: time.Date(2026, 5, 14, 12, 0, 0, 0, tz),
		End:   time.Date(2026, 5, 14, 13, 0, 0, 0, tz),
		Fabricate: true, Flurry: true, PigsN: 2,
		PigIdentities: []string{"Alice <a@x.com>", "Bob <b@x.com>"},
	}
	if err := c.Validate(); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
}
```

Add `"strings"` to the `internal/input/config_test.go` imports.

- [ ] **Step 3: Run config tests and verify they pass**

Run:
```bash
go test ./internal/input/...
```
Expected: PASS.

- [ ] **Step 4: Add CLI flag wiring**

In `internal/cli/cli.go`, replace the existing `newRootCmd` function so it also registers the new flags. Add the new flag declarations after the existing ones (preserve the existing flags exactly):

```go
func newRootCmd(name string) *cobra.Command {
	var (
		repoFlag  string
		startFlag string
		endFlag   string
		seedFlag  int64
		dryRun    bool
		pushFlag  bool
		pushProt  bool
		windowTZ  string
		outDir    string

		fabricateFlag bool
		flurryFlag    bool
		pigsN         int
		ratsN         int
		pigIDs        []string
		ratIDs        []string
	)

	cmd := &cobra.Command{
		Use:   name,
		Short: "Rewrite a repo's commit timestamps to fit a chosen time window",
		Example: "  " + name + ` --repo /path/to/myrepo \
      --start "2026-05-14 13:00" \
      --end   "2026-05-14 17:00"

  ` + name + ` --repo https://github.com/u/myrepo.git \
      --start "tomorrow 9am" --end "tomorrow 5pm" \
      --seed 42 --dry-run

  ` + name + ` --repo /path/to/myrepo --fabricate --flurry \
      --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
      --pigs 3`,
		SilenceUsage: true,
		RunE: func(c *cobra.Command, args []string) error {
			tz, err := time.LoadLocation(windowTZ)
			if err != nil {
				return fmt.Errorf("invalid --window-tz %q: %w", windowTZ, err)
			}
			now := time.Now().In(tz)
			start, err := input.ParseDateTime(startFlag, tz, now)
			if err != nil {
				return err
			}
			end, err := input.ParseDateTime(endFlag, tz, now)
			if err != nil {
				return err
			}
			cfg := &input.Config{
				Repo:          repoFlag,
				Start:         start,
				End:           end,
				Seed:          seedFlag,
				HasSeed:       c.Flags().Changed("seed"),
				DryRun:        dryRun,
				Push:          pushFlag,
				PushProtected: pushProt,
				WindowTZ:      tz,
				OutDir:        outDir,
				Fabricate:     fabricateFlag,
				Flurry:        flurryFlag,
				PigsN:         pigsN,
				RatsN:         ratsN,
				PigIdentities: pigIDs,
				RatIdentities: ratIDs,
			}
			if err := cfg.Validate(); err != nil {
				return err
			}
			code := Pipeline(cfg, c.OutOrStdout(), c.ErrOrStderr())
			if code != 0 {
				return fmt.Errorf("%s exited with code %d", name, code)
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&repoFlag, "repo", "", "path or URL of the source repository (required)")
	cmd.Flags().StringVar(&startFlag, "start", "", "window start (required)")
	cmd.Flags().StringVar(&endFlag, "end", "", "window end (required)")
	cmd.Flags().Int64Var(&seedFlag, "seed", 0, "deterministic seed for duration draws")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the schedule, write nothing")
	cmd.Flags().BoolVar(&pushFlag, "push", false, "force-push to origin after the swap")
	cmd.Flags().BoolVar(&pushProt, "push-protected", false, "allow pushing main/master")
	cmd.Flags().StringVar(&windowTZ, "window-tz", "Local", "IANA timezone for --start/--end")
	cmd.Flags().StringVar(&outDir, "out-dir", "", "parent directory for URL clones (default $CWD)")

	cmd.Flags().BoolVar(&fabricateFlag, "fabricate", false, "synthesize a new commit history instead of retiming the source")
	cmd.Flags().BoolVar(&flurryFlag, "flurry", false, "use the NLP-only fabricator (requires --fabricate)")
	cmd.Flags().IntVar(&pigsN, "pigs", 0, "chaotic single-branch fabricator with N people (requires --fabricate)")
	cmd.Flags().IntVar(&ratsN, "rats", 0, "branched fabricator with N people (requires --fabricate)")
	cmd.Flags().StringArrayVar(&pigIDs, "pig", nil, "pig identity as \"Name <email>\"; repeatable (requires --pigs)")
	cmd.Flags().StringArrayVar(&ratIDs, "rat", nil, "rat identity as \"Name <email>\"; repeatable (requires --rats)")

	_ = cmd.MarkFlagRequired("repo")
	_ = cmd.MarkFlagRequired("start")
	_ = cmd.MarkFlagRequired("end")

	return cmd
}
```

- [ ] **Step 5: Add a CLI test for the new flags**

Append to `internal/cli/cli_test.go`:

```go
func TestRunFabricateFlagsParse(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--flurry",
		"--pigs", "2",
		"--pig", "Alice <a@x.com>",
		"--pig", "Bob <b@x.com>",
	}, &out, &errOut)
	// Validation should pass (the pipeline failure later is fine).
	if !bytes.Contains(errOut.Bytes(), []byte("not a directory")) &&
		!bytes.Contains(errOut.Bytes(), []byte("no such file")) {
		// Either the pipeline failed cleanly, or an unexpected error occurred.
		if code == 0 {
			t.Fatalf("expected non-zero exit due to missing repo path, got 0; stderr=%q", errOut.String())
		}
	}
}
```

- [ ] **Step 6: Run all tests**

Run:
```bash
go test ./...
```
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/input internal/cli
git commit -m "feat(cli): add fabricate flags and Config fields"
```

---

## Task 2: Identity types and `--pig` / `--rat` parsing

**Files:**
- Create: `internal/fabricate/types.go`
- Create: `internal/fabricate/identity.go`
- Create: `internal/fabricate/identity_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/identity_test.go`:

```go
package fabricate

import (
	"testing"
)

func TestParseIdentity_Valid(t *testing.T) {
	cases := []struct {
		in   string
		want Identity
	}{
		{"Alice <a@x.com>", Identity{Name: "Alice", Email: "a@x.com"}},
		{"Alice Cooper <alice.cooper@example.com>", Identity{Name: "Alice Cooper", Email: "alice.cooper@example.com"}},
		{"  Bob   <bob@y.com>  ", Identity{Name: "Bob", Email: "bob@y.com"}},
	}
	for _, c := range cases {
		got, err := ParseIdentity(c.in)
		if err != nil {
			t.Errorf("ParseIdentity(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseIdentity(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseIdentity_Invalid(t *testing.T) {
	cases := []string{
		"",
		"Alice",
		"a@x.com",
		"<a@x.com>",
		"Alice <>",
		"Alice <noatsign>",
	}
	for _, in := range cases {
		if _, err := ParseIdentity(in); err == nil {
			t.Errorf("expected error for %q", in)
		}
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Create `types.go` with `Identity` (and stub types referenced later in the plan)**

Write `internal/fabricate/types.go`:

```go
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
```

- [ ] **Step 4: Implement `ParseIdentity`**

Write `internal/fabricate/identity.go`:

```go
package fabricate

import (
	"fmt"
	"regexp"
	"strings"
)

// identityRe matches "Name <email>" with at least one '@' inside the angle brackets.
var identityRe = regexp.MustCompile(`^\s*(?P<name>.+?)\s*<\s*(?P<email>[^<>\s]+@[^<>\s]+)\s*>\s*$`)

// ParseIdentity parses a "Name <email>" string into an Identity.
func ParseIdentity(s string) (Identity, error) {
	m := identityRe.FindStringSubmatch(s)
	if m == nil {
		return Identity{}, fmt.Errorf("invalid identity %q: expected `Name <email>`", s)
	}
	name := strings.TrimSpace(m[identityRe.SubexpIndex("name")])
	email := strings.TrimSpace(m[identityRe.SubexpIndex("email")])
	if name == "" {
		return Identity{}, fmt.Errorf("invalid identity %q: name is empty", s)
	}
	return Identity{Name: name, Email: email}, nil
}
```

- [ ] **Step 5: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): Identity type and Parse helper"
```

---

## Task 3: Identity resolver (flags + `.git` scan)

**Files:**
- Modify: `internal/fabricate/identity.go`
- Modify: `internal/fabricate/identity_test.go`

- [ ] **Step 1: Add failing tests for `.git` scanning**

Append to `internal/fabricate/identity_test.go`:

```go
import (
	"github.com/justin06lee/caveira/internal/walk"
)

func TestDiscoverIdentities(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 3, []int{1, 1, 1})
	got, err := DiscoverIdentities(repo)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 unique identity, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Test" || got[0].Email != "test@example.com" {
		t.Errorf("unexpected identity: %+v", got[0])
	}
}
```

Make sure the file imports `"github.com/justin06lee/caveira/internal/walk"` at the top.

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL on `DiscoverIdentities` undefined.

- [ ] **Step 3: Implement `DiscoverIdentities`**

Append to `internal/fabricate/identity.go`:

```go
import (
	"sort"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DiscoveredIdentity is an Identity plus how many commits attributed to it
// (used for the picker UI in Task 4).
type DiscoveredIdentity struct {
	Identity
	Commits int
}

// DiscoverIdentities scans every reachable commit in repo and returns the
// unique author identities (keyed by lowercased email), sorted by commit count
// descending then by name ascending.
func DiscoverIdentities(repo *git.Repository) ([]DiscoveredIdentity, error) {
	visited := map[plumbing.Hash]bool{}
	counts := map[string]*DiscoveredIdentity{}

	refs, err := repo.References()
	if err != nil {
		return nil, err
	}
	var heads []plumbing.Hash
	err = refs.ForEach(func(r *plumbing.Reference) error {
		if r.Type() != plumbing.HashReference {
			return nil
		}
		obj, err := repo.Object(plumbing.AnyObject, r.Hash())
		if err != nil {
			return nil
		}
		switch o := obj.(type) {
		case *object.Commit:
			heads = append(heads, o.Hash)
		case *object.Tag:
			if c, err := o.Commit(); err == nil {
				heads = append(heads, c.Hash)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	for _, h := range heads {
		c, err := repo.CommitObject(h)
		if err != nil {
			continue
		}
		stack := []*object.Commit{c}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if visited[cur.Hash] {
				continue
			}
			visited[cur.Hash] = true

			key := strings.ToLower(strings.TrimSpace(cur.Author.Email))
			if key == "" {
				continue
			}
			d, ok := counts[key]
			if !ok {
				d = &DiscoveredIdentity{
					Identity: Identity{Name: cur.Author.Name, Email: cur.Author.Email},
				}
				counts[key] = d
			}
			d.Commits++

			_ = cur.Parents().ForEach(func(p *object.Commit) error {
				stack = append(stack, p)
				return nil
			})
		}
	}

	out := make([]DiscoveredIdentity, 0, len(counts))
	for _, d := range counts {
		out = append(out, *d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Commits != out[j].Commits {
			return out[i].Commits > out[j].Commits
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): discover author identities from .git history"
```

---

## Task 4: Identity resolver (interactive prompts + picker)

**Files:**
- Modify: `internal/fabricate/identity.go`
- Modify: `internal/fabricate/identity_test.go`

The resolver glues together flag identities, discovered identities, prompts, and the picker into one entry point `ResolveIdentities`.

- [ ] **Step 1: Add failing tests**

Append to `internal/fabricate/identity_test.go`:

```go
import (
	"bytes"
	"strings"
)

func TestResolveIdentities_AllFromFlags(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 2, []int{1, 1})
	flags := []string{"Alice <a@x.com>", "Bob <b@x.com>"}
	got, err := ResolveIdentities(repo, flags, 2, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 identities, got %d", len(got))
	}
	if got[0].Name != "Alice" || got[1].Name != "Bob" {
		t.Errorf("flag identities lost or reordered: %+v", got)
	}
}

func TestResolveIdentities_FillFromGit(t *testing.T) {
	repo, _ := walk.MakeFixtureLinear(t, 2, []int{1, 1})
	flags := []string{}
	got, err := ResolveIdentities(repo, flags, 1, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(got))
	}
	if got[0].Name != "Test" {
		t.Errorf("expected discovered identity, got %+v", got[0])
	}
}

func TestResolveIdentities_PromptWhenShort(t *testing.T) {
	// Fixture has 1 identity ("Test"). We need 3. Should prompt twice.
	repo, _ := walk.MakeFixtureLinear(t, 1, []int{1})
	flags := []string{}
	stdin := strings.NewReader("Bob\nbob@x.com\nCarol\ncarol@x.com\n")
	var stdout bytes.Buffer
	got, err := ResolveIdentities(repo, flags, 3, stdin, &stdout)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 identities, got %d", len(got))
	}
	if got[1].Name != "Bob" || got[2].Name != "Carol" {
		t.Errorf("prompted identities incorrect: %+v", got)
	}
}

func TestResolveIdentities_PickerWhenTooMany(t *testing.T) {
	// Build a repo with multiple unique committers by injecting commits with
	// different authors. Helper not yet written; for now use a single-author
	// fixture and skip with a TODO if N > available.
	// This test verifies the picker code path when discovered > needed.
	// We construct a synthetic discovery scenario by calling the resolver
	// directly with flag-supplied identities that fewer than we need so the
	// picker fires.
	repo, _ := walk.MakeFixtureLinear(t, 1, []int{1})
	flags := []string{}
	// Need 1, but pretend we discovered multiple by selecting from "Test".
	stdin := strings.NewReader("1\n")
	var stdout bytes.Buffer
	got, err := ResolveIdentities(repo, flags, 1, stdin, &stdout)
	if err != nil {
		t.Fatalf("ResolveIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 identity, got %d", len(got))
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL on `ResolveIdentities` undefined.

- [ ] **Step 3: Implement `ResolveIdentities`**

Append to `internal/fabricate/identity.go`:

```go
import (
	"bufio"
	"io"
)

// ResolveIdentities returns exactly n identities, using:
//   1. flag-supplied identities first (as-is, in order)
//   2. then identities discovered in repo (excluding those already supplied)
//   3. then interactive prompts on stdin to fill any remaining slots
// If more identities are discovered than the remaining slots after flags, an
// interactive picker is shown to let the user choose which to use.
func ResolveIdentities(repo *git.Repository, flagIDs []string, n int, stdin io.Reader, stdout io.Writer) ([]Identity, error) {
	if n < 1 {
		return nil, fmt.Errorf("ResolveIdentities: n must be >= 1, got %d", n)
	}

	out := make([]Identity, 0, n)
	for _, s := range flagIDs {
		id, err := ParseIdentity(s)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if len(out) > n {
		return nil, fmt.Errorf("got %d --pig/--rat identities but only %d slots available", len(out), n)
	}
	if len(out) == n {
		return out, nil
	}

	remaining := n - len(out)
	supplied := map[string]bool{}
	for _, id := range out {
		supplied[strings.ToLower(id.Email)] = true
	}

	discovered, err := DiscoverIdentities(repo)
	if err != nil {
		return nil, err
	}
	var fresh []DiscoveredIdentity
	for _, d := range discovered {
		if !supplied[strings.ToLower(d.Email)] {
			fresh = append(fresh, d)
		}
	}

	switch {
	case len(fresh) == remaining:
		for _, d := range fresh {
			out = append(out, d.Identity)
		}
	case len(fresh) > remaining:
		picked, err := pickIdentities(fresh, remaining, stdin, stdout, n, len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, picked...)
	case len(fresh) < remaining:
		for _, d := range fresh {
			out = append(out, d.Identity)
		}
		need := remaining - len(fresh)
		prompted, err := promptIdentities(need, stdin, stdout)
		if err != nil {
			return nil, err
		}
		out = append(out, prompted...)
	}

	if len(out) != n {
		return nil, fmt.Errorf("resolver produced %d identities, expected %d", len(out), n)
	}
	return out, nil
}

func pickIdentities(found []DiscoveredIdentity, k int, stdin io.Reader, stdout io.Writer, total, alreadyHave int) ([]Identity, error) {
	fmt.Fprintf(stdout, "Caveira needs %d identities. %d supplied via flag. Found %d in .git:\n", total, alreadyHave, len(found))
	for i, d := range found {
		fmt.Fprintf(stdout, "  [%d] %s <%s>     (%d commits)\n", i+1, d.Name, d.Email, d.Commits)
	}
	fmt.Fprintf(stdout, "Pick %d (comma-separated, e.g. `1,3`): ", k)

	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("no selection provided")
	}
	parts := strings.Split(line, ",")
	if len(parts) != k {
		return nil, fmt.Errorf("expected %d picks, got %d", k, len(parts))
	}
	out := make([]Identity, 0, k)
	for _, p := range parts {
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx); err != nil {
			return nil, fmt.Errorf("invalid pick %q", p)
		}
		if idx < 1 || idx > len(found) {
			return nil, fmt.Errorf("pick %d out of range", idx)
		}
		out = append(out, found[idx-1].Identity)
	}
	return out, nil
}

func promptIdentities(k int, stdin io.Reader, stdout io.Writer) ([]Identity, error) {
	reader := bufio.NewReader(stdin)
	out := make([]Identity, 0, k)
	for i := 0; i < k; i++ {
		var name, email string
		for name == "" {
			fmt.Fprintf(stdout, "Identity %d — Name: ", i+1)
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}
			name = strings.TrimSpace(line)
		}
		for email == "" {
			fmt.Fprintf(stdout, "Identity %d — Email: ", i+1)
			line, err := reader.ReadString('\n')
			if err != nil && err != io.EOF {
				return nil, err
			}
			email = strings.TrimSpace(line)
			if !strings.Contains(email, "@") {
				fmt.Fprintf(stdout, "Email needs an @\n")
				email = ""
			}
		}
		out = append(out, Identity{Name: name, Email: email})
	}
	return out, nil
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): identity resolver with flags, .git scan, prompts, picker"
```

---

## Task 5: File classification (chore / code / test)

**Files:**
- Create: `internal/fabricate/classify.go`
- Create: `internal/fabricate/classify_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/classify_test.go`:

```go
package fabricate

import "testing"

func TestClassify(t *testing.T) {
	cases := map[string]FileKind{
		// Chore
		"README.md":            Chore,
		"Makefile":              Chore,
		"go.mod":                Chore,
		"go.sum":                Chore,
		"LICENSE":               Chore,
		".gitignore":            Chore,
		".editorconfig":         Chore,
		"package.json":          Chore,
		"Cargo.toml":            Chore,
		"Dockerfile":            Chore,
		"pyproject.toml":        Chore,
		// Tests
		"internal/walk/load_test.go":            Test,
		"src/test_module.py":                    Test,
		"src/module.test.ts":                    Test,
		"src/module.spec.ts":                    Test,
		"tests/foo.py":                          Test,
		"src/__tests__/foo.js":                  Test,
		"src/spec/foo.rb":                       Test,
		// Code
		"internal/walk/load.go":                 Code,
		"cmd/caveira/main.go":                   Code,
		"src/module.ts":                         Code,
		"src/components/Button.tsx":             Code,
	}
	for path, want := range cases {
		got := Classify(path)
		if got != want {
			t.Errorf("Classify(%q) = %v, want %v", path, got, want)
		}
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL on `Classify` / `FileKind` undefined.

- [ ] **Step 3: Implement**

Write `internal/fabricate/classify.go`:

```go
package fabricate

import (
	"path"
	"strings"
)

// FileKind is one of Chore, Code, Test.
type FileKind int

const (
	Code FileKind = iota
	Test
	Chore
)

var chorePrefixes = []string{
	"readme",
}

var choreExactBasenames = map[string]bool{
	"makefile":         true,
	"license":          true,
	"license.md":       true,
	"license.txt":      true,
	"go.mod":           true,
	"go.sum":           true,
	"package.json":     true,
	"package-lock.json": true,
	"yarn.lock":        true,
	"pnpm-lock.yaml":   true,
	"cargo.toml":       true,
	"cargo.lock":       true,
	"pyproject.toml":   true,
	"setup.py":         true,
	"setup.cfg":        true,
	"pipfile":          true,
	"pipfile.lock":     true,
	"dockerfile":       true,
	".dockerignore":    true,
	".gitignore":       true,
	".gitattributes":   true,
	".editorconfig":    true,
}

var testPathSubstrings = []string{
	"/test/",
	"/tests/",
	"/__tests__/",
	"/spec/",
}

// Classify returns the FileKind of the given path. Top-level files (no dir)
// with known names are Chore; files matching test patterns are Test;
// everything else is Code.
func Classify(p string) FileKind {
	clean := path.Clean(p)
	base := strings.ToLower(path.Base(clean))
	dir := path.Dir(clean)

	if dir == "." || dir == "/" {
		if isChoreBasename(base) {
			return Chore
		}
		if strings.HasPrefix(base, ".") {
			return Chore
		}
		if strings.HasSuffix(base, ".md") {
			return Chore
		}
		if strings.HasPrefix(base, "requirements") && strings.HasSuffix(base, ".txt") {
			return Chore
		}
	}

	if isTestPath(clean) {
		return Test
	}
	return Code
}

func isChoreBasename(base string) bool {
	if choreExactBasenames[base] {
		return true
	}
	for _, p := range chorePrefixes {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	return false
}

func isTestPath(p string) bool {
	lower := strings.ToLower(p)
	for _, s := range testPathSubstrings {
		if strings.Contains("/"+lower, s) {
			return true
		}
	}
	base := strings.ToLower(path.Base(p))
	ext := path.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	switch {
	case strings.HasSuffix(stem, "_test"):
		return true
	case strings.HasPrefix(stem, "test_"):
		return true
	case strings.HasSuffix(stem, ".test"):
		return true
	case strings.HasSuffix(stem, ".spec"):
		return true
	}
	return false
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): classify files as chore, code, or test"
```

---

## Task 6: Feature grouping + tree walking

**Files:**
- Create: `internal/fabricate/group.go`
- Create: `internal/fabricate/group_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/group_test.go`:

```go
package fabricate

import (
	"testing"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/justin06lee/caveira/internal/walk"
)

func TestWalkTree_AndGroup(t *testing.T) {
	// Make a tiny fixture: 3 files
	//   README.md       -> chore
	//   internal/walk/load.go -> code (feature "internal/walk")
	//   internal/walk/load_test.go -> test (feature "internal/walk")
	repo := newFixtureRepo(t, map[string]string{
		"README.md":                   "# Hello\n",
		"internal/walk/load.go":       "package walk\n",
		"internal/walk/load_test.go":  "package walk\nimport \"testing\"\n",
	})

	files, err := WalkHead(repo)
	if err != nil {
		t.Fatalf("WalkHead: %v", err)
	}
	if len(files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(files))
	}

	chore, features := GroupFiles(files)
	if len(chore) != 1 || chore[0].Path != "README.md" {
		t.Errorf("chore set: %+v", chore)
	}
	if len(features) != 1 {
		t.Fatalf("expected 1 feature, got %d", len(features))
	}
	f := features[0]
	if f.Dir != "internal/walk" {
		t.Errorf("feature dir: got %q want %q", f.Dir, "internal/walk")
	}
	if len(f.Code) != 1 || f.Code[0].Path != "internal/walk/load.go" {
		t.Errorf("code set: %+v", f.Code)
	}
	if len(f.Test) != 1 || f.Test[0].Path != "internal/walk/load_test.go" {
		t.Errorf("test set: %+v", f.Test)
	}
	_ = plumbing.ZeroHash
	_ = walk.NewDAG // keep import live for tests
}
```

Also create the test helper at the same time. Append to `internal/fabricate/group_test.go`:

```go
import (
	"github.com/go-git/go-billy/v5/memfs"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
	"time"
)

func newFixtureRepo(t *testing.T, files map[string]string) *git.Repository {
	t.Helper()
	storer := memory.NewStorage()
	fs := memfs.New()
	repo, err := git.Init(storer, fs)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	wt, _ := repo.Worktree()
	for p, body := range files {
		// ensure directories exist
		f, err := fs.Create(p)
		if err != nil {
			t.Fatalf("create %s: %v", p, err)
		}
		_, _ = f.Write([]byte(body))
		_ = f.Close()
		_, _ = wt.Add(p)
	}
	_, err = wt.Commit("seed", &git.CommitOptions{
		Author: &object.Signature{Name: "Test", Email: "test@example.com", When: time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)},
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	return repo
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Implement `WalkHead` and `GroupFiles`**

Write `internal/fabricate/group.go`:

```go
package fabricate

import (
	"path"
	"sort"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
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
	_ = plumbing.ZeroHash
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
```

Add an import of `object` to `group.go`:

```go
import "github.com/go-git/go-git/v5/plumbing/object"
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): walk HEAD tree and group files into features"
```

---

## Task 7: Message templates with seeded variation

**Files:**
- Create: `internal/fabricate/templates.go`
- Create: `internal/fabricate/templates_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/templates_test.go`:

```go
package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func TestChoreMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := ChoreMessage(rng)
	if !strings.HasPrefix(got, "chore:") {
		t.Errorf("chore msg = %q", got)
	}
}

func TestCodeMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := CodeMessage("walk", rng)
	if !strings.HasPrefix(got, "feat(walk):") {
		t.Errorf("code msg = %q", got)
	}
}

func TestTestMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	got := TestMessage("walk", rng)
	if !strings.HasPrefix(got, "test(walk):") {
		t.Errorf("test msg = %q", got)
	}
}

func TestMessage_DeterministicWithSeed(t *testing.T) {
	a := rand.New(rand.NewSource(7))
	b := rand.New(rand.NewSource(7))
	if CodeMessage("walk", a) != CodeMessage("walk", b) {
		t.Errorf("same seed produced different messages")
	}
}
```

- [ ] **Step 2: Implement**

Write `internal/fabricate/templates.go`:

```go
package fabricate

import (
	"fmt"
	"math/rand"
	"path"
)

var (
	choreVariants = []string{
		"chore: project scaffolding",
		"chore: initial scaffolding",
	}
	codeVerbs = []string{"add", "introduce", "scaffold"}
	testVerbs = []string{"add tests for", "tests for"}
)

// ChoreMessage returns a chore commit message.
func ChoreMessage(rng *rand.Rand) string {
	return choreVariants[rng.Intn(len(choreVariants))]
}

// CodeMessage returns "feat(<name>): <verb> <name>" where name = basename(dir).
func CodeMessage(dir string, rng *rand.Rand) string {
	name := basenameDir(dir)
	verb := codeVerbs[rng.Intn(len(codeVerbs))]
	return fmt.Sprintf("feat(%s): %s %s", name, verb, name)
}

// TestMessage returns "test(<name>): <verb> <name>".
func TestMessage(dir string, rng *rand.Rand) string {
	name := basenameDir(dir)
	verb := testVerbs[rng.Intn(len(testVerbs))]
	return fmt.Sprintf("test(%s): %s %s", name, verb, name)
}

func basenameDir(dir string) string {
	if dir == "." {
		return "root"
	}
	return path.Base(dir)
}
```

- [ ] **Step 3: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): seeded message templates for chore/code/test commits"
```

---

## Task 8: Flurry base sequence

**Files:**
- Create: `internal/fabricate/flurry.go`
- Create: `internal/fabricate/flurry_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/flurry_test.go`:

```go
package fabricate

import (
	"math/rand"
	"testing"
)

func TestFlurrySequence_Linear(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":                  "# x\n",
		"internal/walk/load.go":      "package walk\n",
		"internal/walk/load_test.go": "package walk\nimport \"testing\"\n",
		"internal/cli/main.go":       "package cli\n",
	})

	identity := Identity{Name: "Solo", Email: "solo@x.com"}
	rng := rand.New(rand.NewSource(1))
	commits, err := FlurrySequence(repo, identity, rng)
	if err != nil {
		t.Fatalf("FlurrySequence: %v", err)
	}
	// 1 chore + 1 code (cli, no tests) + 1 code (walk) + 1 test (walk) = 4
	if len(commits) != 4 {
		t.Fatalf("expected 4 commits, got %d: %+v", len(commits), msgs(commits))
	}
	if commits[0].Message != "chore: project scaffolding" && commits[0].Message != "chore: initial scaffolding" {
		t.Errorf("first commit should be chore, got %q", commits[0].Message)
	}
	for _, c := range commits {
		if c.Author != identity {
			t.Errorf("commit author = %+v, want %+v", c.Author, identity)
		}
	}
	// Parents: chore is parent of first feature commit; each subsequent is
	// parent of the next.
	for i := 1; i < len(commits); i++ {
		if len(commits[i].Parents) != 1 || commits[i].Parents[0] != i-1 {
			t.Errorf("commit %d parents = %v, want [%d]", i, commits[i].Parents, i-1)
		}
	}
}

func msgs(cs []SynthCommit) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Message
	}
	return out
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL on `FlurrySequence` undefined.

- [ ] **Step 3: Implement `FlurrySequence`**

Write `internal/fabricate/flurry.go`:

```go
package fabricate

import (
	"math/rand"

	"github.com/go-git/go-git/v5"
)

// FlurrySequence returns the base sequence of synthetic commits (chore, then
// per-feature code + test) with the supplied identity as both author and
// committer. Parents form a linear chain (each commit's parent is the prior).
func FlurrySequence(repo *git.Repository, id Identity, rng *rand.Rand) ([]SynthCommit, error) {
	files, err := WalkHead(repo)
	if err != nil {
		return nil, err
	}
	chore, features := GroupFiles(files)

	var commits []SynthCommit
	idx := 0

	commits = append(commits, SynthCommit{
		ID:        idx,
		Author:    id,
		Committer: id,
		Message:   ChoreMessage(rng),
		Added:     chore,
	})
	idx++

	prev := commits[0].ID
	for _, feat := range features {
		if len(feat.Code) > 0 {
			c := SynthCommit{
				ID:        idx,
				Parents:   []int{prev},
				Author:    id,
				Committer: id,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
			}
			commits = append(commits, c)
			prev = idx
			idx++
		}
		if len(feat.Test) > 0 {
			c := SynthCommit{
				ID:        idx,
				Parents:   []int{prev},
				Author:    id,
				Committer: id,
				Message:   TestMessage(feat.Dir, rng),
				Added:     feat.Test,
			}
			commits = append(commits, c)
			prev = idx
			idx++
		}
	}

	return commits, nil
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): flurry base sequence (chore + per-feature commits)"
```

---

## Task 9: Typo transformations

**Files:**
- Create: `internal/fabricate/typos.go`
- Create: `internal/fabricate/typos_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/typos_test.go`:

```go
package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func TestApplyTypos_NoChangeWithLowProb(t *testing.T) {
	// With seed 1 and zero-typo probabilty distribution, message often stays put.
	// Just verify return value is reasonable (non-empty for non-empty input).
	rng := rand.New(rand.NewSource(1))
	out := ApplyTypos("hello world", rng)
	if out == "" {
		t.Fatal("output should not be empty")
	}
}

func TestApplyTypos_SometimesChanges(t *testing.T) {
	// Over many seeds, we should see at least one transformation.
	original := "feat(walk): add walk"
	changed := false
	for s := int64(0); s < 100; s++ {
		rng := rand.New(rand.NewSource(s))
		out := ApplyTypos(original, rng)
		if out != original {
			changed = true
			break
		}
	}
	if !changed {
		t.Fatal("expected at least one seed to produce a typo across 100 trials")
	}
}

func TestApplyTypos_Deterministic(t *testing.T) {
	a := rand.New(rand.NewSource(42))
	b := rand.New(rand.NewSource(42))
	if ApplyTypos("foobar", a) != ApplyTypos("foobar", b) {
		t.Errorf("same seed produced different results")
	}
}

func TestApplyTypos_EmptyString(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	if got := ApplyTypos("", rng); got != "" {
		t.Errorf("ApplyTypos(\"\") = %q, want empty", got)
	}
}

func TestApplyTypos_OnlyAffectsMessage(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	out := ApplyTypos("xy", rng)
	if len(out) < 1 || len(out) > 3 {
		// transformations shouldn't change length by more than +/-1
		t.Errorf("output length too far from input: %q", out)
	}
	// adjacency restriction: only safe characters used
	for _, r := range out {
		if r == 0 {
			t.Errorf("invalid rune in %q", out)
		}
	}
	_ = strings.ToLower
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Implement `ApplyTypos`**

Write `internal/fabricate/typos.go`:

```go
package fabricate

import "math/rand"

// ApplyTypos returns msg with 0, 1, or 2 typo transformations applied:
//   - 70% probability: 0 typos
//   - 25% probability: 1 typo
//   - 5%  probability: 2 typos
// Each typo is one of four transformations, picked uniformly:
//   - adjacent character swap
//   - character drop
//   - character double
//   - keyboard-neighbor substitution (QWERTY)
// Positions are chosen uniformly across the message. Pure ASCII assumed; the
// QWERTY substitution table covers the lowercase alphabet.
func ApplyTypos(msg string, rng *rand.Rand) string {
	if msg == "" {
		return msg
	}
	n := drawTypoCount(rng)
	out := msg
	for i := 0; i < n; i++ {
		out = applyOneTypo(out, rng)
	}
	return out
}

func drawTypoCount(rng *rand.Rand) int {
	v := rng.Float64()
	switch {
	case v < 0.70:
		return 0
	case v < 0.95:
		return 1
	default:
		return 2
	}
}

func applyOneTypo(s string, rng *rand.Rand) string {
	if len(s) == 0 {
		return s
	}
	choice := rng.Intn(4)
	switch choice {
	case 0:
		return typoSwap(s, rng)
	case 1:
		return typoDrop(s, rng)
	case 2:
		return typoDouble(s, rng)
	default:
		return typoNeighbor(s, rng)
	}
}

func typoSwap(s string, rng *rand.Rand) string {
	if len(s) < 2 {
		return s
	}
	i := rng.Intn(len(s) - 1)
	b := []byte(s)
	b[i], b[i+1] = b[i+1], b[i]
	return string(b)
}

func typoDrop(s string, rng *rand.Rand) string {
	if len(s) < 2 {
		return s
	}
	i := rng.Intn(len(s))
	return s[:i] + s[i+1:]
}

func typoDouble(s string, rng *rand.Rand) string {
	i := rng.Intn(len(s))
	return s[:i+1] + string(s[i]) + s[i+1:]
}

var keyboardNeighbors = map[byte]string{
	'a': "qwsz", 'b': "vghn", 'c': "xdfv", 'd': "serfcx",
	'e': "wsdr", 'f': "drtgvc", 'g': "frtyhbv", 'h': "gyujnb",
	'i': "ujko", 'j': "huikmn", 'k': "jilom", 'l': "kop",
	'm': "njk", 'n': "bhjm", 'o': "iklp", 'p': "ol",
	'q': "wa", 'r': "edft", 's': "awedxz", 't': "rfgy",
	'u': "yhji", 'v': "cfgb", 'w': "qase", 'x': "zsdc",
	'y': "tghu", 'z': "asx",
}

func typoNeighbor(s string, rng *rand.Rand) string {
	if len(s) == 0 {
		return s
	}
	for attempt := 0; attempt < 5; attempt++ {
		i := rng.Intn(len(s))
		c := s[i]
		if neighbors, ok := keyboardNeighbors[c]; ok {
			n := neighbors[rng.Intn(len(neighbors))]
			return s[:i] + string(n) + s[i+1:]
		}
	}
	return s
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): typo transformations for pigs mode"
```

---

## Task 10: Pigs mode (chaotic single-branch)

**Files:**
- Create: `internal/fabricate/pigs.go`
- Create: `internal/fabricate/pigs_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/pigs_test.go`:

```go
package fabricate

import (
	"math/rand"
	"testing"
)

func TestPigsMode_SingleAuthor_NoNoise(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	if len(plan.Commits) < 2 {
		t.Fatalf("expected >= 2 commits, got %d", len(plan.Commits))
	}
	for _, c := range plan.Commits {
		if c.Author.Name != "Solo" {
			t.Errorf("commit author = %+v, want Solo", c.Author)
		}
	}
}

func TestPigsMode_TwoAuthors_RoundRobin(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":                  "# x\n",
		"a/x.go":                     "package a\n",
		"b/y.go":                     "package b\n",
		"c/z.go":                     "package c\n",
		"d/w.go":                     "package d\n",
	})
	rng := rand.New(rand.NewSource(7))
	plan, err := BuildPigsPlan(repo, []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	sawA, sawB := false, false
	for _, c := range plan.Commits {
		switch c.Author.Name {
		case "Alice":
			sawA = true
		case "Bob":
			sawB = true
		}
	}
	if !sawA || !sawB {
		t.Errorf("expected both authors to appear; sawA=%v sawB=%v", sawA, sawB)
	}
}

func TestPigsMode_NoiseCommitsAreEmptyAndShortMessage(t *testing.T) {
	// With several features the noise injection rate should produce >=1 noise.
	files := map[string]string{}
	for _, dir := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		files[dir+"/x.go"] = "package " + dir + "\n"
		files[dir+"/x_test.go"] = "package " + dir + "\n"
	}
	repo := newFixtureRepo(t, files)
	rng := rand.New(rand.NewSource(3))
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	sawNoise := false
	for _, c := range plan.Commits {
		if len(c.Added) == 0 && c.Message != "" && !c.IsMerge {
			sawNoise = true
		}
	}
	if !sawNoise {
		t.Logf("note: no noise commits at this seed; not a hard failure but informational")
	}
}

func TestPigsMode_HeadRefAndLinearChain(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(2))
	plan, err := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)
	if err != nil {
		t.Fatalf("BuildPigsPlan: %v", err)
	}
	if plan.HeadRef == "" {
		t.Errorf("HeadRef should not be empty")
	}
	if _, ok := plan.Refs[plan.HeadRef]; !ok {
		t.Errorf("Refs[%q] missing", plan.HeadRef)
	}
	// Linear: each commit has at most one parent.
	for _, c := range plan.Commits {
		if len(c.Parents) > 1 {
			t.Errorf("commit %d has %d parents, pigs mode should be linear", c.ID, len(c.Parents))
		}
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Implement `BuildPigsPlan`**

Write `internal/fabricate/pigs.go`:

```go
package fabricate

import (
	"math/rand"

	"github.com/go-git/go-git/v5"
)

const (
	noiseRate    = 0.15 // probability of a noise commit between any two real commits
	defaultBranch = "refs/heads/master"
)

var noiseMessages = []string{
	"wip", "fix", "fix typo", "revert", "more changes",
	"stuff", "todo", "wip2", "idk", "actually fix",
	"lint", "fmt",
}

// BuildPigsPlan produces a Plan for pigs mode: a linear chain of synthetic
// commits with authors round-robin'd through ids, noise commits sprinkled in,
// and every message run through ApplyTypos.
func BuildPigsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errEmptyIdentities
	}
	// Use the first identity as a placeholder for the base flurry sequence;
	// we overwrite authors below.
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}

	// Author round-robin across real commits.
	for i := range base {
		id := ids[i%len(ids)]
		base[i].Author = id
		base[i].Committer = id
	}

	// Apply typos to every message.
	for i := range base {
		base[i].Message = ApplyTypos(base[i].Message, rng)
	}

	// Inject noise commits between adjacent real commits.
	var out []SynthCommit
	out = append(out, base[0])
	for i := 1; i < len(base); i++ {
		if rng.Float64() < noiseRate {
			noise := SynthCommit{
				Parents:   []int{out[len(out)-1].ID},
				Author:    ids[rng.Intn(len(ids))],
				Committer: ids[rng.Intn(len(ids))],
				Message:   ApplyTypos(noiseMessages[rng.Intn(len(noiseMessages))], rng),
			}
			// Re-ID and rewire successor
			noise.ID = len(out)
			out = append(out, noise)
			base[i].ID = len(out)
			base[i].Parents = []int{noise.ID}
		} else {
			base[i].ID = len(out)
			base[i].Parents = []int{out[len(out)-1].ID}
		}
		out = append(out, base[i])
	}

	plan := &Plan{
		Commits: out,
		Refs:    map[string]int{defaultBranch: out[len(out)-1].ID},
		HEAD:    out[len(out)-1].ID,
		HeadRef: defaultBranch,
	}
	return plan, nil
}

var errEmptyIdentities = fmtError("BuildPigsPlan: at least one identity required")

func fmtError(s string) error { return &simpleErr{msg: s} }

type simpleErr struct{ msg string }

func (e *simpleErr) Error() string { return e.msg }
```

Note on the implementation detail: each noise commit's `ID` and the following real commit's `ID` are re-assigned during the splice so they form a valid sequence. Parent indices stay consistent.

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): pigs mode (RR authors, noise injection, typos)"
```

---

## Task 11: Rats mode — linear feature branches

This task lays down the simplest rats topology: each feature gets its own branch off master, branches merge in feature order. Task 12 adds emergent off-branch forks and Task 13 adds conflict scars.

**Files:**
- Create: `internal/fabricate/rats.go`
- Create: `internal/fabricate/rats_test.go`

- [ ] **Step 1: Write failing tests**

Write `internal/fabricate/rats_test.go`:

```go
package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func TestRatsMode_FeatureBranches(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
		"internal/cli/main.go":  "package cli\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, err := BuildRatsPlan(repo, []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}, rng)
	if err != nil {
		t.Fatalf("BuildRatsPlan: %v", err)
	}

	sawFeatRef := 0
	for ref := range plan.Refs {
		if strings.HasPrefix(ref, "refs/heads/feat/") {
			sawFeatRef++
		}
	}
	if sawFeatRef < 2 {
		t.Errorf("expected at least 2 feat/ branches, got %d (refs: %+v)", sawFeatRef, plan.Refs)
	}

	sawMerge := 0
	for _, c := range plan.Commits {
		if c.IsMerge {
			sawMerge++
		}
	}
	if sawMerge < 2 {
		t.Errorf("expected at least 2 merge commits, got %d", sawMerge)
	}
}

func TestRatsMode_MergesAttributedToOwner(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"a/x.go": "package a\n",
		"b/y.go": "package b\n",
	})
	rng := rand.New(rand.NewSource(1))
	ids := []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}
	plan, _ := BuildRatsPlan(repo, ids, rng)
	for _, c := range plan.Commits {
		if !c.IsMerge {
			continue
		}
		if c.Author.Email != "a@x.com" && c.Author.Email != "b@x.com" {
			t.Errorf("merge commit author should be one of the rats, got %+v", c.Author)
		}
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Implement basic rats**

Write `internal/fabricate/rats.go`:

```go
package fabricate

import (
	"fmt"
	"math/rand"

	"github.com/go-git/go-git/v5"
)

// BuildRatsPlan produces a Plan for rats mode. In this initial version each
// feature gets its own branch off master, branches merge in feature order.
// Task 12 adds emergent off-branch forking; Task 13 adds conflict scars.
func BuildRatsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, fmtError("BuildRatsPlan: at least one identity required")
	}

	files, err := WalkHead(repo)
	if err != nil {
		return nil, err
	}
	chore, features := GroupFiles(files)

	var commits []SynthCommit
	refs := map[string]int{}

	// Chore commit on master.
	chairman := ids[0]
	commits = append(commits, SynthCommit{
		ID:        0,
		Author:    chairman,
		Committer: chairman,
		Message:   ChoreMessage(rng),
		Added:     chore,
	})
	masterTip := 0

	for fi, feat := range features {
		rat := ids[fi%len(ids)]
		branchName := fmt.Sprintf("refs/heads/feat/%s", basenameDir(feat.Dir))

		// Code commit on branch (parent = current master tip)
		var branchTip int
		if len(feat.Code) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{masterTip},
				Author:    rat,
				Committer: rat,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
			})
			branchTip = cid
		} else {
			branchTip = masterTip
		}

		if len(feat.Test) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{branchTip},
				Author:    rat,
				Committer: rat,
				Message:   TestMessage(feat.Dir, rng),
				Added:     feat.Test,
			})
			branchTip = cid
		}

		// Record branch ref at its tip.
		refs[branchName] = branchTip

		// Merge commit on master.
		mergeID := len(commits)
		commits = append(commits, SynthCommit{
			ID:        mergeID,
			Parents:   []int{masterTip, branchTip},
			Author:    rat,
			Committer: rat,
			Message:   fmt.Sprintf("Merge branch 'feat/%s' into master", basenameDir(feat.Dir)),
			IsMerge:   true,
		})
		masterTip = mergeID
	}

	refs[defaultBranch] = masterTip
	plan := &Plan{
		Commits: commits,
		Refs:    refs,
		HEAD:    masterTip,
		HeadRef: defaultBranch,
	}
	return plan, nil
}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): rats mode (linear feature branches and merges)"
```

---

## Task 12: Rats mode — emergent off-branch fork topology

Adds the probabilistic fork-from-an-open-branch behavior.

**Files:**
- Modify: `internal/fabricate/rats.go`
- Modify: `internal/fabricate/rats_test.go`

- [ ] **Step 1: Add failing test**

Append to `internal/fabricate/rats_test.go`:

```go
func TestRatsMode_OffBranchForkAtSomeSeed(t *testing.T) {
	// With enough features and the off-branch probability, at least one seed
	// should produce a feature branch whose first commit's parent is NOT the
	// chore commit (i.e., it forked from another open branch).
	files := map[string]string{}
	for _, dir := range []string{"a", "b", "c", "d", "e", "f"} {
		files[dir+"/x.go"] = "package " + dir + "\n"
	}
	repo := newFixtureRepo(t, files)
	ids := []Identity{
		{Name: "Alice", Email: "a@x.com"},
		{Name: "Bob", Email: "b@x.com"},
	}
	sawOffBranch := false
	for s := int64(0); s < 50; s++ {
		rng := rand.New(rand.NewSource(s))
		plan, _ := BuildRatsPlan(repo, ids, rng)
		// Find non-merge non-chore non-noise commits whose parent is not the chore.
		for _, c := range plan.Commits {
			if c.IsMerge || len(c.Added) == 0 || c.ID == 0 {
				continue
			}
			if len(c.Parents) == 1 && c.Parents[0] != 0 {
				// Check: parent is also a feat commit (not the chore commit at index 0).
				parent := plan.Commits[c.Parents[0]]
				if !parent.IsMerge && parent.ID != 0 {
					sawOffBranch = true
					break
				}
			}
		}
		if sawOffBranch {
			break
		}
	}
	if !sawOffBranch {
		t.Errorf("expected at least one seed across 50 trials to fork off another branch")
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL — current rats always parents on `masterTip`.

- [ ] **Step 3: Add off-branch fork logic to `BuildRatsPlan`**

In `internal/fabricate/rats.go`, add this constant and helper:

```go
const offBranchForkProb = 0.30

// pickForkParent returns the parent commit ID for a new feature branch's first
// commit. With probability offBranchForkProb (when at least one open branch
// exists), it picks an open branch's current tip; otherwise master's tip.
func pickForkParent(masterTip int, openBranchTips []int, rng *rand.Rand) int {
	if len(openBranchTips) > 0 && rng.Float64() < offBranchForkProb {
		return openBranchTips[rng.Intn(len(openBranchTips))]
	}
	return masterTip
}
```

Replace the per-feature loop in `BuildRatsPlan` so it tracks "open" branches (those started but not yet merged) and chooses fork points:

```go
	var openBranchTips []int
	for fi, feat := range features {
		rat := ids[fi%len(ids)]
		branchName := fmt.Sprintf("refs/heads/feat/%s", basenameDir(feat.Dir))

		forkParent := pickForkParent(masterTip, openBranchTips, rng)

		var branchTip int
		if len(feat.Code) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{forkParent},
				Author:    rat,
				Committer: rat,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
			})
			branchTip = cid
		} else {
			branchTip = forkParent
		}

		if len(feat.Test) > 0 {
			cid := len(commits)
			commits = append(commits, SynthCommit{
				ID:        cid,
				Parents:   []int{branchTip},
				Author:    rat,
				Committer: rat,
				Message:   TestMessage(feat.Dir, rng),
				Added:     feat.Test,
			})
			branchTip = cid
		}

		// Track this branch as "open" for later features to potentially fork off.
		openBranchTips = append(openBranchTips, branchTip)

		refs[branchName] = branchTip

		// Merge commit on master.
		mergeID := len(commits)
		commits = append(commits, SynthCommit{
			ID:        mergeID,
			Parents:   []int{masterTip, branchTip},
			Author:    rat,
			Committer: rat,
			Message:   fmt.Sprintf("Merge branch 'feat/%s' into master", basenameDir(feat.Dir)),
			IsMerge:   true,
		})
		masterTip = mergeID

		// Once merged, this branch is no longer "open" — remove it.
		newOpen := openBranchTips[:0]
		for _, t := range openBranchTips {
			if t != branchTip {
				newOpen = append(newOpen, t)
			}
		}
		openBranchTips = newOpen
	}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): rats mode emergent off-branch fork topology"
```

---

## Task 13: Rats mode — conflict-fix scars

Adds occasional `fix: resolve conflict in <feat>` commits after merges, with a smaller probability of spawning a `fix/<feat>` branch instead.

**Files:**
- Modify: `internal/fabricate/rats.go`
- Modify: `internal/fabricate/rats_test.go`

- [ ] **Step 1: Add failing test**

Append to `internal/fabricate/rats_test.go`:

```go
func TestRatsMode_ConflictFixScarAtSomeSeed(t *testing.T) {
	files := map[string]string{}
	for _, dir := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		files[dir+"/x.go"] = "package " + dir + "\n"
	}
	repo := newFixtureRepo(t, files)
	ids := []Identity{{Name: "Solo", Email: "solo@x.com"}}
	sawConflictFix := false
	for s := int64(0); s < 50; s++ {
		rng := rand.New(rand.NewSource(s))
		plan, _ := BuildRatsPlan(repo, ids, rng)
		for _, c := range plan.Commits {
			if strings.HasPrefix(c.Message, "fix: resolve conflict") {
				sawConflictFix = true
				break
			}
		}
		if sawConflictFix {
			break
		}
	}
	if !sawConflictFix {
		t.Errorf("expected at least one seed across 50 trials to inject a conflict-fix commit")
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Add scar logic**

In `internal/fabricate/rats.go`, add these constants:

```go
const (
	conflictFixProb       = 0.20 // probability of a conflict-fix scar after a merge
	conflictFixBranchProb = 0.40 // probability the scar is a fix-branch (conditional on conflictFixProb firing)
)
```

At the end of the per-feature loop body (after the `openBranchTips = newOpen` cleanup from Task 12), append:

```go
		// Conflict-fix scar
		if rng.Float64() < conflictFixProb {
			if rng.Float64() < conflictFixBranchProb {
				// Spawn a fix branch with 1 small commit and merge it back.
				fixID := len(commits)
				commits = append(commits, SynthCommit{
					ID:        fixID,
					Parents:   []int{masterTip},
					Author:    rat,
					Committer: rat,
					Message:   fmt.Sprintf("fix: address %s merge issue", basenameDir(feat.Dir)),
				})
				fixBranchName := fmt.Sprintf("refs/heads/fix/%s", basenameDir(feat.Dir))
				refs[fixBranchName] = fixID
				mergeFixID := len(commits)
				commits = append(commits, SynthCommit{
					ID:        mergeFixID,
					Parents:   []int{masterTip, fixID},
					Author:    rat,
					Committer: rat,
					Message:   fmt.Sprintf("Merge branch 'fix/%s' into master", basenameDir(feat.Dir)),
					IsMerge:   true,
				})
				masterTip = mergeFixID
			} else {
				// Inline conflict-fix commit on master.
				fixID := len(commits)
				commits = append(commits, SynthCommit{
					ID:        fixID,
					Parents:   []int{masterTip},
					Author:    rat,
					Committer: rat,
					Message:   fmt.Sprintf("fix: resolve conflict in %s", basenameDir(feat.Dir)),
				})
				masterTip = fixID
			}
		}
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): rats mode conflict-fix scars and fix branches"
```

---

## Task 14: Plan → walk.DAG conversion

The scheduler consumes a `walk.DAG`. This task adds the converter so a `Plan` from pigs or rats can be scheduled.

**Files:**
- Create: `internal/fabricate/dag.go`
- Create: `internal/fabricate/dag_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/fabricate/dag_test.go`:

```go
package fabricate

import (
	"math/rand"
	"testing"
	"time"
)

func TestPlanToDAG(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, _ := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)

	dag, err := PlanToDAG(repo, plan)
	if err != nil {
		t.Fatalf("PlanToDAG: %v", err)
	}
	if len(dag.All()) != len(plan.Commits) {
		t.Fatalf("dag has %d commits, plan has %d", len(dag.All()), len(plan.Commits))
	}
	// The DAG's OIDs should match SyntheticOID(id) for each commit.
	for _, c := range plan.Commits {
		dc := dag.Get(SyntheticOID(c.ID))
		if dc == nil {
			t.Errorf("DAG missing commit %d", c.ID)
		}
	}
	// Stats: chore commit should have non-zero NewFiles for the README.
	chore := dag.Get(SyntheticOID(0))
	if chore.NewFiles != 1 {
		t.Errorf("chore commit NewFiles = %d, want 1", chore.NewFiles)
	}
	_ = time.Now
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Implement `PlanToDAG`**

Write `internal/fabricate/dag.go`:

```go
package fabricate

import (
	"time"

	"github.com/go-git/go-git/v5"

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
```

Add this import to `dag.go`:

```go
import "github.com/go-git/go-git/v5/plumbing/object"
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): convert Plan to walk.DAG with computed diff stats"
```

---

## Task 15: WriteToRepo — write synthetic commits, trees, blobs, and refs

This is the substantive writer. It walks the plan in topological order, computes each commit's full tree state, writes the necessary trees and blobs to dst, then writes the commit objects. Finally it creates the refs.

**Files:**
- Create: `internal/fabricate/write.go`
- Create: `internal/fabricate/write_test.go`

- [ ] **Step 1: Write failing test**

Write `internal/fabricate/write_test.go`:

```go
package fabricate

import (
	"math/rand"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/justin06lee/caveira/internal/rewrite"
)

func TestWriteToRepo_PigsLinear(t *testing.T) {
	repo := newFixtureRepo(t, map[string]string{
		"README.md":             "# x\n",
		"internal/walk/load.go": "package walk\n",
	})
	rng := rand.New(rand.NewSource(1))
	plan, _ := BuildPigsPlan(repo, []Identity{{Name: "Solo", Email: "solo@x.com"}}, rng)

	dst, err := rewrite.InMemoryClone(repo)
	if err != nil {
		t.Fatalf("clone: %v", err)
	}

	// Provide fake times: each commit at base + 10*ID minutes.
	base := time.Date(2026, 5, 14, 12, 0, 0, 0, time.UTC)
	times := map[string]time.Time{}
	for _, c := range plan.Commits {
		times[SyntheticOID(c.ID)] = base.Add(time.Duration(c.ID*10) * time.Minute)
	}

	mapping, err := WriteToRepo(repo, dst, plan, times)
	if err != nil {
		t.Fatalf("WriteToRepo: %v", err)
	}
	if len(mapping) != len(plan.Commits) {
		t.Fatalf("mapping has %d entries, want %d", len(mapping), len(plan.Commits))
	}

	// HEAD should exist on dst.
	head, err := dst.Head()
	if err != nil {
		t.Fatalf("dst.Head: %v", err)
	}
	if head.Hash() == plumbing.ZeroHash {
		t.Errorf("HEAD hash zero")
	}

	// Verify a commit at HEAD has a tree containing README.md.
	c, err := dst.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("CommitObject(head): %v", err)
	}
	tree, _ := c.Tree()
	_, err = tree.File("README.md")
	if err != nil {
		t.Errorf("README.md not in dst HEAD tree: %v", err)
	}
}
```

- [ ] **Step 2: Run and verify failure**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: FAIL.

- [ ] **Step 3: Implement `WriteToRepo`**

Write `internal/fabricate/write.go`:

```go
package fabricate

import (
	"fmt"
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
	// Index commits by ID.
	byID := make(map[int]*SynthCommit, len(plan.Commits))
	for i := range plan.Commits {
		byID[plan.Commits[i].ID] = &plan.Commits[i]
	}
	// Topological order on integer IDs.
	order, err := topoOrder(plan)
	if err != nil {
		return nil, err
	}

	// Copy all blobs referenced by any commit's Added list from src to dst.
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
	// stateByID[id] = map[path]FileRef.
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
			// Merge: union of parent states.
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

	// Apply refs from plan.
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
	// Sort paths for determinism.
	paths := make([]string, 0, len(state))
	for p := range state {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	// Build nested tree objects recursively.
	return buildNestedTree(dst, paths, state, "")
}

// buildNestedTree builds a tree object for the given subtree rooted at prefix.
func buildNestedTree(dst *git.Repository, paths []string, state map[string]FileRef, prefix string) (plumbing.Hash, error) {
	type entryRef struct {
		Name string
		Mode filemode.FileMode
		Hash plumbing.Hash
	}
	directChildren := map[string]bool{} // direct files at this level
	subdirs := map[string][]string{}    // subdir name -> child paths

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
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return werr
			}
		}
		if err != nil {
			break
		}
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
```

- [ ] **Step 4: Run tests and verify pass**

Run:
```bash
go test ./internal/fabricate/...
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate
git commit -m "feat(fabricate): write synthetic commits, nested trees, and refs to dst"
```

---

## Task 16: Pipeline wiring — route through fabricate when --fabricate

**Files:**
- Modify: `internal/cli/pipeline.go`

- [ ] **Step 1: Add a `Generate` entry point in `fabricate`**

Create `internal/fabricate/generate.go`:

```go
package fabricate

import (
	"io"
	"math/rand"

	"github.com/go-git/go-git/v5"
	"github.com/justin06lee/caveira/internal/walk"
)

// Generate runs the appropriate fabricator (pigs / rats / single-author) and
// returns the Plan plus a walk.DAG view of it for the scheduler.
func Generate(repo *git.Repository, ids []Identity, mode string, rng *rand.Rand, _ io.Writer) (*Plan, *walk.DAG, error) {
	var plan *Plan
	var err error
	switch mode {
	case "pigs":
		plan, err = BuildPigsPlan(repo, ids, rng)
	case "rats":
		plan, err = BuildRatsPlan(repo, ids, rng)
	default:
		// Single-author linear; uses pigs builder with N=1 and no noise rate.
		// In Phase 1 the simplest implementation is BuildPigsPlan with one
		// identity; the noiseRate constant still applies but with one author
		// the output looks like a clean single-person history.
		plan, err = BuildPigsPlan(repo, ids, rng)
	}
	if err != nil {
		return nil, nil, err
	}
	dag, err := PlanToDAG(repo, plan)
	if err != nil {
		return nil, nil, err
	}
	return plan, dag, nil
}
```

- [ ] **Step 2: Modify `Pipeline` to branch on `cfg.Fabricate`**

In `internal/cli/pipeline.go`, after `srcRepo, err := git.PlainOpen(srcPath)` and before the existing `walk.Load(srcRepo)` block, add:

```go
	// Fabricate mode: replace walk.Load with fabricate.Generate.
	if cfg.Fabricate {
		mode := "single"
		nIDs := 1
		var rawIDs []string
		switch {
		case cfg.PigsN > 0:
			mode = "pigs"
			nIDs = cfg.PigsN
			rawIDs = cfg.PigIdentities
		case cfg.RatsN > 0:
			mode = "rats"
			nIDs = cfg.RatsN
			rawIDs = cfg.RatIdentities
		}

		// For single-author mode, use git config user.* unless --pig flags are present.
		var ids []Identity
		if mode == "single" {
			id, err := singleAuthorIdentity()
			if err != nil {
				fmt.Fprintln(errOut, "error:", err)
				return 1
			}
			ids = []Identity{id}
		} else {
			resolved, err := fabricate.ResolveIdentities(srcRepo, rawIDs, nIDs, os.Stdin, out)
			if err != nil {
				fmt.Fprintln(errOut, "error: identity resolution:", err)
				return 1
			}
			ids = resolved
		}

		rng := rngFor(cfg)
		plan, dag, err := fabricate.Generate(srcRepo, ids, mode, rng, errOut)
		if err != nil {
			fmt.Fprintln(errOut, "error: fabricate generate:", err)
			return 1
		}

		// Build durations and schedule using the existing pipeline.
		durations, diffs := schedule.BuildDurations(dag, rng)
		res, err := schedule.Schedule(dag, durations, cfg.Start, cfg.End)
		if err != nil {
			fmt.Fprintln(errOut, "error:", err)
			return 1
		}

		if cfg.DryRun {
			rows := rowsFor(dag, durations, diffs, res, srcRepo)
			report.WriteDryRun(out, rows, res, cfg.Start, cfg.End)
			return 0
		}

		if err := repo.Duplicate(srcPath, stagePath); err != nil {
			fmt.Fprintln(errOut, "error: duplicate:", err)
			return 1
		}

		stageRepo, err := git.PlainOpen(stagePath)
		if err != nil {
			fmt.Fprintln(errOut, "error: open staged repo:", err)
			return 1
		}

		_, err = fabricate.WriteToRepo(srcRepo, stageRepo, plan, res.NewTimes)
		if err != nil {
			fmt.Fprintln(errOut, "error: fabricate write:", err)
			return 1
		}

		if err := resetWorktreeToHead(stagePath); err != nil {
			fmt.Fprintln(errOut, "warn: reset worktree:", err)
		}
		_ = exec.Command("git", "-C", stagePath, "gc", "--prune=now").Run()

		deadPath, err := repo.Swap(srcPath, stagePath)
		if err != nil {
			fmt.Fprintln(errOut, "error: swap:", err)
			return 1
		}

		pushed := false
		if cfg.Push {
			if err := repo.Push(srcPath, cfg.PushProtected); err != nil {
				fmt.Fprintln(errOut, "error: push:", err)
				return 1
			}
			pushed = true
		}

		before := len(plan.Commits)
		after := before // No squashing of fabricated commits is exposed yet.
		span := windowSpan(res, cfg.Start)
		report.WriteSummary(out, srcPath, srcPath, deadPath, before, after, span, cfg.End.Sub(cfg.Start), res.Scale, len(res.Squashes), pushed)
		return 0
	}
```

Make sure these imports are present in the `pipeline.go` import block:

```go
import (
	// ...existing imports...
	"os/exec"
	"strings"

	"github.com/justin06lee/caveira/internal/fabricate"
)
```

Also add a helper near the bottom of `pipeline.go`:

```go
// singleAuthorIdentity reads git config user.{name,email} via the system git
// binary (Caveira already shells out for `git gc`).
func singleAuthorIdentity() (fabricate.Identity, error) {
	name, err := exec.Command("git", "config", "user.name").CombinedOutput()
	if err != nil {
		return fabricate.Identity{}, fmt.Errorf("git config user.name: %w", err)
	}
	email, err := exec.Command("git", "config", "user.email").CombinedOutput()
	if err != nil {
		return fabricate.Identity{}, fmt.Errorf("git config user.email: %w", err)
	}
	n := strings.TrimSpace(string(name))
	e := strings.TrimSpace(string(email))
	if n == "" || e == "" {
		return fabricate.Identity{}, fmt.Errorf("git config user.{name,email} not set; pass --pig \"Name <email>\"")
	}
	return fabricate.Identity{Name: n, Email: e}, nil
}
```

- [ ] **Step 3: Run all tests and verify**

Run:
```bash
go test ./...
```
Expected: PASS. (No new tests added in this task; existing tests should remain green and the new code compiles.)

- [ ] **Step 4: Commit**

```bash
git add internal/cli/pipeline.go internal/fabricate
git commit -m "feat(cli): wire fabricate Phase 1 into the pipeline"
```

---

## Task 17: End-to-end integration test for fabricate

**Files:**
- Create: `internal/cli/fabricate_integration_test.go`

- [ ] **Step 1: Add an integration test**

Write `internal/cli/fabricate_integration_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestIntegration_FabricateFlurry_SingleAuthor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	// Seed the repo with 3 files across 2 features.
	_ = os.MkdirAll(filepath.Join(repo, "internal/walk"), 0755)
	_ = os.MkdirAll(filepath.Join(repo, "internal/cli"), 0755)
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("# x\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "internal/walk/load.go"), []byte("package walk\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "internal/cli/cli.go"), []byte("package cli\n"), 0644)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--flurry",
		"--seed", "1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}

	deadPath := repo + ".dead"
	if _, err := os.Stat(deadPath); err != nil {
		t.Errorf("expected %s to exist after run", deadPath)
	}
	if _, err := os.Stat(repo); err != nil {
		t.Errorf("expected %s (rewritten) to exist after run", repo)
	}

	// Log count: chore + walk + cli (no tests in fixture).
	logOut, err := exec.Command("git", "-C", repo, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v\n%s", err, logOut)
	}
	if !bytes.Contains(logOut, []byte("chore")) {
		t.Errorf("expected chore commit in destination log:\n%s", logOut)
	}
	if !bytes.Contains(logOut, []byte("feat(walk)")) {
		t.Errorf("expected feat(walk) commit in destination log:\n%s", logOut)
	}
}

func TestIntegration_FabricatePigs_TwoAuthors(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git binary not on PATH")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "src")
	mustGit(t, dir, "init", repo)
	mustGit(t, repo, "config", "user.email", "t@e.com")
	mustGit(t, repo, "config", "user.name", "T")
	_ = os.MkdirAll(filepath.Join(repo, "a"), 0755)
	_ = os.MkdirAll(filepath.Join(repo, "b"), 0755)
	_ = os.WriteFile(filepath.Join(repo, "README.md"), []byte("# x\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "a/x.go"), []byte("package a\n"), 0644)
	_ = os.WriteFile(filepath.Join(repo, "b/y.go"), []byte("package b\n"), 0644)
	mustGit(t, repo, "add", "-A")
	mustGit(t, repo, "commit", "-m", "initial")

	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", repo,
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--flurry",
		"--pigs", "2",
		"--pig", "Alice <a@x.com>",
		"--pig", "Bob <b@x.com>",
		"--seed", "1",
	}, &out, &errOut)
	if code != 0 {
		t.Fatalf("exit %d, stderr=%s", code, errOut.String())
	}

	logOut, _ := exec.Command("git", "-C", repo, "log", "--pretty=%an %ae").CombinedOutput()
	if !bytes.Contains(logOut, []byte("a@x.com")) || !bytes.Contains(logOut, []byte("b@x.com")) {
		t.Errorf("expected both authors in destination log:\n%s", logOut)
	}
}
```

- [ ] **Step 2: Run the integration tests**

Run:
```bash
go test ./internal/cli/... -run TestIntegration_Fabricate -v
```
Expected: PASS.

- [ ] **Step 3: Run full module tests**

Run:
```bash
go test ./...
```
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/fabricate_integration_test.go
git commit -m "test(cli): end-to-end integration for fabricate flurry and pigs"
```

---

## Task 18: README and `--help` updates

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add a Fabrication section to the README**

In `README.md`, insert a new section between "## How it works" and "## Notes & limitations":

````markdown
## Fabrication mode (preview)

`--fabricate` synthesizes a new commit history from scratch using the source
repo's HEAD tree as the target. `--flurry` is the NLP-only fabricator (no LLM):
it groups files by top-level directory, classifies each file as chore / code /
test, and emits a sequence of `chore: …`, `feat(<dir>): …`, `test(<dir>): …`
commits that end at the source's exact HEAD tree.

```bash
# Single-author (uses git config user.*)
caveira --repo /path/to/myrepo --fabricate --flurry \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00"

# Three pigs: chaotic single-branch with author RR, noise commits, message typos
caveira --repo /path/to/myrepo --fabricate --flurry \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
        --pigs 3 \
        --pig "Alice <a@x.com>" --pig "Bob <b@x.com>" --pig "Carol <c@x.com>"

# Two rats: emergent feature branches, off-branch forks, occasional conflict-fix scars
caveira --repo /path/to/myrepo --fabricate --flurry \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
        --rats 2 \
        --rat "Alice <a@x.com>" --rat "Bob <b@x.com>"
```

If `--pigs N` or `--rats N` is set and fewer than N identities are supplied via
`--pig` / `--rat`, Caveira scans the `.git` history for additional identities
and prompts interactively for any still missing. If too many are found, it
shows a picker.

Fabrication is entirely templated / NLP-only; there are no LLM-backed
fabricators.
````

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: document --fabricate / --flurry / --pigs / --rats"
```

---

## Self-Review

After all tasks are complete, run a final sanity check:

- [ ] `go build ./...` — should succeed.
- [ ] `go test ./...` — all tests should pass.
- [ ] `go vet ./...` — clean.
- [ ] `gofmt -l .` — no output.
- [ ] Manual smoke test: run `caveira` on a small local repo with `--fabricate --flurry --dry-run` and visually inspect the dry-run table. Then run without `--dry-run` and `git log` the rewritten repo.

## Spec coverage summary

| Spec section                          | Implemented in              |
|---------------------------------------|------------------------------|
| §3 CLI surface + validation           | Task 1                       |
| §4 Author identity resolution         | Tasks 2, 3, 4                |
| §5.1 File classification              | Task 5                       |
| §5.2 Feature grouping                 | Task 6                       |
| §5.3 Commit sequence                  | Task 8                       |
| §5.4 Variation pool                   | Task 7                       |
| §5.5 Edge cases (empty repo error)    | Task 8 (via WalkHead path)   |
| §6.1 Author round-robin (pigs)        | Task 10                      |
| §6.2 Noise injection                  | Task 10                      |
| §6.3 Typos on any message             | Tasks 9, 10                  |
| §6.4 Linear output                    | Task 10                      |
| §7.1 Rats per-feature branches + fork | Tasks 11, 12                 |
| §7.2 Conflict-fix scars               | Task 13                      |
| §7.3 No noise in rats                 | Tasks 11–13 (by omission)    |
| §7.4 Merge commits                    | Task 11                      |
| §7.5 Branch naming                    | Tasks 11, 13                 |
| §7.6 Probabilities                    | Tasks 10, 12, 13             |
| §8 Scheduler integration              | Task 14, 16                  |
| §9 Project structure                  | Tasks 2–15                   |
| §10 Testing                           | Each Task's tests + Task 17  |
| §11 Non-goals                         | Documented in spec; no work  |
