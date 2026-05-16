# Random Author Assignment + `--earned` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace round-robin commit-to-player assignment in the fabricators with a seeded random draw, and add an `--earned` flag that weights the draw by each player's real commit count in the source history.

**Architecture:** A `pickAuthor` helper does a uniform or weight-biased random draw using the existing seeded RNG. The reshapers call it instead of round-robin index math. An optional `weights []int` (parallel to `ids`, `nil` = uniform) threads through `Generate`/`BuildPigsPlan`/`BuildRatsPlan` into the reshapers; the pipeline computes it via `EarnedWeights` only when `--earned` is set.

**Tech Stack:** Go 1.22+, `github.com/go-git/go-git/v5`.

**Reference spec:** `docs/superpowers/specs/2026-05-15-caveira-author-assignment-design.md`. Work is committed directly to `master`.

---

## File Structure

**New file:** `internal/fabricate/author.go` (+ `author_test.go`) — `pickAuthor` (the draw) and, added later, `EarnedWeights` (weight computation).

**Modified:**
- `internal/fabricate/pigs.go` — `reshapePigs`/`BuildPigsPlan` use `pickAuthor` and take `weights`.
- `internal/fabricate/rats.go` — `reshapeRats`/`BuildRatsPlan` use `pickAuthor` and take `weights`.
- `internal/fabricate/generate.go` — `Generate` takes and forwards `weights`.
- `internal/input/config.go` — `Earned bool` field + validation.
- `internal/cli/cli.go` — `--earned` flag.
- `internal/cli/pipeline.go` — compute `weights` when `--earned`; thread into `Generate`.

---

## Task 1: `pickAuthor` helper

**Files:**
- Create: `internal/fabricate/author.go`, `internal/fabricate/author_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/author_test.go`:

```go
package fabricate

import (
	"math/rand"
	"testing"
)

func TestPickAuthor_UniformCoversAll(t *testing.T) {
	ids := []Identity{
		{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}, {Name: "C", Email: "c@x"},
	}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 3000; i++ {
		seen[pickAuthor(ids, nil, rng).Email]++
	}
	for _, id := range ids {
		if seen[id.Email] == 0 {
			t.Fatalf("uniform draw never picked %s", id.Email)
		}
	}
}

func TestPickAuthor_Weighted(t *testing.T) {
	ids := []Identity{{Name: "Heavy", Email: "h@x"}, {Name: "Light", Email: "l@x"}}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 5000; i++ {
		seen[pickAuthor(ids, []int{9, 1}, rng).Email]++
	}
	// Heavy (weight 9) should dominate Light (weight 1) by a wide margin.
	if seen["h@x"] <= seen["l@x"] {
		t.Fatalf("weighted draw not skewed: h=%d l=%d", seen["h@x"], seen["l@x"])
	}
	if seen["h@x"] < 3500 || seen["l@x"] == 0 {
		t.Fatalf("weighted distribution off: h=%d l=%d (want h~4500, l~500)", seen["h@x"], seen["l@x"])
	}
}

func TestPickAuthor_AllZeroWeightsUniform(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 2000; i++ {
		seen[pickAuthor(ids, []int{0, 0}, rng).Email]++
	}
	if seen["a@x"] == 0 || seen["b@x"] == 0 {
		t.Fatalf("all-zero weights should fall back to uniform: %+v", seen)
	}
}

func TestPickAuthor_MismatchedWeightsUniform(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}}
	rng := rand.New(rand.NewSource(1))
	seen := map[string]int{}
	for i := 0; i < 2000; i++ {
		// weights length != ids length -> ignored, uniform.
		seen[pickAuthor(ids, []int{5}, rng).Email]++
	}
	if seen["a@x"] == 0 || seen["b@x"] == 0 {
		t.Fatalf("mismatched-length weights should fall back to uniform: %+v", seen)
	}
}

func TestPickAuthor_SingleIdentity(t *testing.T) {
	ids := []Identity{{Name: "Only", Email: "only@x"}}
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 10; i++ {
		if pickAuthor(ids, nil, rng).Email != "only@x" {
			t.Fatal("single-identity pick must always return that identity")
		}
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/ -run TestPickAuthor -v`
Expected: FAIL — `pickAuthor` undefined.

- [ ] **Step 3: Implement author.go**

Create `internal/fabricate/author.go`:

