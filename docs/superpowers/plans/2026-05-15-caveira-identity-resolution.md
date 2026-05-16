# Identity Resolution: `.mailmap` + `--pick` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Caveira honor a repository's `.mailmap` (unifying identities that drifted across `user.name`/`user.email` changes) and add a `--pick` flag that always opens an interactive curation step so the user hand-selects players.

**Architecture:** A new `Mailmap` type parses `<repo>/.mailmap` and canonicalizes identities; it is threaded into every function that reads identities from history (`DiscoverIdentities`, `ScanModelReport`). `--pick` adds a `Config.Pick` flag and a variable-count `curateIdentities` picker invoked by `ResolveIdentities`.

**Tech Stack:** Go 1.22+, `github.com/go-git/go-git/v5`.

**Reference spec:** `docs/superpowers/specs/2026-05-15-caveira-identity-resolution-design.md`. Work is committed directly to `master`.

---

## File Structure

**New file:** `internal/fabricate/mailmap.go` (+ `mailmap_test.go`) — `Mailmap` type, `ParseMailmap`, `Canonical`, `LoadMailmap`.

**Modified:**
- `internal/fabricate/identity.go` — `DiscoverIdentities` and `ResolveIdentities` take a `*Mailmap`; `ResolveIdentities` takes a `pick bool`; new `curateIdentities`.
- `internal/fabricate/modelreport.go` — `ScanModelReport` takes a `*Mailmap`.
- `internal/input/config.go` — `Pick bool` field + validation.
- `internal/cli/cli.go` — `--pick` flag.
- `internal/cli/pipeline.go` — load `.mailmap`, thread `*Mailmap` + `cfg.Pick`.

---

## Task 1: Mailmap parsing, canonicalization, and loading

**Files:**
- Create: `internal/fabricate/mailmap.go`, `internal/fabricate/mailmap_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/mailmap_test.go`:

