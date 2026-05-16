# Model Co-Authorship Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make Caveira's fabricator detect AI coding-model identities in the source repo's history, exclude them from the human "player" pool, and instead surface them as `Co-Authored-By:` trailers on fabricated commits — weighted by each player's real-world model usage.

**Architecture:** A model *classifier* (`IsModel`) plus a history *scan* (`ScanModelReport`) produce a `ModelReport` (detected models + per-player usage profiles). `DiscoverIdentities` filters models out of the player pool. A separate post-pass, `InjectCoAuthors`, runs after the fabricator builds the `Plan` and appends `Co-Authored-By:` trailers — keeping the fabricators (`reshapePigs`/`reshapeRats`) untouched.

**Tech Stack:** Go 1.22+, `github.com/go-git/go-git/v5`.

**Reference spec:** `docs/superpowers/specs/2026-05-15-caveira-model-coauthors-design.md`. Work is committed directly to `master`.

---

## File Structure

**New files** (`internal/fabricate/`):
- `model.go` (+ `model_test.go`) — `IsModel(Identity) bool`: recognized-model list + heuristic.
- `modelreport.go` (+ `modelreport_test.go`) — `parseCoAuthors`, the `ModelReport`/`PlayerProfile` types, and `ScanModelReport(repo)`.
- `coauthor.go` (+ `coauthor_test.go`) — `InjectCoAuthors(plan, report, rng)` post-pass + trailer formatting.

**Modified files:**
- `internal/fabricate/identity.go` — `DiscoverIdentities` filters out models.
- `internal/cli/pipeline.go` — `fabricatePipeline` computes the `ModelReport` and runs `InjectCoAuthors` before `WriteToRepo`.

---

## Task 1: Model classifier

**Files:**
- Create: `internal/fabricate/model.go`, `internal/fabricate/model_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/fabricate/model_test.go`:

```go
package fabricate

import "testing"

func TestIsModel(t *testing.T) {
	cases := []struct {
		id   Identity
		want bool
	}{
		{Identity{Name: "Claude Opus 4.7", Email: "noreply@anthropic.com"}, true},
		{Identity{Name: "Codex", Email: "codex@openai.com"}, true},
		{Identity{Name: "Cursor Agent", Email: "agent@example.com"}, true},
		{Identity{Name: "github-actions[bot]", Email: "actions@github.com"}, true},
		{Identity{Name: "Aider", Email: "aider@local"}, true},
		{Identity{Name: "copilot", Email: "copilot@github.com"}, true},
		{Identity{Name: "Alice Cooper", Email: "alice@example.com"}, false},
		{Identity{Name: "Bob", Email: "bob@anthropic.example"}, false},
		{Identity{Name: "", Email: ""}, false},
	}
	for _, c := range cases {
		if got := IsModel(c.id); got != c.want {
			t.Errorf("IsModel(%q <%s>) = %v, want %v", c.id.Name, c.id.Email, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestIsModel -v`
Expected: FAIL — `IsModel` undefined.

- [ ] **Step 3: Implement model.go**

Create `internal/fabricate/model.go`:

```go
package fabricate

import "strings"

// modelEmailExact is the set of email addresses (lowercased) known to belong to
// AI coding agents.
var modelEmailExact = map[string]bool{
	"noreply@anthropic.com": true,
	"copilot@github.com":    true,
}

// modelTokens are case-insensitive substrings that mark an identity as an AI
// coding agent when found anywhere in its name or email.
var modelTokens = []string{
	"claude", "codex", "copilot", "cursor", "aider", "devin", "opencode",
}

// IsModel reports whether an identity belongs to an AI coding agent rather than
// a human. It matches a recognized list of known agent emails, a "[bot]" name
// suffix, and a heuristic set of agent-name tokens.
func IsModel(id Identity) bool {
	name := strings.ToLower(strings.TrimSpace(id.Name))
	email := strings.ToLower(strings.TrimSpace(id.Email))
	if email != "" && modelEmailExact[email] {
		return true
	}
	if strings.HasSuffix(name, "[bot]") {
		return true
	}
	haystack := name + " " + email
	for _, tok := range modelTokens {
		if strings.Contains(haystack, tok) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/fabricate/ -run TestIsModel -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/model.go internal/fabricate/model_test.go
git commit -m "feat(fabricate): IsModel classifier for AI coding agents"
```