```go
package fabricate

import "math/rand"

// pickAuthor selects one player from ids using rng. With a nil, wrong-length,
// or all-non-positive weights slice it draws uniformly. Otherwise it draws
// weighted: weights[i] is the (parallel) weight of ids[i]; non-positive weights
// are treated as zero. ids must be non-empty.
func pickAuthor(ids []Identity, weights []int, rng *rand.Rand) Identity {
	n := len(ids)
	total := 0
	if len(weights) == n {
		for _, w := range weights {
			if w > 0 {
				total += w
			}
		}
	}
	if total <= 0 {
		return ids[rng.Intn(n)]
	}
	r := rng.Intn(total)
	acc := 0
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		acc += w
		if r < acc {
			return ids[i]
		}
	}
	return ids[n-1]
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run TestPickAuthor -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/author.go internal/fabricate/author_test.go
git commit -m "feat(fabricate): pickAuthor uniform/weighted player draw"
```

## Before You Begin (Task 1)

Confirm none of `pickAuthor` already exists in the `internal/fabricate` package. The plan provides complete code. Run `go build ./...`, `go vet ./...`, `go test ./internal/fabricate/...`.

---

## Task 2: Reshapers use uniform random draws

**Files:**
- Modify: `internal/fabricate/pigs.go`, `internal/fabricate/rats.go`
- Test: `internal/fabricate/pigs_test.go`, `internal/fabricate/rats_test.go`

This task switches both reshapers from round-robin to a uniform random draw via `pickAuthor(ids, nil, rng)`. No function signatures change (the reshapers already hold `rng`). This is a *behavior change*: the existing `TestPigsMode_TwoAuthors_RoundRobin` test (which asserts the round-robin pattern) going red is the TDD red signal — the old test is then deleted and replaced.

- [ ] **Step 1: Confirm the baseline**

Run: `go test ./internal/fabricate/ -run 'Pigs|Rats' -v`
Expected: PASS — in particular `TestPigsMode_TwoAuthors_RoundRobin` is green, asserting the round-robin pattern that this task removes.

- [ ] **Step 2: Switch `reshapePigs` to a random draw**

In `internal/fabricate/pigs.go`, in `reshapePigs`, the author-assignment loop currently is:

```go
	for i := range base {
		id := ids[i%len(ids)]
		base[i].Author = id
		base[i].Committer = id
	}
```

Change it to a random draw:

```go
	for i := range base {
		id := pickAuthor(ids, nil, rng)
		base[i].Author = id
		base[i].Committer = id
	}
```

And in the noise-injection block, the noise commit currently does:

```go
			noise := SynthCommit{
				Author:    ids[rng.Intn(len(ids))],
				Committer: ids[rng.Intn(len(ids))],
				Message:   ApplyTypos(noiseMessages[rng.Intn(len(noiseMessages))], rng),
			}
```

Change the author/committer draws to `pickAuthor` for consistency:

```go
			noise := SynthCommit{
				Author:    pickAuthor(ids, nil, rng),
				Committer: pickAuthor(ids, nil, rng),
				Message:   ApplyTypos(noiseMessages[rng.Intn(len(noiseMessages))], rng),
			}
```

- [ ] **Step 3: Switch `reshapeRats` to a random draw**

In `internal/fabricate/rats.go`, in `reshapeRats`:

The chore-commit loop currently assigns `ids[0]`:

```go
	for _, cc := range choreCommits {
		id := next()
		cc.ID = id
		cc.Author = ids[0]
		cc.Committer = ids[0]
```

Change to a random draw:

```go
	for _, cc := range choreCommits {
		id := next()
		cc.ID = id
		choreAuthor := pickAuthor(ids, nil, rng)
		cc.Author = choreAuthor
		cc.Committer = choreAuthor
```

The synthesized-empty-root commit currently uses `ids[0]`:

```go
		commits = append(commits, SynthCommit{
			ID: 0, Author: ids[0], Committer: ids[0], Message: "chore: initial commit",
		})
```

Change to:

```go
		rootAuthor := pickAuthor(ids, nil, rng)
		commits = append(commits, SynthCommit{
			ID: 0, Author: rootAuthor, Committer: rootAuthor, Message: "chore: initial commit",
		})
```

The per-feature rat assignment currently is:

```go
	for fi, run := range runs {
		rat := ids[fi%len(ids)]
```