```go
package fabricate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMailmap_NilSafe(t *testing.T) {
	var mm *Mailmap
	id := Identity{Name: "Bob", Email: "bob@example.com"}
	if got := mm.Canonical(id); got != id {
		t.Fatalf("nil Mailmap should pass through, got %+v", got)
	}
}

func TestMailmap_Form1_NameForEmail(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <e@x.com>\n"))
	got := mm.Canonical(Identity{Name: "old name", Email: "e@x.com"})
	if got.Name != "Proper Name" || got.Email != "e@x.com" {
		t.Fatalf("form 1: got %+v", got)
	}
}

func TestMailmap_Form2_EmailRemap(t *testing.T) {
	mm := ParseMailmap([]byte("<proper@x.com> <commit@x.com>\n"))
	got := mm.Canonical(Identity{Name: "Bob", Email: "commit@x.com"})
	// form 2 keeps the commit name, remaps only the email.
	if got.Name != "Bob" || got.Email != "proper@x.com" {
		t.Fatalf("form 2: got %+v", got)
	}
}

func TestMailmap_Form3_NameAndEmailRemap(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <proper@x.com> <commit@x.com>\n"))
	got := mm.Canonical(Identity{Name: "whatever", Email: "commit@x.com"})
	if got.Name != "Proper Name" || got.Email != "proper@x.com" {
		t.Fatalf("form 3: got %+v", got)
	}
	// a commit already under the proper email also gets the proper name.
	got2 := mm.Canonical(Identity{Name: "x", Email: "proper@x.com"})
	if got2.Name != "Proper Name" {
		t.Fatalf("form 3 proper-email name: got %+v", got2)
	}
}

func TestMailmap_Form4_NameSpecific(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <proper@x.com> Commit Name <commit@x.com>\n"))
	hit := mm.Canonical(Identity{Name: "Commit Name", Email: "commit@x.com"})
	if hit.Name != "Proper Name" || hit.Email != "proper@x.com" {
		t.Fatalf("form 4 match: got %+v", hit)
	}
	// a different name on the same commit email must NOT be remapped.
	miss := mm.Canonical(Identity{Name: "Someone Else", Email: "commit@x.com"})
	if miss.Name != "Someone Else" || miss.Email != "commit@x.com" {
		t.Fatalf("form 4 non-match should pass through: got %+v", miss)
	}
}

func TestMailmap_CommentsAndCaseInsensitive(t *testing.T) {
	mm := ParseMailmap([]byte("# a comment\n\nProper Name <proper@x.com> <Commit@X.com>\n"))
	got := mm.Canonical(Identity{Name: "x", Email: "commit@x.COM"})
	if got.Email != "proper@x.com" {
		t.Fatalf("case-insensitive email match failed: got %+v", got)
	}
}

func TestMailmap_Unmapped(t *testing.T) {
	mm := ParseMailmap([]byte("Proper Name <proper@x.com> <commit@x.com>\n"))
	id := Identity{Name: "Stranger", Email: "stranger@elsewhere.com"}
	if got := mm.Canonical(id); got != id {
		t.Fatalf("unmapped identity should pass through, got %+v", got)
	}
}

func TestLoadMailmap(t *testing.T) {
	dir := t.TempDir()
	// Absent file -> nil, no error.
	mm, err := LoadMailmap(dir)
	if err != nil {
		t.Fatalf("LoadMailmap absent: %v", err)
	}
	if mm.Canonical(Identity{Name: "a", Email: "b@c"}) != (Identity{Name: "a", Email: "b@c"}) {
		t.Fatal("absent mailmap should be a passthrough")
	}
	// Present file -> parsed.
	if err := os.WriteFile(filepath.Join(dir, ".mailmap"),
		[]byte("Proper <proper@x.com> <commit@x.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mm, err = LoadMailmap(dir)
	if err != nil {
		t.Fatalf("LoadMailmap present: %v", err)
	}
	if got := mm.Canonical(Identity{Name: "x", Email: "commit@x.com"}); got.Email != "proper@x.com" {
		t.Fatalf("loaded mailmap not applied: got %+v", got)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/ -run 'TestMailmap|TestLoadMailmap' -v`
Expected: FAIL — `Mailmap`, `ParseMailmap`, `LoadMailmap` undefined.

- [ ] **Step 3: Implement mailmap.go**

Create `internal/fabricate/mailmap.go`:

```go
package fabricate

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// mmEntry is one commit-email -> canonical mapping from a .mailmap line.
type mmEntry struct {
	commitName  string // lowercased; "" matches any commit name
	properName  string // "" = no name override
	properEmail string // canonical email
}

// Mailmap canonicalizes git identities per a repository's .mailmap file.
// The zero value and a nil *Mailmap are valid passthrough (no-op) maps.
type Mailmap struct {
	byCommitEmail map[string][]mmEntry // lowercased commit email -> entries
	nameByEmail   map[string]string    // lowercased email -> canonical name
}

var mailmapAngleRe = regexp.MustCompile(`<([^<>]*)>`)

// ParseMailmap parses .mailmap content into a Mailmap. It handles the four
// standard line forms and `#` comments; unparseable lines are skipped.
func ParseMailmap(content []byte) *Mailmap {
	mm := &Mailmap{
		byCommitEmail: map[string][]mmEntry{},
		nameByEmail:   map[string]string{},
	}
	for _, raw := range strings.Split(string(content), "\n") {
		line := raw
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		locs := mailmapAngleRe.FindAllStringSubmatchIndex(line, -1)
		switch {
		case len(locs) == 1:
			// Form 1: Proper Name <proper@email>
			loc := locs[0]
			email := strings.TrimSpace(line[loc[2]:loc[3]])
			name := strings.TrimSpace(line[:loc[0]])
			if email != "" && name != "" {
				mm.nameByEmail[strings.ToLower(email)] = name
			}
		case len(locs) >= 2:
			// Forms 2/3/4: [Proper Name] <proper@email> [Commit Name] <commit@email>
			l1, l2 := locs[0], locs[1]
			properEmail := strings.TrimSpace(line[l1[2]:l1[3]])
			commitEmail := strings.TrimSpace(line[l2[2]:l2[3]])
			properName := strings.TrimSpace(line[:l1[0]])
			commitName := strings.TrimSpace(line[l1[1]:l2[0]])
			if properEmail == "" || commitEmail == "" {
				continue
			}
			key := strings.ToLower(commitEmail)
			mm.byCommitEmail[key] = append(mm.byCommitEmail[key], mmEntry{
				commitName:  strings.ToLower(commitName),
				properName:  properName,
				properEmail: properEmail,
			})
			if properName != "" {
				mm.nameByEmail[strings.ToLower(properEmail)] = properName
			}
		}
	}
	return mm
}