---

## Task 2: Co-Authored-By trailer parser

**Files:**
- Create: `internal/fabricate/modelreport.go`, `internal/fabricate/modelreport_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/fabricate/modelreport_test.go`:

```go
package fabricate

import "testing"

func TestParseCoAuthors(t *testing.T) {
	msg := "feat: add thing\n\n" +
		"Some body text.\n\n" +
		"Co-Authored-By: Claude <noreply@anthropic.com>\n" +
		"co-authored-by: Bob Jones <bob@example.com>\n"
	got := parseCoAuthors(msg)
	if len(got) != 2 {
		t.Fatalf("got %d co-authors, want 2: %+v", len(got), got)
	}
	if got[0].Name != "Claude" || got[0].Email != "noreply@anthropic.com" {
		t.Errorf("co-author 0 = %+v", got[0])
	}
	if got[1].Name != "Bob Jones" || got[1].Email != "bob@example.com" {
		t.Errorf("co-author 1 = %+v", got[1])
	}
}

func TestParseCoAuthors_None(t *testing.T) {
	if got := parseCoAuthors("feat: a plain commit\n\nno trailers here"); len(got) != 0 {
		t.Fatalf("expected no co-authors, got %+v", got)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestParseCoAuthors -v`
Expected: FAIL — `parseCoAuthors` undefined.

- [ ] **Step 3: Create modelreport.go with the parser**

Create `internal/fabricate/modelreport.go`:

```go
package fabricate

import "strings"

// coAuthorPrefix is the case-insensitive git trailer key for co-author lines.
const coAuthorPrefix = "co-authored-by:"

// parseCoAuthors extracts the identities named in "Co-Authored-By: Name <email>"
// trailer lines anywhere in a commit message. Lines that are not valid
// identities are skipped.
func parseCoAuthors(message string) []Identity {
	var out []Identity
	for _, line := range strings.Split(message, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < len(coAuthorPrefix) {
			continue
		}
		if !strings.EqualFold(trimmed[:len(coAuthorPrefix)], coAuthorPrefix) {
			continue
		}
		rest := strings.TrimSpace(trimmed[len(coAuthorPrefix):])
		id, err := ParseIdentity(rest)
		if err != nil {
			continue
		}
		out = append(out, id)
	}
	return out
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/fabricate/ -run TestParseCoAuthors -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/modelreport.go internal/fabricate/modelreport_test.go
git commit -m "feat(fabricate): parse Co-Authored-By trailers"
```

---

## Task 3: ModelReport and ScanModelReport

**Files:**
- Modify: `internal/fabricate/modelreport.go`
- Test: `internal/fabricate/modelreport_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/modelreport_test.go`. The `fabricate` package's test files already have an in-memory-repo helper (`newEmptyRepo(t)` in `write_test.go`, which returns a `*git.Repository` backed by memory storage + an in-memory worktree) — reuse it. Add the imports this test needs: `"time"`, `git "github.com/go-git/go-git/v5"`, `"github.com/go-git/go-git/v5/plumbing/object"` (`"testing"` is already present).