Change to a random draw (the loop index `fi` is now unused for assignment; keep `range` as `for _, run := range runs` if `fi` becomes unused, or keep `fi` if other code uses it — check and adjust to compile cleanly):

```go
	for _, run := range runs {
		rat := pickAuthor(ids, nil, rng)
```

(If removing `fi` from the `range` causes an "unused" issue elsewhere in the loop body, verify — the loop body otherwise uses `run`, `rat`, `branchName`, etc. `fi` was only used for `ids[fi%len(ids)]`.)

- [ ] **Step 4: Run the suite — confirm the round-robin test goes red**

Run: `go test ./internal/fabricate/ -run 'Pigs|Rats|ReshapePigs|ReshapeRats' -v`
Expected: `TestPigsMode_TwoAuthors_RoundRobin` now FAILS — round-robin is gone; this is the expected red signal of the behavior change. Other RNG-order-sensitive pigs/rats tests may also fail (the author loop now consumes RNG, shifting the typo/noise draws).

- [ ] **Step 5: Replace the round-robin test and fix other breakage**

- **Delete `TestPigsMode_TwoAuthors_RoundRobin`** — it asserts the exact round-robin pattern, which no longer exists. Replace it by adding this new test to `internal/fabricate/pigs_test.go`:

```go
func TestReshapePigs_RandomAuthorDistribution(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}, {Name: "B", Email: "b@x"}}
	base := make([]SynthCommit, 300)
	for i := range base {
		base[i] = SynthCommit{ID: i, Message: "feat: c"}
	}
	plan := reshapePigs(base, ids, rand.New(rand.NewSource(1)))
	counts := map[string]int{}
	for _, c := range plan.Commits {
		counts[c.Author.Email]++
	}
	// A uniform draw over 300+ commits gives both players a substantial share.
	if counts["a@x"] < 90 || counts["b@x"] < 90 {
		t.Fatalf("expected both authors well-represented, got %+v", counts)
	}
}
```