// Canonical returns the canonical identity for id per the mailmap. A nil
// *Mailmap returns id unchanged.
func (mm *Mailmap) Canonical(id Identity) Identity {
	if mm == nil {
		return id
	}
	lcEmail := strings.ToLower(strings.TrimSpace(id.Email))
	lcName := strings.ToLower(strings.TrimSpace(id.Name))

	if entries := mm.byCommitEmail[lcEmail]; len(entries) > 0 {
		var best *mmEntry
		for i := range entries {
			if entries[i].commitName != "" && entries[i].commitName == lcName {
				best = &entries[i]
				break
			}
		}
		if best == nil {
			for i := range entries {
				if entries[i].commitName == "" {
					best = &entries[i]
					break
				}
			}
		}
		if best != nil {
			name := best.properName
			if name == "" {
				if n := mm.nameByEmail[strings.ToLower(best.properEmail)]; n != "" {
					name = n
				} else {
					name = id.Name
				}
			}
			return Identity{Name: name, Email: best.properEmail}
		}
	}
	if n := mm.nameByEmail[lcEmail]; n != "" {
		return Identity{Name: n, Email: id.Email}
	}
	return id
}

// LoadMailmap reads and parses <repoPath>/.mailmap. An absent file yields a
// nil *Mailmap (a valid passthrough); a read error other than not-exist is
// returned.
func LoadMailmap(repoPath string) (*Mailmap, error) {
	content, err := os.ReadFile(filepath.Join(repoPath, ".mailmap"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return ParseMailmap(content), nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run 'TestMailmap|TestLoadMailmap' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/mailmap.go internal/fabricate/mailmap_test.go
git commit -m "feat(fabricate): .mailmap parsing and identity canonicalization"
```

---

## Task 2: Thread `*Mailmap` through identity resolution

**Files:**
- Modify: `internal/fabricate/identity.go`, `internal/fabricate/modelreport.go`, `internal/cli/pipeline.go`
- Test: `internal/fabricate/identity_test.go`, `internal/fabricate/modelreport_test.go`, and update existing callers in all `*_test.go` files

This task changes three function signatures, so it must update every caller in the same commit to keep the build green.

- [ ] **Step 1: Write the failing tests**

Add to `internal/fabricate/identity_test.go`:

```go
func TestDiscoverIdentities_MailmapUnifies(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	a1 := Identity{Name: "Jay", Email: "jay@personal.com"}
	a2 := Identity{Name: "jay06", Email: "jay@work.com"}
	commitAs(t, wt, a1, a1, "feat: one")
	commitAs(t, wt, a2, a2, "feat: two")

	mm := ParseMailmap([]byte("Jay <jay@personal.com> <jay@work.com>\n"))
	got, err := DiscoverIdentities(repo, mm)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 unified identity, got %d: %+v", len(got), got)
	}
	if got[0].Email != "jay@personal.com" || got[0].Commits != 2 {
		t.Fatalf("unified identity wrong: %+v", got[0])
	}
}
```

Add to `internal/fabricate/modelreport_test.go`:

```go
func TestScanModelReport_MailmapMergesProfile(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	a1 := Identity{Name: "Jay", Email: "jay@personal.com"}
	a2 := Identity{Name: "jay06", Email: "jay@work.com"}
	// Under each email, one commit; one of them co-authored by Claude.
	commitAs(t, wt, a1, a1, "feat: p1\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, a2, a2, "feat: w1")

	mm := ParseMailmap([]byte("Jay <jay@personal.com> <jay@work.com>\n"))
	report, err := ScanModelReport(repo, mm)
	if err != nil {
		t.Fatalf("ScanModelReport: %v", err)
	}
	if len(report.Profiles) != 1 {
		t.Fatalf("expected 1 merged profile, got %d: %+v", len(report.Profiles), report.Profiles)
	}
	prof, ok := report.Profiles["jay@personal.com"]
	if !ok {
		t.Fatalf("no profile under canonical email: %+v", report.Profiles)
	}
	if prof.Rate < 0.49 || prof.Rate > 0.51 { // 1 of 2 merged commits
		t.Fatalf("merged Rate = %v, want ~0.5", prof.Rate)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/ -run 'MailmapUnifies|MailmapMergesProfile' -v`
Expected: FAIL — `DiscoverIdentities` / `ScanModelReport` do not take a `*Mailmap` argument.

- [ ] **Step 3: Update `DiscoverIdentities`**

In `internal/fabricate/identity.go`, change `DiscoverIdentities` to take a `*Mailmap` and canonicalize each author identity before keying:

```go
// DiscoverIdentities scans every reachable commit in repo and returns the
// unique author identities (keyed by lowercased email, canonicalized through
// mm), sorted by commit count descending then by name ascending. Models are
// excluded. A nil mm applies no canonicalization.
func DiscoverIdentities(repo *git.Repository, mm *Mailmap) ([]DiscoveredIdentity, error) {
	counts := map[string]*DiscoveredIdentity{}

	err := walkCommits(repo, func(cur *object.Commit) {
		id := mm.Canonical(Identity{Name: cur.Author.Name, Email: cur.Author.Email})
		key := strings.ToLower(strings.TrimSpace(id.Email))
		if key == "" {
			return
		}
		d, ok := counts[key]
		if !ok {
			d = &DiscoveredIdentity{Identity: id}
			counts[key] = d
		}
		d.Commits++
	})
	if err != nil {
		return nil, err
	}

	out := make([]DiscoveredIdentity, 0, len(counts))
	for _, d := range counts {
		if IsModel(d.Identity) {
			continue // AI coding agents are never offered as players
		}
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

- [ ] **Step 4: Update `ResolveIdentities` to take and forward `*Mailmap`**

In `internal/fabricate/identity.go`, change `ResolveIdentities`'s signature to add `mm *Mailmap` (after `n int`), and change its internal `DiscoverIdentities(repo)` call to `DiscoverIdentities(repo, mm)`:

```go
func ResolveIdentities(repo *git.Repository, flagIDs []string, n int, mm *Mailmap, stdin io.Reader, stdout io.Writer) ([]Identity, error) {
```

and inside, the discovery call becomes:

```go
	discovered, err := DiscoverIdentities(repo, mm)
```

Leave the rest of `ResolveIdentities` unchanged in this task.

- [ ] **Step 5: Update `ScanModelReport` to take and apply `*Mailmap`**

In `internal/fabricate/modelreport.go`, change `ScanModelReport` to take `mm *Mailmap` and canonicalize the author, committer, and co-author identities at the top of the walk closure:

```go
func ScanModelReport(repo *git.Repository, mm *Mailmap) (*ModelReport, error) {
```

and inside the `walkCommits` closure, replace the first three lines that build `author`, `committer`, `coAuthors` with canonicalized versions:

```go
		author := mm.Canonical(Identity{Name: cur.Author.Name, Email: cur.Author.Email})
		committer := mm.Canonical(Identity{Name: cur.Committer.Name, Email: cur.Committer.Email})
		var coAuthors []Identity
		for _, ca := range parseCoAuthors(cur.Message) {
			coAuthors = append(coAuthors, mm.Canonical(ca))
		}
```

The rest of `ScanModelReport` is unchanged — it already keys by lowercased email, which now receives canonical emails.

- [ ] **Step 6: Update all callers**

The signature changes break every caller. Update them:

- `internal/cli/pipeline.go` — in `fabricatePipeline`, load the mailmap once near the top (after `srcRepo` is available, before the identity-resolution block) and pass it to both functions:

```go
	mailmap, err := fabricate.LoadMailmap(srcPath)
	if err != nil {
		fmt.Fprintln(errOut, "error: read .mailmap:", err)
		return 1
	}
```

  Then change the `ResolveIdentities` call to pass `mailmap` (after `nIDs`):
  `fabricate.ResolveIdentities(srcRepo, rawIDs, nIDs, mailmap, os.Stdin, out)`.
  And change the `ScanModelReport` call: `fabricate.ScanModelReport(srcRepo, mailmap)`.

- **All existing test callers** in `internal/fabricate/*_test.go` and `internal/cli/*_test.go` that call `DiscoverIdentities`, `ResolveIdentities`, or `ScanModelReport` must pass `nil` for the new `*Mailmap` parameter. Grep for each function name across the test files and update every call site (e.g. `DiscoverIdentities(repo)` → `DiscoverIdentities(repo, nil)`, `ScanModelReport(repo)` → `ScanModelReport(repo, nil)`, `ResolveIdentities(repo, ids, n, stdin, stdout)` → `ResolveIdentities(repo, ids, n, nil, stdin, stdout)`). The new tests from Step 1 pass a real `*Mailmap`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go build ./... && go test ./... -v 2>&1 | tail -30`
Expected: build clean; all packages pass, including the two new mailmap tests and every pre-existing identity/model test (now passing `nil`).

- [ ] **Step 8: Commit**

```bash
git add internal/fabricate/identity.go internal/fabricate/modelreport.go internal/cli/pipeline.go internal/fabricate/identity_test.go internal/fabricate/modelreport_test.go internal/cli/cli_test.go internal/cli/fabricate_integration_test.go
git commit -m "feat(fabricate): apply .mailmap when resolving identities and scanning models"
```

(Stage whichever test files you actually had to touch.)

---

## Task 3: `--pick` config field and validation

**Files:**
- Modify: `internal/input/config.go`
- Test: `internal/input/config_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/input/config_test.go`:

```go
func TestValidate_PickRequiresPigsOrRats(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Pick = true
	if err := c.Validate(); err == nil {
		t.Fatal("expected --pick without --pigs/--rats to be rejected")
	}

	c.RatsN = 3
	if err := c.Validate(); err != nil {
		t.Fatalf("--pick with --rats should validate, got: %v", err)
	}
}

func TestValidate_PickRequiresFabricate(t *testing.T) {
	c := baseValidConfig()
	c.Pick = true
	c.RatsN = 3
	if err := c.Validate(); err == nil {
		t.Fatal("expected --pick without --fabricate to be rejected")
	}
}
```

These tests use a `baseValidConfig()` helper. Check whether `config_test.go` already defines one; if not, add it (and ensure the file imports `"time"`):

```go
func baseValidConfig() *Config {
	return &Config{
		Repo:     "/tmp/x",
		Start:    time.Date(2026, 5, 14, 13, 0, 0, 0, time.UTC),
		End:      time.Date(2026, 5, 14, 17, 0, 0, 0, time.UTC),
		WindowTZ: time.UTC,
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/input/ -run TestValidate_Pick -v`
Expected: FAIL — `Config` has no `Pick` field.

- [ ] **Step 3: Add the field and validation**

In `internal/input/config.go`, add a `Pick bool` field to the `Config` struct (alongside the other fabricate-mode fields such as `PigsN`/`RatsN`):

```go
	Pick bool // --pick: always open the interactive player picker
```

In `Validate()`, add — after the existing `--pigs`/`--rats`/`--pig`/`--rat` checks:

```go
	if c.Pick && !c.Fabricate {
		return errors.New("--pick requires --fabricate")
	}
	if c.Pick && c.PigsN == 0 && c.RatsN == 0 {
		return errors.New("--pick requires --pigs N or --rats N")
	}
```

Also include `c.Pick` in the existing `fabFlagsUsed` expression (the check that flags requiring `--fabricate` were not used without it), so `--pick` alone is also caught by that path — though the explicit check above is the primary guard. (If updating `fabFlagsUsed` is awkward, the two explicit checks above are sufficient; use judgement.)

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/input/ -v`
Expected: PASS (all input tests).

- [ ] **Step 5: Commit**

```bash
git add internal/input/config.go internal/input/config_test.go
git commit -m "feat(input): --pick config field and validation"
```

---

## Task 4: `--pick` CLI flag

**Files:**
- Modify: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/cli_test.go`:

```go
func TestRunPickFlagParses(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent-pick",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--rats", "3", "--pick",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit (missing repo), got 0; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("--pick requires")) {
		t.Fatalf("--pick with --fabricate --rats should pass validation; stderr=%q", errOut.String())
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunPickFlagParses -v`
Expected: FAIL — `--pick` is an unknown flag.

- [ ] **Step 3: Register the flag**

In `internal/cli/cli.go`, in `newRootCmd`, add a flag variable to the `var (...)` block:

```go
		pickFlag bool
```

Register it alongside the other fabricate flags:

```go
	cmd.Flags().BoolVar(&pickFlag, "pick", false, "always open the interactive player picker (requires --pigs/--rats)")
```

And add `Pick: pickFlag,` to the `input.Config{...}` struct literal in `RunE`.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestRun' -v`
Expected: PASS (the new test and all existing CLI tests).

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat(cli): --pick flag"
```

---

## Task 5: `curateIdentities` and the `ResolveIdentities` pick path

**Files:**
- Modify: `internal/fabricate/identity.go`, `internal/cli/pipeline.go`
- Test: `internal/fabricate/identity_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/fabricate/identity_test.go`:

```go
func TestCurateIdentities_SubsetAndEmpty(t *testing.T) {
	found := []DiscoveredIdentity{
		{Identity: Identity{Name: "A", Email: "a@x"}, Commits: 5},
		{Identity: Identity{Name: "B", Email: "b@x"}, Commits: 3},
		{Identity: Identity{Name: "C", Email: "c@x"}, Commits: 1},
	}
	// Pick a subset of 2 from 3.
	got, err := curateIdentities(found, 3, strings.NewReader("1,3\n"), io.Discard, 3, 0)
	if err != nil {
		t.Fatalf("curate subset: %v", err)
	}
	if len(got) != 2 || got[0].Email != "a@x" || got[1].Email != "c@x" {
		t.Fatalf("curate subset got %+v", got)
	}
	// Empty selection -> zero identities, no error.
	got, err = curateIdentities(found, 3, strings.NewReader("\n"), io.Discard, 3, 0)
	if err != nil {
		t.Fatalf("curate empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("curate empty got %+v", got)
	}
}

func TestCurateIdentities_RejectsOverAndOutOfRange(t *testing.T) {
	found := []DiscoveredIdentity{
		{Identity: Identity{Name: "A", Email: "a@x"}, Commits: 1},
		{Identity: Identity{Name: "B", Email: "b@x"}, Commits: 1},
	}
	if _, err := curateIdentities(found, 1, strings.NewReader("1,2\n"), io.Discard, 1, 0); err == nil {
		t.Fatal("expected error selecting more than max")
	}
	if _, err := curateIdentities(found, 2, strings.NewReader("9\n"), io.Discard, 2, 0); err == nil {
		t.Fatal("expected error for out-of-range index")
	}
}

func TestResolveIdentities_PickPathPromptsShortfall(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	alice := Identity{Name: "Alice", Email: "alice@x"}
	commitAs(t, wt, alice, alice, "feat: a")

	// --rats 2, pick mode: select the 1 discovered (Alice), then prompt 1 more.
	stdin := strings.NewReader("1\nBob\nbob@x\n")
	got, err := ResolveIdentities(repo, nil, 2, nil, true, stdin, io.Discard)
	if err != nil {
		t.Fatalf("ResolveIdentities pick: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 identities, got %+v", got)
	}
	if got[0].Email != "alice@x" || got[1].Email != "bob@x" {
		t.Fatalf("pick-path identities wrong: %+v", got)
	}
}
```

Ensure `identity_test.go` imports `"io"` and `"strings"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/ -run 'TestCurateIdentities|TestResolveIdentities_PickPath' -v`
Expected: FAIL — `curateIdentities` undefined; `ResolveIdentities` does not take a `pick bool`.

- [ ] **Step 3: Add `curateIdentities`**

In `internal/fabricate/identity.go`, add:

```go
// curateIdentities shows found and reads a comma-separated selection of 0..max
// entries by 1-based index. An empty line selects none. Returns the chosen
// identities. Errors on out-of-range indices, duplicates, or more than max.
func curateIdentities(found []DiscoveredIdentity, max int, stdin io.Reader, stdout io.Writer, total, alreadyHave int) ([]Identity, error) {
	fmt.Fprintf(stdout, "Caveira needs %d identities. %d supplied via flag. Found %d in .git:\n", total, alreadyHave, len(found))
	for i, d := range found {
		fmt.Fprintf(stdout, "  [%d] %s <%s>     (%d commits)\n", i+1, d.Name, d.Email, d.Commits)
	}
	fmt.Fprintf(stdout, "Pick up to %d (comma-separated, e.g. `1,3`; empty to pick none): ", max)

	reader := bufio.NewReader(stdin)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, nil
	}
	parts := strings.Split(line, ",")
	if len(parts) > max {
		return nil, fmt.Errorf("picked %d but only %d slot(s) available", len(parts), max)
	}
	seen := map[int]bool{}
	out := make([]Identity, 0, len(parts))
	for _, p := range parts {
		var idx int
		if _, err := fmt.Sscanf(strings.TrimSpace(p), "%d", &idx); err != nil {
			return nil, fmt.Errorf("invalid pick %q", p)
		}
		if idx < 1 || idx > len(found) {
			return nil, fmt.Errorf("pick %d out of range", idx)
		}
		if seen[idx] {
			return nil, fmt.Errorf("duplicate pick %d", idx)
		}
		seen[idx] = true
		out = append(out, found[idx-1].Identity)
	}
	return out, nil
}
```

- [ ] **Step 4: Add the `pick` parameter and pick path to `ResolveIdentities`**

In `internal/fabricate/identity.go`, change `ResolveIdentities`'s signature to add `pick bool` (after `mm *Mailmap`):

```go
func ResolveIdentities(repo *git.Repository, flagIDs []string, n int, mm *Mailmap, pick bool, stdin io.Reader, stdout io.Writer) ([]Identity, error) {
```

After the flag-parsing block (after the `if len(out) == n { return out, nil }` early return and the `remaining`/`supplied`/`discovered`/`fresh` setup), insert the pick branch *before* the existing `switch`:

```go
	if pick {
		picked, err := curateIdentities(fresh, remaining, stdin, stdout, n, len(out))
		if err != nil {
			return nil, err
		}
		out = append(out, picked...)
		if len(out) < n {
			prompted, err := promptIdentities(n-len(out), stdin, stdout)
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
```

The existing `switch` and the rest stay as the non-pick path.

- [ ] **Step 5: Update the `pipeline.go` caller**

In `internal/cli/pipeline.go`, change the `ResolveIdentities` call (updated in Task 2 to include `mailmap`) to also pass `cfg.Pick`:

```go
		resolved, err := fabricate.ResolveIdentities(srcRepo, rawIDs, nIDs, mailmap, cfg.Pick, os.Stdin, out)
```

- [ ] **Step 6: Update existing `ResolveIdentities` test callers**

Any `*_test.go` calling `ResolveIdentities` (from earlier work — e.g. picker tests) must add `false` for the new `pick` parameter, after the `*Mailmap` argument. Grep `ResolveIdentities(` across test files and update each call.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go build ./... && go test ./... 2>&1 | tail -30`
Expected: build clean; all packages pass.

- [ ] **Step 8: Commit**

```bash
git add internal/fabricate/identity.go internal/cli/pipeline.go internal/fabricate/identity_test.go
git commit -m "feat(fabricate): --pick interactive player curation"
```

(Stage any other test files whose `ResolveIdentities` calls you updated.)

---

## Task 6: End-to-end integration test

**Files:**
- Modify: `internal/cli/fabricate_integration_test.go`

- [ ] **Step 1: Write the test**

Add to `internal/cli/fabricate_integration_test.go`. This builds a repo where one person committed under two emails, adds a `.mailmap` unifying them, and runs the full fabricate pipeline — asserting the unified identity is the sole fabricated author.

```go
func TestIntegration_Fabricate_MailmapUnifies(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "repo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	commit := func(name, email, file string) {
		t.Helper()
		p := filepath.Join(src, file)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, args := range [][]string{{"add", file}, {"commit", "-m", "add " + file}} {
			cmd := exec.Command("git", append([]string{"-C", src}, args...)...)
			cmd.Env = append(os.Environ(),
				"GIT_AUTHOR_NAME="+name, "GIT_AUTHOR_EMAIL="+email,
				"GIT_COMMITTER_NAME="+name, "GIT_COMMITTER_EMAIL="+email)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v: %s", args, err, out)
			}
		}
	}
	initCmd := exec.Command("git", "-C", src, "init")
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// Same person, two emails, across two feature dirs.
	commit("Jay", "jay@personal.com", "internal/walk/load.go")
	commit("jay06", "jay@work.com", "internal/cli/main.go")
	// .mailmap unifies the two emails into one identity.
	if err := os.WriteFile(filepath.Join(src, ".mailmap"),
		[]byte("Jay <jay@personal.com> <jay@work.com>\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &input.Config{
		Repo:          src,
		Start:         time.Now().Add(-30 * 24 * time.Hour),
		End:           time.Now(),
		WindowTZ:      time.UTC,
		Fabricate:     true,
		RatsN:         1,
		Seed:          5,
		HasSeed:       true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	emails, err := exec.Command("git", "-C", src, "log", "--all", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, emails)
	}
	if bytes.Contains(emails, []byte("jay@work.com")) {
		t.Fatalf("non-canonical email leaked into fabricated history:\n%s", emails)
	}
	if !bytes.Contains(emails, []byte("jay@personal.com")) {
		t.Fatalf("expected the canonical email as the fabricated author:\n%s", emails)
	}
}
```

`RatsN: 1` with a `.mailmap`-unified single discovered identity means the resolver finds exactly one identity (the unified Jay) and uses it without prompting. The 30-day window is wide enough to avoid squashing.

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test ./internal/cli/ -run TestIntegration_Fabricate_MailmapUnifies -v`
Expected: PASS. If the resolver prompts (test hangs / fails on stdin), the `.mailmap` is not collapsing the two emails — investigate `LoadMailmap`/`DiscoverIdentities` wiring; do not weaken the assertions.

- [ ] **Step 3: Run the full suite**

Run: `go build ./... && go test ./... && go vet ./... && gofmt -l internal/ cmd/`
Expected: all pass, `gofmt` reports nothing.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/fabricate_integration_test.go
git commit -m "test(cli): end-to-end .mailmap identity unification"
```

---

## Final verification

After all tasks, dispatch a final code reviewer for the whole change set, then:

```bash
gofmt -l internal/ cmd/
go build ./... && go test ./... && go vet ./...
```

All must be clean.