```go
// commitAs creates one (empty) commit on wt with the given author, committer,
// and message. Used to build controlled history fixtures.
func commitAs(t *testing.T, wt *git.Worktree, author, committer Identity, msg string) {
	t.Helper()
	when := time.Now()
	_, err := wt.Commit(msg, &git.CommitOptions{
		Author:            &object.Signature{Name: author.Name, Email: author.Email, When: when},
		Committer:         &object.Signature{Name: committer.Name, Email: committer.Email, When: when},
		AllowEmptyCommits: true,
	})
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
}

func TestScanModelReport(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}

	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	bob := Identity{Name: "Bob", Email: "bob@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}

	// Alice: 4 commits, 3 co-authored by Claude.
	commitAs(t, wt, alice, alice, "feat: a1\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a2\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a3\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	commitAs(t, wt, alice, alice, "feat: a4 (solo)")
	// Bob: 2 commits, none with a model.
	commitAs(t, wt, bob, bob, "fix: b1")
	commitAs(t, wt, bob, bob, "fix: b2")

	report, err := ScanModelReport(repo)
	if err != nil {
		t.Fatalf("ScanModelReport: %v", err)
	}

	if len(report.Models) != 1 || report.Models[0].Email != claude.Email {
		t.Fatalf("Models = %+v, want [Claude]", report.Models)
	}
	pa, ok := report.Profiles["alice@example.com"]
	if !ok {
		t.Fatal("no profile for Alice")
	}
	if pa.Rate < 0.74 || pa.Rate > 0.76 { // 3/4
		t.Errorf("Alice Rate = %v, want ~0.75", pa.Rate)
	}
	if mix := pa.Mix["noreply@anthropic.com"]; mix < 0.99 { // 3/3
		t.Errorf("Alice Mix[claude] = %v, want 1.0", mix)
	}
	pb, ok := report.Profiles["bob@example.com"]
	if !ok {
		t.Fatal("no profile for Bob")
	}
	if pb.Rate != 0 || len(pb.Mix) != 0 {
		t.Errorf("Bob profile = %+v, want zero rate / empty mix", pb)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestScanModelReport -v`
Expected: FAIL — `ScanModelReport`, `ModelReport`, `PlayerProfile` undefined.

- [ ] **Step 3: Add the types and ScanModelReport to modelreport.go**

Append to `internal/fabricate/modelreport.go` and add the imports `"sort"`, `git "github.com/go-git/go-git/v5"`, `"github.com/go-git/go-git/v5/plumbing"`, `"github.com/go-git/go-git/v5/plumbing/object"` to its import block (it currently imports only `"strings"`):