- For any **other** pigs or rats test that now fails because the RNG draw order shifted, or because it asserted `ids[0]`/round-robin authorship: fix it honestly. Prefer asserting *structural invariants* (linear chain, HEAD ref, both authors appear, noise commits empty) and, for author-distribution claims, assert over a *large* base sequence so the law of large numbers makes it reliable — never assert an exact author cycle. `TestReshapePigs_LinearChainWithAuthors` asserts both identities appear over a 3-commit base; under a random draw 3 commits may not include both — if it fails, enlarge its base sequence (e.g. to 50 commits) so both authors reliably appear, keeping its linear-chain and HeadRef assertions intact. Do NOT weaken structural assertions.

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/... -v`
Expected: PASS — the new distribution test plus all (updated) pigs/rats tests.

- [ ] **Step 7: Commit**

```bash
git add internal/fabricate/pigs.go internal/fabricate/rats.go internal/fabricate/pigs_test.go internal/fabricate/rats_test.go
git commit -m "feat(fabricate): random author assignment, replacing round-robin"
```

## Before You Begin (Task 2)

Read the current `internal/fabricate/pigs.go` and `internal/fabricate/rats.go` to confirm the loops match the snippets above. Read `internal/fabricate/pigs_test.go` and `rats_test.go` to see exactly which tests assert round-robin / `ids[0]` / RNG-exact output, so Step 5's fixes are accurate. Run `go build ./...`, `go vet ./...` after.

---

## Task 3: Thread `weights []int` through the fabricator chain

**Files:**
- Modify: `internal/fabricate/pigs.go`, `internal/fabricate/rats.go`, `internal/fabricate/generate.go`, `internal/cli/pipeline.go`
- Test: update existing callers across `internal/fabricate/*_test.go`

This task adds an optional `weights []int` parameter (parallel to `ids`, `nil` = uniform) to the fabricator chain, replacing the hard-coded `nil` from Task 2. It is a mechanical signature change with no behavior change yet (the pipeline passes `nil`); the build only compiles when every caller is updated, so all five functions and all callers change together.

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/pigs_test.go`:

```go
func TestReshapePigs_WeightedAuthorDistribution(t *testing.T) {
	ids := []Identity{{Name: "Heavy", Email: "h@x"}, {Name: "Light", Email: "l@x"}}
	base := make([]SynthCommit, 300)
	for i := range base {
		base[i] = SynthCommit{ID: i, Message: "feat: c"}
	}
	plan := reshapePigs(base, ids, []int{9, 1}, rand.New(rand.NewSource(1)))
	counts := map[string]int{}
	for _, c := range plan.Commits {
		counts[c.Author.Email]++
	}
	if counts["h@x"] <= counts["l@x"] {
		t.Fatalf("weighted reshape not skewed toward Heavy: %+v", counts)
	}
}
```

- [ ] **Step 2: Run it to verify it fails**

Run: `go test ./internal/fabricate/ -run TestReshapePigs_WeightedAuthorDistribution -v`
Expected: FAIL — `reshapePigs` does not take a `weights` argument.

- [ ] **Step 3: Add `weights` to `reshapePigs`**

In `internal/fabricate/pigs.go`, change `reshapePigs`'s signature and its `pickAuthor` calls:

```go
func reshapePigs(base []SynthCommit, ids []Identity, weights []int, rng *rand.Rand) *Plan {
```

Inside, the author-assignment loop's `pickAuthor(ids, nil, rng)` becomes `pickAuthor(ids, weights, rng)`, and both noise-commit `pickAuthor(ids, nil, rng)` calls become `pickAuthor(ids, weights, rng)`.

Change `BuildPigsPlan` to take `weights` and forward it:

```go
func BuildPigsPlan(repo *git.Repository, ids []Identity, weights []int, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildPigsPlan: at least one identity required")
	}
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}
	return reshapePigs(base, ids, weights, rng), nil
}
```

- [ ] **Step 4: Add `weights` to `reshapeRats`**

In `internal/fabricate/rats.go`, change `reshapeRats`'s signature:

```go
func reshapeRats(base []SynthCommit, ids []Identity, weights []int, rng *rand.Rand) (*Plan, error) {
```

Inside, every `pickAuthor(ids, nil, rng)` call (chore loop, synthesized root, per-feature rat) becomes `pickAuthor(ids, weights, rng)`.

Change `BuildRatsPlan` to take `weights` and forward it:

```go
func BuildRatsPlan(repo *git.Repository, ids []Identity, weights []int, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildRatsPlan: at least one identity required")
	}
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}
	return reshapeRats(base, ids, weights, rng)
}
```

- [ ] **Step 5: Add `weights` to `Generate`**

In `internal/fabricate/generate.go`, change `Generate` to take `weights []int` (after `ids`) and forward it:

```go
func Generate(repo *git.Repository, ids []Identity, weights []int, mode string, rng *rand.Rand) (*Plan, *walk.DAG, error) {
	var plan *Plan
	var err error
	switch mode {
	case "rats":
		plan, err = BuildRatsPlan(repo, ids, weights, rng)
	default:
		plan, err = BuildPigsPlan(repo, ids, weights, rng)
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

- [ ] **Step 6: Update all callers**

The signature changes break callers. Update them:

- `internal/cli/pipeline.go` — the `fabricate.Generate(...)` call in `fabricatePipeline` becomes `fabricate.Generate(srcRepo, ids, nil, mode, rng)` (passing `nil` weights for now; Task 5 supplies real weights).
- **All existing test callers** in `internal/fabricate/*_test.go` of `reshapePigs`, `reshapeRats`, `BuildPigsPlan`, `BuildRatsPlan`, and `Generate` must pass `nil` for the new `weights` parameter (in the position shown above). Grep each function name across the test files and update every call site. The new `TestReshapePigs_WeightedAuthorDistribution` passes a real `[]int`; the Task-2 `TestReshapePigs_RandomAuthorDistribution` call becomes `reshapePigs(base, ids, nil, rand...)`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go build ./... && go test ./... 2>&1 | tail -30`
Expected: build clean; all packages pass, including both reshapePigs distribution tests.

- [ ] **Step 8: Commit**

```bash
git add internal/fabricate/pigs.go internal/fabricate/rats.go internal/fabricate/generate.go internal/cli/pipeline.go internal/fabricate/pigs_test.go internal/fabricate/rats_test.go
git commit -m "feat(fabricate): thread weights through the fabricator chain"
```

(Stage any other `internal/fabricate/*_test.go` files whose calls you updated, e.g. `write_test.go`, `squash_test.go`, `generate_test.go` if present.)

## Before You Begin (Task 3)

Grep the whole repo for `reshapePigs(`, `reshapeRats(`, `BuildPigsPlan(`, `BuildRatsPlan(`, `Generate(` to find every call site — production and test. A missed caller is a build break. `go build ./...` and `go test ./...` MUST be green at the end.

---

## Task 4: `--earned` flag (config + validation + CLI)

**Files:**
- Modify: `internal/input/config.go`, `internal/cli/cli.go`
- Test: `internal/input/config_test.go`, `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing config tests**

Add to `internal/input/config_test.go`:

```go
func TestValidate_EarnedRequiresPigsOrRats(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Earned = true
	if err := c.Validate(); err == nil {
		t.Fatal("expected --earned without --pigs/--rats to be rejected")
	}
	c.PigsN = 2
	if err := c.Validate(); err != nil {
		t.Fatalf("--earned with --pigs should validate, got: %v", err)
	}
}

func TestValidate_EarnedRequiresFabricate(t *testing.T) {
	c := baseValidConfig()
	c.Earned = true
	c.PigsN = 2
	if err := c.Validate(); err == nil {
		t.Fatal("expected --earned without --fabricate to be rejected")
	}
}
```

(`baseValidConfig()` already exists in `config_test.go` from earlier work; reuse it.)

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/input/ -run TestValidate_Earned -v`
Expected: FAIL — `Config` has no `Earned` field.

- [ ] **Step 3: Add the config field and validation**

In `internal/input/config.go`, add an `Earned bool` field to the `Config` struct alongside `Pick`:

```go
	Earned bool // --earned: weight author assignment by real commit-count distribution
```

In `Validate()`, after the `--pick` checks, add:

```go
	if c.Earned && !c.Fabricate {
		return errors.New("--earned requires --fabricate")
	}
	if c.Earned && c.PigsN == 0 && c.RatsN == 0 {
		return errors.New("--earned requires --pigs N or --rats N")
	}
```

Also add `c.Earned` to the `fabFlagsUsed` expression (the check that fabricate-only flags were not used without `--fabricate`), consistent with how `c.Pick` was added.

- [ ] **Step 4: Write the failing CLI test**

Add to `internal/cli/cli_test.go`:

```go
func TestRunEarnedFlagParses(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent-earned",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--pigs", "2", "--earned",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit (missing repo), got 0; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("--earned requires")) {
		t.Fatalf("--earned with --fabricate --pigs should pass validation; stderr=%q", errOut.String())
	}
}
```

- [ ] **Step 5: Run it to verify it fails**

Run: `go test ./internal/cli/ -run TestRunEarnedFlagParses -v`
Expected: FAIL — `--earned` is an unknown flag.

- [ ] **Step 6: Register the CLI flag**

In `internal/cli/cli.go`, in `newRootCmd`, add a flag variable to the `var (...)` block:

```go
		earnedFlag bool
```

Register it alongside the other fabricate flags:

```go
	cmd.Flags().BoolVar(&earnedFlag, "earned", false, "weight author assignment by real commit-count distribution (requires --pigs/--rats)")
```

And add `Earned: earnedFlag,` to the `input.Config{...}` struct literal in `RunE`.

- [ ] **Step 7: Run the tests to verify they pass**

Run: `go test ./internal/input/... ./internal/cli/... -v && go build ./... && go vet ./...`
Expected: PASS / clean.

- [ ] **Step 8: Commit**

```bash
git add internal/input/config.go internal/input/config_test.go internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat: --earned flag (config, validation, CLI)"
```

## Before You Begin (Task 4)

Read `internal/input/config.go` (`Config` struct, `Validate()`, the `--pick` checks and `fabFlagsUsed`) and `internal/cli/cli.go` (the `var (...)` block, flag registrations, `input.Config{}` literal). Mirror exactly how `--pick` was added — `--earned` is structurally identical.

---

## Task 5: `EarnedWeights` and pipeline weight computation

**Files:**
- Modify: `internal/fabricate/author.go`, `internal/cli/pipeline.go`
- Test: `internal/fabricate/author_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/author_test.go`:

```go
func TestEarnedWeights(t *testing.T) {
	discovered := []DiscoveredIdentity{
		{Identity: Identity{Name: "Heavy", Email: "h@x"}, Commits: 40},
		{Identity: Identity{Name: "Light", Email: "l@x"}, Commits: 10},
	}
	// Two discovered players + one non-discovered (typed) player.
	ids := []Identity{
		{Name: "Heavy", Email: "h@x"},
		{Name: "Light", Email: "l@x"},
		{Name: "Newcomer", Email: "new@x"},
	}
	w := EarnedWeights(ids, discovered, nil)
	if w == nil || len(w) != 3 {
		t.Fatalf("expected 3 weights, got %+v", w)
	}
	if w[0] != 40 || w[1] != 10 {
		t.Fatalf("discovered weights wrong: %+v", w)
	}
	// Newcomer gets the mean of discovered counts: round((40+10)/2) = 25.
	if w[2] != 25 {
		t.Fatalf("non-discovered weight should be the mean (25), got %d", w[2])
	}
}

func TestEarnedWeights_NoDiscoveredIsNil(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x"}}
	if w := EarnedWeights(ids, nil, nil); w != nil {
		t.Fatalf("no discovered identities should yield nil weights, got %+v", w)
	}
}

func TestEarnedWeights_MailmapCanonicalized(t *testing.T) {
	discovered := []DiscoveredIdentity{
		{Identity: Identity{Name: "Jay", Email: "jay@personal.com"}, Commits: 30},
	}
	mm := ParseMailmap([]byte("Jay <jay@personal.com> <jay@work.com>\n"))
	// A player listed under the non-canonical email resolves to the canonical
	// discovered count.
	ids := []Identity{{Name: "Jay", Email: "jay@work.com"}}
	w := EarnedWeights(ids, discovered, mm)
	if len(w) != 1 || w[0] != 30 {
		t.Fatalf("mailmap-canonicalized weight wrong: %+v", w)
	}
}
```

- [ ] **Step 2: Run them to verify they fail**

Run: `go test ./internal/fabricate/ -run TestEarnedWeights -v`
Expected: FAIL — `EarnedWeights` undefined.

- [ ] **Step 3: Implement `EarnedWeights`**

Append to `internal/fabricate/author.go` (and add `"strings"` to its import block — it currently imports only `"math/rand"`):

```go
// EarnedWeights builds a weights slice parallel to ids for the --earned draw.
// Each player's weight is its real commit count from discovered (matched by
// mailmap-canonicalized, lowercased email). A player absent from discovered
// gets the rounded mean of the discovered counts (min 1) — an "average
// contributor". If discovered is empty there is nothing to weight by and nil
// is returned, signalling a uniform fallback.
func EarnedWeights(ids []Identity, discovered []DiscoveredIdentity, mm *Mailmap) []int {
	if len(discovered) == 0 {
		return nil
	}
	counts := make(map[string]int, len(discovered))
	total := 0
	for _, d := range discovered {
		counts[strings.ToLower(strings.TrimSpace(d.Email))] = d.Commits
		total += d.Commits
	}
	mean := int(float64(total)/float64(len(discovered)) + 0.5)
	if mean < 1 {
		mean = 1
	}
	weights := make([]int, len(ids))
	for i, id := range ids {
		c := mm.Canonical(id)
		if w, ok := counts[strings.ToLower(strings.TrimSpace(c.Email))]; ok {
			weights[i] = w
		} else {
			weights[i] = mean
		}
	}
	return weights
}
```

- [ ] **Step 4: Run them to verify they pass**

Run: `go test ./internal/fabricate/ -run TestEarnedWeights -v`
Expected: PASS.

- [ ] **Step 5: Wire weight computation into the pipeline**

In `internal/cli/pipeline.go`, in `fabricatePipeline`: after `ids` is resolved and the `modelReport` is built (and `mailmap` is in scope from the `.mailmap` feature), compute the weights when `--earned` is set, then pass them into `Generate`.

Replace the current call (set in Task 3 to pass `nil`):

```go
	plan, dag, err := fabricate.Generate(srcRepo, ids, nil, mode, rng)
```

with:

```go
	var weights []int
	if cfg.Earned {
		discovered, derr := fabricate.DiscoverIdentities(srcRepo, mailmap)
		if derr != nil {
			fmt.Fprintln(errOut, "error: discover identities for --earned:", derr)
			return 1
		}
		weights = fabricate.EarnedWeights(ids, discovered, mailmap)
		if weights == nil {
			fmt.Fprintln(errOut, "note: --earned: no identities found in history; using equal weights")
		}
	}
	plan, dag, err := fabricate.Generate(srcRepo, ids, weights, mode, rng)
```

(`mailmap` is the `*fabricate.Mailmap` already loaded earlier in `fabricatePipeline`; `cfg.Earned` is the field from Task 4. `weights` stays `nil` when `--earned` is off, so `Generate`/`pickAuthor` draw uniformly.)

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go build ./... && go vet ./... && go test ./... 2>&1 | tail -20`
Expected: build clean; all packages pass.

- [ ] **Step 7: Commit**

```bash
git add internal/fabricate/author.go internal/fabricate/author_test.go internal/cli/pipeline.go
git commit -m "feat: compute --earned weights from real commit distribution"
```

## Before You Begin (Task 5)

Read `internal/cli/pipeline.go`'s `fabricatePipeline` to confirm where `ids`, `mailmap`, and `rng` are in scope and where the `fabricate.Generate(...)` call (passing `nil` from Task 3) sits — the weight computation goes immediately before it. Confirm `DiscoverIdentities` has the signature `DiscoverIdentities(repo, *Mailmap)` and `DiscoveredIdentity` has a `Commits int` field. `go build ./...` / `go test ./...` MUST be green.

---

## Task 6: End-to-end `--earned` integration test

**Files:**
- Modify: `internal/cli/fabricate_integration_test.go`

- [ ] **Step 1: Write the test**

Add to `internal/cli/fabricate_integration_test.go`. It builds a repo with a lopsided real history — one heavy author, one light author — runs the full fabricate pipeline with `--pigs 2 --earned`, and asserts the fabricated history's author distribution clearly favors the heavy contributor.

```go
func TestIntegration_Fabricate_EarnedFavorsHeavyContributor(t *testing.T) {
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
	if out, err := exec.Command("git", "-C", src, "init").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	// Heavy author: 8 commits across several feature dirs. Light author: 1.
	heavyFiles := []string{
		"internal/walk/load.go", "internal/walk/dag.go", "internal/cli/main.go",
		"internal/cli/run.go", "internal/repo/clone.go", "internal/repo/swap.go",
		"internal/report/row.go", "README.md",
	}
	for _, f := range heavyFiles {
		commit("Heavy", "heavy@example.com", f)
	}
	commit("Light", "light@example.com", "internal/input/config.go")

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-60 * 24 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		PigsN:     2,
		Earned:    true,
		Seed:      5,
		HasSeed:   true,
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}

	emails, err := exec.Command("git", "-C", src, "log", "--all", "--format=%ae").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, emails)
	}
	heavy := bytes.Count(emails, []byte("heavy@example.com"))
	light := bytes.Count(emails, []byte("light@example.com"))
	if heavy == 0 {
		t.Fatalf("heavy contributor absent from fabricated history:\n%s", emails)
	}
	if heavy <= light {
		t.Fatalf("--earned did not favor the heavy contributor: heavy=%d light=%d\n%s", heavy, light, emails)
	}
}
```

- [ ] **Step 2: Run the test to verify it passes**

Run: `go test ./internal/cli/ -run TestIntegration_Fabricate_EarnedFavorsHeavyContributor -v`
Expected: PASS. With `--pigs 2` the resolver discovers exactly the two authors (8:1 commit counts → weights 8 and 1), so `--earned` skews fabricated authorship heavily toward `heavy@example.com`. If the assertion `heavy <= light` trips, the weighting is not being applied — investigate the Task 5 wiring; do not weaken the assertion. The 60-day window is wide enough to avoid squashing.

- [ ] **Step 3: Run the full suite**

Run: `go build ./... && go test ./... && go vet ./... && gofmt -l internal/ cmd/`
Expected: all pass, `gofmt` reports nothing.

- [ ] **Step 4: Commit**

```bash
git add internal/cli/fabricate_integration_test.go
git commit -m "test(cli): end-to-end --earned favors heavy contributor"
```

## Before You Begin (Task 6)

Confirm `internal/cli/fabricate_integration_test.go` already imports `bytes`, `os`, `os/exec`, `path/filepath`, `time`, and the `input` package (other integration tests in the file use them). Confirm no test of this name already exists.

---

## Final verification

After all tasks, dispatch a final code reviewer for the whole change set, then:

```bash
gofmt -l internal/ cmd/
go build ./... && go test ./... && go vet ./...
```

All must be clean.