```go
// PlayerProfile captures how much one human author used AI coding models.
type PlayerProfile struct {
	// Rate is the fraction of the player's commits that had at least one model.
	Rate float64
	// Mix maps a lowercased model email to the fraction of the player's
	// model-assisted commits in which that model appeared.
	Mix map[string]float64
}

// ModelReport is the result of scanning a repo for AI-model usage.
type ModelReport struct {
	// Models is the set of distinct model identities found anywhere in history,
	// sorted by lowercased email.
	Models []Identity
	// Profiles maps a lowercased human-author email to that player's profile.
	Profiles map[string]PlayerProfile
}

// ScanModelReport walks every reachable commit in repo and builds a ModelReport:
// the set of AI models present in history, and a per-human-author profile of
// how much each used those models (as committers or Co-Authored-By trailers).
func ScanModelReport(repo *git.Repository) (*ModelReport, error) {
	type acc struct {
		total      int
		withModel  int
		modelCount map[string]int // lowercased model email -> count
	}
	players := map[string]*acc{}
	modelsByEmail := map[string]Identity{}

	lc := func(s string) string { return strings.ToLower(strings.TrimSpace(s)) }
	noteModel := func(id Identity) {
		if IsModel(id) {
			modelsByEmail[lc(id.Email)] = id
		}
	}

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

	visited := map[plumbing.Hash]bool{}
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

			author := Identity{Name: cur.Author.Name, Email: cur.Author.Email}
			committer := Identity{Name: cur.Committer.Name, Email: cur.Committer.Email}
			coAuthors := parseCoAuthors(cur.Message)

			noteModel(author)
			noteModel(committer)
			for _, ca := range coAuthors {
				noteModel(ca)
			}

			if !IsModel(author) && lc(author.Email) != "" {
				key := lc(author.Email)
				a := players[key]
				if a == nil {
					a = &acc{modelCount: map[string]int{}}
					players[key] = a
				}
				a.total++
				onCommit := map[string]bool{}
				if IsModel(committer) {
					onCommit[lc(committer.Email)] = true
				}
				for _, ca := range coAuthors {
					if IsModel(ca) {
						onCommit[lc(ca.Email)] = true
					}
				}
				if len(onCommit) > 0 {
					a.withModel++
					for email := range onCommit {
						a.modelCount[email]++
					}
				}
			}

			_ = cur.Parents().ForEach(func(p *object.Commit) error {
				stack = append(stack, p)
				return nil
			})
		}
	}

	report := &ModelReport{Profiles: map[string]PlayerProfile{}}
	for _, m := range modelsByEmail {
		report.Models = append(report.Models, m)
	}
	sort.SliceStable(report.Models, func(i, j int) bool {
		return lc(report.Models[i].Email) < lc(report.Models[j].Email)
	})
	for key, a := range players {
		if a.total == 0 {
			continue
		}
		prof := PlayerProfile{
			Rate: float64(a.withModel) / float64(a.total),
			Mix:  map[string]float64{},
		}
		sum := 0
		for _, n := range a.modelCount {
			sum += n
		}
		if sum > 0 {
			for email, n := range a.modelCount {
				prof.Mix[email] = float64(n) / float64(sum)
			}
		}
		report.Profiles[key] = prof
	}
	return report, nil
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/fabricate/ -run TestScanModelReport -v`
Expected: PASS. Also run `go build ./...` to confirm the new imports resolve.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/modelreport.go internal/fabricate/modelreport_test.go
git commit -m "feat(fabricate): ScanModelReport — detect models and per-player usage"
```

---

## Task 4: DiscoverIdentities filters out models

**Files:**
- Modify: `internal/fabricate/identity.go`
- Test: `internal/fabricate/identity_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/identity_test.go` (it is `package fabricate`). The `commitAs` helper and `newEmptyRepo` helper are defined in other `*_test.go` files of the same package, so they are directly accessible — no extra imports needed for them. Add only what this test body references (`"testing"` is already present).

```go
func TestDiscoverIdentities_ExcludesModels(t *testing.T) {
	repo := newEmptyRepo(t)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatal(err)
	}
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}

	commitAs(t, wt, alice, alice, "feat: human work")
	commitAs(t, wt, claude, claude, "chore: model-authored commit")

	got, err := DiscoverIdentities(repo)
	if err != nil {
		t.Fatalf("DiscoverIdentities: %v", err)
	}
	for _, d := range got {
		if IsModel(d.Identity) {
			t.Fatalf("model %q <%s> leaked into discovered identities", d.Name, d.Email)
		}
	}
	foundAlice := false
	for _, d := range got {
		if d.Email == alice.Email {
			foundAlice = true
		}
	}
	if !foundAlice {
		t.Fatal("expected Alice in discovered identities")
	}
}
```

This test calls only `newEmptyRepo`, `commitAs` (both package-local test helpers), `repo.Worktree()`, `DiscoverIdentities`, and `IsModel` — all in-package. No new imports beyond `"testing"` (already present) are required.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestDiscoverIdentities_ExcludesModels -v`
Expected: FAIL — Claude appears in the discovered identities.

- [ ] **Step 3: Filter models in DiscoverIdentities**

In `internal/fabricate/identity.go`, in `DiscoverIdentities`, the final loop builds `out` from the `counts` map:

```go
	out := make([]DiscoveredIdentity, 0, len(counts))
	for _, d := range counts {
		out = append(out, *d)
	}
```

Change it to skip models:

```go
	out := make([]DiscoveredIdentity, 0, len(counts))
	for _, d := range counts {
		if IsModel(d.Identity) {
			continue // AI coding agents are never offered as players
		}
		out = append(out, *d)
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/fabricate/ -run 'TestDiscoverIdentities' -v`
Expected: PASS, including any pre-existing `DiscoverIdentities` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/identity.go internal/fabricate/identity_test.go
git commit -m "feat(fabricate): exclude AI models from the discovered player pool"
```

---

## Task 5: InjectCoAuthors post-pass

**Files:**
- Create: `internal/fabricate/coauthor.go`, `internal/fabricate/coauthor_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/fabricate/coauthor_test.go`:

```go
package fabricate

import (
	"math/rand"
	"strings"
	"testing"
)

func codeFile(path string) []FileRef { return []FileRef{{Path: path}} }

func TestInjectCoAuthors_AppendsTrailer(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: alice, Message: "feat(walk): add walk", Added: codeFile("internal/walk/load.go")},
	}}
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 1.0, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	if !strings.Contains(plan.Commits[0].Message, "Co-Authored-By: Claude <noreply@anthropic.com>") {
		t.Fatalf("expected co-author trailer, got: %q", plan.Commits[0].Message)
	}
	if !strings.HasPrefix(plan.Commits[0].Message, "feat(walk): add walk") {
		t.Fatalf("original message not preserved: %q", plan.Commits[0].Message)
	}
}

func TestInjectCoAuthors_NoModels_NoOp(t *testing.T) {
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: Identity{Name: "Alice", Email: "alice@example.com"},
			Message: "feat: x", Added: codeFile("a.go")},
	}}
	report := &ModelReport{Profiles: map[string]PlayerProfile{}}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	if plan.Commits[0].Message != "feat: x" {
		t.Fatalf("message changed with no models: %q", plan.Commits[0].Message)
	}
}

func TestInjectCoAuthors_SkipsMergeAndEmpty(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: alice, Message: "Merge branch 'x'", IsMerge: true, Added: codeFile("a.go")},
		{ID: 1, Author: alice, Message: "wip"}, // empty Added
	}}
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 1.0, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	for _, c := range plan.Commits {
		if strings.Contains(c.Message, "Co-Authored-By") {
			t.Fatalf("merge/empty commit got a trailer: %q", c.Message)
		}
	}
}

func TestInjectCoAuthors_ZeroRatePlayerSkipped(t *testing.T) {
	alice := Identity{Name: "Alice", Email: "alice@example.com"}
	claude := Identity{Name: "Claude", Email: "noreply@anthropic.com"}
	plan := &Plan{Commits: []SynthCommit{
		{ID: 0, Author: alice, Message: "feat: x", Added: codeFile("a.go")},
	}}
	report := &ModelReport{
		Models: []Identity{claude},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 0, Mix: map[string]float64{}},
		},
	}
	InjectCoAuthors(plan, report, rand.New(rand.NewSource(1)))
	if strings.Contains(plan.Commits[0].Message, "Co-Authored-By") {
		t.Fatalf("zero-rate player got a trailer: %q", plan.Commits[0].Message)
	}
}

func TestInjectCoAuthors_Deterministic(t *testing.T) {
	build := func() *Plan {
		alice := Identity{Name: "Alice", Email: "alice@example.com"}
		return &Plan{Commits: []SynthCommit{
			{ID: 0, Author: alice, Message: "feat: a", Added: codeFile("a.go")},
			{ID: 1, Author: alice, Message: "feat: b", Added: codeFile("b.go")},
			{ID: 2, Author: alice, Message: "feat: c", Added: codeFile("c.go")},
		}}
	}
	report := &ModelReport{
		Models: []Identity{{Name: "Claude", Email: "noreply@anthropic.com"}},
		Profiles: map[string]PlayerProfile{
			"alice@example.com": {Rate: 0.5, Mix: map[string]float64{"noreply@anthropic.com": 1.0}},
		},
	}
	p1 := build()
	InjectCoAuthors(p1, report, rand.New(rand.NewSource(42)))
	p2 := build()
	InjectCoAuthors(p2, report, rand.New(rand.NewSource(42)))
	for i := range p1.Commits {
		if p1.Commits[i].Message != p2.Commits[i].Message {
			t.Fatalf("commit %d differs across seeded runs", i)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestInjectCoAuthors -v`
Expected: FAIL — `InjectCoAuthors` undefined.

- [ ] **Step 3: Implement coauthor.go**

Create `internal/fabricate/coauthor.go`:

```go
package fabricate

import (
	"math/rand"
	"strings"
)

// Commit-type multipliers applied to a player's base model-co-author rate.
const (
	choreTypeFactor = 1.5 // documentation / chore commits — models show up more
	codeTypeFactor  = 1.0 // code / test commits — base rate
)

// InjectCoAuthors appends a single "Co-Authored-By:" trailer to plan commits,
// driven by the ModelReport. For each non-merge, non-empty commit it looks up
// the author's player profile; with probability min(1, Rate*typeFactor) it
// appends a model co-author chosen weighted by the player's model mix. It is a
// no-op when the report contains no models. All randomness uses rng, so seeded
// runs stay reproducible.
func InjectCoAuthors(plan *Plan, report *ModelReport, rng *rand.Rand) {
	if report == nil || len(report.Models) == 0 {
		return
	}
	for i := range plan.Commits {
		sc := &plan.Commits[i]
		if sc.IsMerge || len(sc.Added) == 0 {
			continue
		}
		prof, ok := report.Profiles[strings.ToLower(strings.TrimSpace(sc.Author.Email))]
		if !ok || prof.Rate <= 0 {
			continue
		}
		factor := codeTypeFactor
		if allChore(sc.Added) {
			factor = choreTypeFactor
		}
		p := prof.Rate * factor
		if p > 1.0 {
			p = 1.0
		}
		if rng.Float64() >= p {
			continue
		}
		model, ok := pickModel(report.Models, prof.Mix, rng)
		if !ok {
			continue
		}
		sc.Message = appendCoAuthor(sc.Message, model)
	}
}

// allChore reports whether every file in files classifies as Chore.
func allChore(files []FileRef) bool {
	for _, f := range files {
		if Classify(f.Path) != Chore {
			return false
		}
	}
	return len(files) > 0
}

// pickModel chooses one model from models, weighted by mix (keyed by lowercased
// model email). Returns false if the player's mix has no positive weight.
func pickModel(models []Identity, mix map[string]float64, rng *rand.Rand) (Identity, bool) {
	total := 0.0
	for _, m := range models {
		total += mix[strings.ToLower(strings.TrimSpace(m.Email))]
	}
	if total <= 0 {
		return Identity{}, false
	}
	r := rng.Float64() * total
	acc := 0.0
	for _, m := range models {
		acc += mix[strings.ToLower(strings.TrimSpace(m.Email))]
		if r < acc {
			return m, true
		}
	}
	return models[len(models)-1], true
}

// appendCoAuthor appends a Co-Authored-By git trailer to a commit message.
func appendCoAuthor(message string, model Identity) string {
	body := strings.TrimRight(message, "\n")
	return body + "\n\nCo-Authored-By: " + model.Name + " <" + model.Email + ">"
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/fabricate/ -run TestInjectCoAuthors -v`
Expected: PASS (all five InjectCoAuthors tests).

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/coauthor.go internal/fabricate/coauthor_test.go
git commit -m "feat(fabricate): InjectCoAuthors post-pass appends model trailers"
```

---

## Task 6: Pipeline integration

**Files:**
- Modify: `internal/cli/pipeline.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/fabricate_integration_test.go` (match the existing helpers in that file — `makeFixtureRepoDir` builds a real on-disk repo; `mustGit` runs git commands; reuse them):

```go
func TestIntegration_Fabricate_NoModels_NoCoAuthors(t *testing.T) {
	src := makeFixtureRepoDir(t) // fixture history has no AI-model identities
	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-30 * 24 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}
	logOut, err := exec.Command("git", "-C", src, "log", "--format=%B").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, logOut)
	}
	if bytes.Contains(logOut, []byte("Co-Authored-By")) {
		t.Fatalf("no models in source, but output has Co-Authored-By trailers:\n%s", logOut)
	}
}
```

This test passes today (it asserts no-regression). The behavior-adding test is the integration test in Task 7. This task's "failing" condition is the next step's build break if `ScanModelReport`/`InjectCoAuthors` are mis-wired — run the existing fabricate integration tests as the gate.

- [ ] **Step 2: Run the existing fabricate tests to confirm the baseline**

Run: `go test ./internal/cli/ -run TestIntegration_Fabricate -v`
Expected: PASS (existing tests + the new no-models test).

- [ ] **Step 3: Wire ScanModelReport + InjectCoAuthors into fabricatePipeline**

In `internal/cli/pipeline.go`, in `fabricatePipeline`, after the identity-resolution block that sets `ids` (right before `rng := rngFor(cfg)`), add the model-report scan:

```go
	modelReport, err := fabricate.ScanModelReport(srcRepo)
	if err != nil {
		fmt.Fprintln(errOut, "error: scan models:", err)
		return 1
	}
```

Then, after the squash-handling block and after the `if cfg.DryRun { ... return 0 }` block — i.e. immediately before `if _, err := os.Stat(stagePath); err == nil {` — add the injection pass:

```go
	fabricate.InjectCoAuthors(plan, modelReport, rng)
```

The `rng` variable is already in scope (created by `rngFor(cfg)` and used by `Generate`/`BuildDurations`). `InjectCoAuthors` consumes it after those, keeping seeded runs deterministic.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/... -v` then `go build ./... && go vet ./...`
Expected: PASS / clean — including `TestIntegration_Fabricate_NoModels_NoCoAuthors`.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/pipeline.go internal/cli/fabricate_integration_test.go
git commit -m "feat(cli): scan models and inject co-author trailers in fabricate"
```

---

## Task 7: End-to-end co-authorship integration test

**Files:**
- Modify: `internal/cli/fabricate_integration_test.go`

- [ ] **Step 1: Write the test**

Add to `internal/cli/fabricate_integration_test.go`. This builds a fixture repo whose history has a human author (Alice) whose commits are all co-authored by Claude, runs the full fabricate pipeline in pigs mode, and asserts: (a) the fabricated output carries `Co-Authored-By: Claude` trailers, and (b) Claude never appears as a commit *author*.

```go
func TestIntegration_Fabricate_ModelBecomesCoAuthor(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not on PATH")
	}
	dir := t.TempDir()
	src := filepath.Join(dir, "repo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", src}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=Alice", "GIT_AUTHOR_EMAIL=alice@example.com",
			"GIT_COMMITTER_NAME=Alice", "GIT_COMMITTER_EMAIL=alice@example.com")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	run("init")
	run("config", "user.name", "Alice")
	run("config", "user.email", "alice@example.com")
	// Several files across two feature dirs so flurry yields multiple commits.
	for _, f := range []string{"README.md", "internal/walk/load.go", "internal/cli/main.go"} {
		p := filepath.Join(src, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("package x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		run("add", f)
		run("commit", "-m", "add "+f+"\n\nCo-Authored-By: Claude <noreply@anthropic.com>")
	}

	cfg := &input.Config{
		Repo:          src,
		Start:         time.Now().Add(-30 * 24 * time.Hour),
		End:           time.Now(),
		WindowTZ:      time.UTC,
		Fabricate:     true,
		PigsN:         1,
		Seed:          7,
		HasSeed:       true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	bodies, err := exec.Command("git", "-C", src, "log", "--all", "--format=%B").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, bodies)
	}
	if !bytes.Contains(bodies, []byte("Co-Authored-By: Claude <noreply@anthropic.com>")) {
		t.Fatalf("expected Claude co-author trailers in fabricated history:\n%s", bodies)
	}

	authors, err := exec.Command("git", "-C", src, "log", "--all", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log authors: %v: %s", err, authors)
	}
	if bytes.Contains(authors, []byte("noreply@anthropic.com")) {
		t.Fatalf("Claude appeared as a commit author; it must only be a co-author:\n%s", authors)
	}
	if !bytes.Contains(authors, []byte("alice@example.com")) {
		t.Fatalf("expected Alice as the fabricated author:\n%s", authors)
	}
}
```

Notes for the engineer: `PigsN: 1` makes the resolver discover exactly one player from history — Alice (Claude is filtered out by Task 4). Every Alice commit in the fixture is co-authored by Claude, so her profile `Rate == 1.0`; with `Rate*typeFactor >= 1.0` every eligible fabricated commit gets a Claude trailer. The window is 30 days wide so the scheduler does not squash. If the fixture is too small and the run still squashes, widen the window further — do not weaken the assertions.

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test ./internal/cli/ -run TestIntegration_Fabricate_ModelBecomesCoAuthor -v`
Expected: PASS. If it fails, investigate — confirm Alice is the resolved player, the report has Claude with Alice `Rate == 1.0`, and the window is wide enough to avoid squashing.

- [ ] **Step 3: Run the full suite**

Run: `go build ./... && go test ./... && go vet ./... && gofmt -l internal/ cmd/`
Expected: all pass, `gofmt` reports nothing.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/fabricate_integration_test.go
git commit -m "test(cli): end-to-end model co-authorship"
```

---

## Final verification

After all tasks, dispatch a final code reviewer for the whole change set, then:

```bash
gofmt -l internal/ cmd/
go build ./... && go test ./... && go vet ./...
```

All must be clean.
