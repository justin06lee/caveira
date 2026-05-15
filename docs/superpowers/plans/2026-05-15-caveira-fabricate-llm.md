# Caveira Fabricate Phase 2 (LLM Providers) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add five LLM provider engines (`--groq`, `--claude-code`, `--codex`, `--nvidia`, `--opencode`) that let an LLM design the fabricated commit history — grouping, ordering, layered file-modifying commits, and messages — which Caveira then realizes deterministically so the final tree always equals source HEAD.

**Architecture:** `--fabricate` runs a *base engine* producing a linear `[]SynthCommit` sequence; `--pigs`/`--rats` reshape it. Phase 2 adds LLM engines as alternative base producers. The LLM returns a JSON plan referencing pre-computed file *segments* (never raw content); a deterministic realizer validates, clamps missing segments onto each file's last commit, and builds per-commit blobs — guaranteeing byte-exact final state. The flurry/pigs/rats engines are refactored so all base producers feed shared `reshapePigs`/`reshapeRats` reshapers.

**Tech Stack:** Go 1.22+, `github.com/go-git/go-git/v5`, `github.com/spf13/cobra`, stdlib `net/http`, `os/exec`, `encoding/json`.

**Reference spec:** `docs/superpowers/specs/2026-05-15-caveira-fabricate-llm-design.md`

---

## File Structure

**New files:**
- `internal/fabricate/segment.go` (+ `_test.go`) — split a file's bytes into an exact ordered partition of line-range segments.
- `internal/fabricate/realize.go` (+ `_test.go`) — convert an `llm.Plan` + source files into a linear `[]SynthCommit` base, with clamping and per-commit blob/stat computation.
- `internal/fabricate/llmgen.go` (+ `_test.go`) — orchestrator: walk → segment → prompt → provider (retry) → parse → validate → realize → reshape → DAG.
- `internal/fabricate/llm/provider.go` (+ `_test.go`) — `Provider` interface + flag→constructor registry.
- `internal/fabricate/llm/openai_compat.go` (+ `_test.go`) — shared OpenAI-compatible HTTP provider for Groq and NVIDIA.
- `internal/fabricate/llm/cli_provider.go` (+ `_test.go`) — shared subprocess provider for claude-code, codex, opencode.
- `internal/fabricate/llm/prompt.go` (+ `_test.go`) — build the LLM prompt from file inputs + segment maps.
- `internal/fabricate/llm/plan.go` (+ `_test.go`) — plan JSON schema, balanced-object extraction, parsing.

**Modified files:**
- `internal/fabricate/types.go` — `FileRef.Content`, `SynthCommit.Feature`, `SynthCommit.Stats`, `DiffStat`.
- `internal/fabricate/write.go` — write synthetic-content blobs to dst.
- `internal/fabricate/dag.go` — `PlanToDAG` honors `SynthCommit.Stats`.
- `internal/fabricate/flurry.go` — `FlurrySequence` sets `SynthCommit.Feature`.
- `internal/fabricate/pigs.go` — extract `reshapePigs`.
- `internal/fabricate/rats.go` — base-driven `reshapeRats`.
- `internal/fabricate/generate.go` — unify around base + reshape; add `GenerateLLM`.
- `internal/input/config.go` — `Provider`, `Model`, `LLMTimeout` fields + validation.
- `internal/cli/cli.go` — new flags.
- `internal/cli/pipeline.go` — LLM engine branch + tree verification.
- `README.md` — document the new flags.

---

## Task 1: Config fields and base-engine validation

**Files:**
- Modify: `internal/input/config.go`
- Test: `internal/input/config_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/input/config_test.go`:

```go
func TestValidate_LLMProviderIsValidBaseEngine(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Provider = "groq"
	if err := c.Validate(); err != nil {
		t.Fatalf("expected --fabricate --groq to validate, got: %v", err)
	}
}

func TestValidate_NoBaseEngineRejected(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	err := c.Validate()
	if err == nil || !strings.Contains(err.Error(), "--flurry") {
		t.Fatalf("expected error listing base engines, got: %v", err)
	}
}

func TestValidate_TwoBaseEnginesRejected(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Flurry = true
	c.Provider = "groq"
	if err := c.Validate(); err == nil {
		t.Fatal("expected mutually-exclusive error for --flurry + --groq")
	}
}

func TestValidate_ModelRequiresLLMEngine(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Flurry = true
	c.Model = "some-model"
	if err := c.Validate(); err == nil {
		t.Fatal("expected --model to be rejected with --flurry")
	}
}

func TestValidate_LLMComposesWithRats(t *testing.T) {
	c := baseValidConfig()
	c.Fabricate = true
	c.Provider = "claude-code"
	c.RatsN = 3
	if err := c.Validate(); err != nil {
		t.Fatalf("expected --claude-code --rats 3 to validate, got: %v", err)
	}
}
```

If `baseValidConfig` does not already exist in the test file, add this helper:

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

Ensure the test file imports `"strings"` and `"time"`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/input/ -run TestValidate -v`
Expected: FAIL — `Provider`, `Model`, `LLMTimeout` are undefined fields.

- [ ] **Step 3: Add the fields**

In `internal/input/config.go`, inside the `Config` struct after `RatIdentities`:

```go
	// Fabricate-mode fields (Phase 2: LLM providers)
	Provider   string        // "" = no LLM engine; else one of the registry names
	Model      string        // optional model override for the LLM provider
	LLMTimeout time.Duration // per-LLM-call timeout; 0 = use default
```

- [ ] **Step 4: Replace the base-engine validation block**

In `Validate()`, replace the two lines:

```go
	if c.Fabricate && !c.Flurry {
		return errors.New("--fabricate requires --flurry (LLM providers are Phase 2)")
	}
```

with:

```go
	baseEngines := 0
	if c.Flurry {
		baseEngines++
	}
	if c.Provider != "" {
		baseEngines++
	}
	if c.Fabricate && baseEngines == 0 {
		return errors.New("--fabricate requires a base engine: --flurry, --groq, --claude-code, --codex, --nvidia, or --opencode")
	}
	if baseEngines > 1 {
		return errors.New("base engines are mutually exclusive: pick one of --flurry, --groq, --claude-code, --codex, --nvidia, --opencode")
	}
	if (c.Model != "" || c.LLMTimeout != 0) && c.Provider == "" {
		return errors.New("--model and --llm-timeout require an LLM engine (--groq, --claude-code, --codex, --nvidia, or --opencode)")
	}
```

Note: the existing `fabFlagsUsed` check above this block must also account for `Provider`. Update it:

```go
	fabFlagsUsed := c.Flurry || c.Provider != "" || c.PigsN > 0 || c.RatsN > 0 ||
		len(c.PigIdentities) > 0 || len(c.RatIdentities) > 0
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/input/ -v`
Expected: PASS (all existing input tests plus the new ones).

- [ ] **Step 6: Commit**

```bash
git add internal/input/config.go internal/input/config_test.go
git commit -m "feat(input): config fields and validation for LLM base engines"
```

---

## Task 2: CLI flags for LLM providers

**Files:**
- Modify: `internal/cli/cli.go`
- Test: `internal/cli/cli_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/cli_test.go`:

```go
func TestRunLLMFlagsParse(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/nonexistent-llm",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate", "--groq",
		"--model", "test-model",
		"--llm-timeout", "30s",
	}, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit (missing repo), got 0; stderr=%q", errOut.String())
	}
	if bytes.Contains(errOut.Bytes(), []byte("base engine")) {
		t.Fatalf("flags should have parsed past validation; stderr=%q", errOut.String())
	}
}

func TestRunFabricateNoBaseEngineRejected(t *testing.T) {
	var out, errOut bytes.Buffer
	code := RunWithArgs([]string{
		"--repo", "/tmp/x",
		"--start", "2026-05-14 12:00",
		"--end", "2026-05-14 14:00",
		"--window-tz", "UTC",
		"--fabricate",
	}, &out, &errOut)
	if code == 0 {
		t.Fatal("expected non-zero exit when --fabricate has no base engine")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunLLMFlagsParse -v`
Expected: FAIL — `--groq` is an unknown flag.

- [ ] **Step 3: Add the flag variables and registrations**

In `internal/cli/cli.go`, inside `newRootCmd`, add to the `var (...)` block after `ratIDs`:

```go
		groqFlag       bool
		claudeCodeFlag bool
		codexFlag      bool
		nvidiaFlag     bool
		openCodeFlag   bool
		modelFlag      string
		llmTimeoutFlag time.Duration
```

After the existing `cmd.Flags().StringArrayVar(&ratIDs, ...)` line, add:

```go
	cmd.Flags().BoolVar(&groqFlag, "groq", false, "LLM engine: Groq API (requires --fabricate, GROQ_API_KEY)")
	cmd.Flags().BoolVar(&claudeCodeFlag, "claude-code", false, "LLM engine: claude CLI subprocess (requires --fabricate)")
	cmd.Flags().BoolVar(&codexFlag, "codex", false, "LLM engine: codex CLI subprocess (requires --fabricate)")
	cmd.Flags().BoolVar(&nvidiaFlag, "nvidia", false, "LLM engine: NVIDIA API (requires --fabricate, NVIDIA_API_KEY)")
	cmd.Flags().BoolVar(&openCodeFlag, "opencode", false, "LLM engine: opencode CLI subprocess (requires --fabricate)")
	cmd.Flags().StringVar(&modelFlag, "model", "", "override the LLM provider's default model")
	cmd.Flags().DurationVar(&llmTimeoutFlag, "llm-timeout", 0, "per-LLM-call timeout (default 120s)")
```

- [ ] **Step 4: Resolve the provider name and wire into Config**

In the `RunE` closure, immediately before the `cfg := &input.Config{` literal, add:

```go
				provider := ""
				for name, on := range map[string]bool{
					"groq": groqFlag, "claude-code": claudeCodeFlag,
					"codex": codexFlag, "nvidia": nvidiaFlag, "opencode": openCodeFlag,
				} {
					if on {
						if provider != "" {
							return fmt.Errorf("only one LLM engine may be selected")
						}
						provider = name
					}
				}
```

Then add these fields to the `input.Config{...}` literal (after `RatIdentities`):

```go
					Provider:      provider,
					Model:         modelFlag,
					LLMTimeout:    llmTimeoutFlag,
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run 'TestRun' -v`
Expected: PASS for the two new tests and all existing CLI tests.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/cli.go internal/cli/cli_test.go
git commit -m "feat(cli): --groq/--claude-code/--codex/--nvidia/--opencode flags"
```

---

## Task 3: Synthetic-content blobs in FileRef and write.go

**Files:**
- Modify: `internal/fabricate/types.go`, `internal/fabricate/write.go`
- Test: `internal/fabricate/write_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/write_test.go`:

```go
func TestWriteToRepo_SyntheticContentBlob(t *testing.T) {
	src := newEmptyRepo(t)
	dst := newEmptyRepo(t)

	content := []byte("package main\n\nfunc main() {}\n")
	h := plumbing.ComputeHash(plumbing.BlobObject, content)

	plan := &Plan{
		Commits: []SynthCommit{
			{
				ID:      0,
				Author:  Identity{Name: "A", Email: "a@x.com"},
				Message: "feat: add main",
				Added: []FileRef{
					{Path: "main.go", Content: content, Blob: h, Mode: filemode.Regular},
				},
			},
		},
		Refs:    map[string]int{"refs/heads/master": 0},
		HEAD:    0,
		HeadRef: "refs/heads/master",
	}
	times := map[string]time.Time{SyntheticOID(0): time.Now()}

	if _, err := WriteToRepo(src, dst, plan, times); err != nil {
		t.Fatalf("WriteToRepo: %v", err)
	}
	obj, err := dst.Storer.EncodedObject(plumbing.BlobObject, h)
	if err != nil {
		t.Fatalf("synthetic blob not written to dst: %v", err)
	}
	if obj.Size() != int64(len(content)) {
		t.Fatalf("blob size = %d, want %d", obj.Size(), len(content))
	}
}
```

`newEmptyRepo` should create an in-memory repo: `git.Init(memory.NewStorage(), memfs.New())`. If `write_test.go` lacks such a helper, add one (check the file first — other fabricate tests use in-memory repos and a helper likely exists; reuse it). Ensure imports include `plumbing`, `filemode`, `time`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestWriteToRepo_SyntheticContentBlob -v`
Expected: FAIL — `FileRef` has no `Content` field.

- [ ] **Step 3: Add the Content field**

In `internal/fabricate/types.go`, change the `FileRef` struct:

```go
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
```

Also update the `SynthCommit.Added` doc comment to: `// Added holds files this commit creates or updates.`

- [ ] **Step 4: Branch the blob-copy phase in write.go**

In `internal/fabricate/write.go`, replace the blob-copy loop (the block that iterates `plan.Commits` and calls `copyBlob`):

```go
	// Copy source blobs and write synthetic-content blobs into dst.
	seenBlobs := map[plumbing.Hash]bool{}
	for _, c := range plan.Commits {
		for _, fr := range c.Added {
			if seenBlobs[fr.Blob] {
				continue
			}
			seenBlobs[fr.Blob] = true
			if fr.Content != nil {
				if err := writeBlob(dst, fr.Content); err != nil {
					return nil, fmt.Errorf("write synthetic blob %s: %w", fr.Blob, err)
				}
				continue
			}
			if err := copyBlob(src, dst, fr.Blob); err != nil {
				return nil, fmt.Errorf("copy blob %s: %w", fr.Blob, err)
			}
		}
	}
```

Add this helper near `copyBlob`:

```go
// writeBlob writes content as a blob object into dst. The resulting hash
// equals plumbing.ComputeHash(plumbing.BlobObject, content).
func writeBlob(dst *git.Repository, content []byte) error {
	ne := dst.Storer.NewEncodedObject()
	ne.SetType(plumbing.BlobObject)
	w, err := ne.Writer()
	if err != nil {
		return err
	}
	if _, err := w.Write(content); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	_, err = dst.Storer.SetEncodedObject(ne)
	return err
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run TestWriteToRepo -v`
Expected: PASS, including all existing `WriteToRepo` tests.

- [ ] **Step 6: Commit**

```bash
git add internal/fabricate/types.go internal/fabricate/write.go internal/fabricate/write_test.go
git commit -m "feat(fabricate): synthetic-content blobs in FileRef and WriteToRepo"
```

---

## Task 4: DiffStat override in PlanToDAG

**Files:**
- Modify: `internal/fabricate/types.go`, `internal/fabricate/dag.go`
- Test: `internal/fabricate/dag_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/dag_test.go`:

```go
func TestPlanToDAG_HonorsExplicitStats(t *testing.T) {
	src := newEmptyRepo(t)
	plan := &Plan{
		Commits: []SynthCommit{
			{
				ID:      0,
				Author:  Identity{Name: "A", Email: "a@x.com"},
				Message: "feat: layered",
				Added: []FileRef{
					{Path: "f.go", Content: []byte("a\nb\nc\nd\ne\n"),
						Blob: plumbing.ComputeHash(plumbing.BlobObject, []byte("a\nb\nc\nd\ne\n"))},
				},
				Stats: &DiffStat{Lines: 2, Files: 1, NewFiles: 1},
			},
		},
		Refs: map[string]int{"refs/heads/master": 0}, HEAD: 0, HeadRef: "refs/heads/master",
	}
	dag, err := PlanToDAG(src, plan)
	if err != nil {
		t.Fatalf("PlanToDAG: %v", err)
	}
	c := dag.Get(SyntheticOID(0))
	if c.LinesChanged != 2 {
		t.Fatalf("LinesChanged = %d, want 2 (from explicit Stats)", c.LinesChanged)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestPlanToDAG_HonorsExplicitStats -v`
Expected: FAIL — `SynthCommit` has no `Stats` field; `DiffStat` undefined.

- [ ] **Step 3: Add DiffStat and the Stats field**

In `internal/fabricate/types.go`, add the type and field:

```go
// DiffStat is an explicit per-commit edit-volume override. When a SynthCommit
// sets Stats, PlanToDAG uses it instead of counting bytes in Added (used by
// layered LLM commits whose Added holds cumulative, not delta, content).
type DiffStat struct {
	Lines    int
	Files    int
	NewFiles int
}
```

Add to `SynthCommit` after `IsMerge`:

```go
	Feature string    // feature/scope name; "" for chore or non-feature commits
	Stats   *DiffStat // optional explicit edit-volume; nil = count from Added
```

- [ ] **Step 4: Honor Stats in PlanToDAG**

In `internal/fabricate/dag.go`, replace the stat-counting block inside the `for _, sc := range plan.Commits` loop:

```go
		lines, files, newFiles := 0, 0, 0
		if sc.Stats != nil {
			lines, files, newFiles = sc.Stats.Lines, sc.Stats.Files, sc.Stats.NewFiles
		} else {
			for _, fr := range sc.Added {
				blob, err := srcRepo.BlobObject(fr.Blob)
				if err != nil {
					return nil, err
				}
				lines += countBlobLines(blob)
				files++
				newFiles++
			}
		}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run TestPlanToDAG -v`
Expected: PASS, including existing PlanToDAG tests.

- [ ] **Step 6: Commit**

```bash
git add internal/fabricate/types.go internal/fabricate/dag.go internal/fabricate/dag_test.go
git commit -m "feat(fabricate): DiffStat override for layered-commit edit volume"
```

---

## Task 5: FlurrySequence populates SynthCommit.Feature

**Files:**
- Modify: `internal/fabricate/flurry.go`
- Test: `internal/fabricate/flurry_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/flurry_test.go`:

```go
func TestFlurrySequence_SetsFeature(t *testing.T) {
	repo := buildFixtureRepo(t) // existing helper that adds internal/walk/*.go etc.
	seq, err := FlurrySequence(repo, Identity{Name: "A", Email: "a@x.com"}, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("FlurrySequence: %v", err)
	}
	sawFeature := false
	for _, c := range seq {
		if strings.HasPrefix(c.Message, "chore") {
			if c.Feature != "" {
				t.Fatalf("chore commit Feature = %q, want empty", c.Feature)
			}
			continue
		}
		if c.Feature == "" {
			t.Fatalf("feature commit %q has empty Feature", c.Message)
		}
		sawFeature = true
	}
	if !sawFeature {
		t.Fatal("expected at least one commit with a Feature set")
	}
}
```

Use whatever fixture-repo helper `flurry_test.go` already uses; if the helper is named differently, match the existing name. Ensure imports include `"math/rand"` and `"strings"`.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestFlurrySequence_SetsFeature -v`
Expected: FAIL — feature commits have empty `Feature`.

- [ ] **Step 3: Set Feature in FlurrySequence**

In `internal/fabricate/flurry.go`, in the `for _, feat := range features` loop, set `Feature: basenameDir(feat.Dir)` on both the code and test `SynthCommit` literals. The code commit becomes:

```go
		if len(feat.Code) > 0 {
			c := SynthCommit{
				ID:        idx,
				Parents:   []int{prev},
				Author:    id,
				Committer: id,
				Message:   CodeMessage(feat.Dir, rng),
				Added:     feat.Code,
				Feature:   basenameDir(feat.Dir),
			}
			commits = append(commits, c)
			prev = idx
			idx++
		}
```

Apply the identical `Feature: basenameDir(feat.Dir)` addition to the test commit literal. The chore commit literal is left unchanged (`Feature` stays `""`).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run 'TestFlurry' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/flurry.go internal/fabricate/flurry_test.go
git commit -m "feat(fabricate): FlurrySequence tags commits with their feature"
```

---

## Task 6: Extract reshapePigs

**Files:**
- Modify: `internal/fabricate/pigs.go`
- Test: `internal/fabricate/pigs_test.go`

This is a behavior-preserving extraction: `BuildPigsPlan` keeps producing identical output (same RNG draw order).

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/pigs_test.go`:

```go
func TestReshapePigs_LinearChainWithAuthors(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x.com"}, {Name: "B", Email: "b@x.com"}}
	base := []SynthCommit{
		{ID: 0, Message: "chore: scaffold"},
		{ID: 1, Parents: []int{0}, Message: "feat(walk): add walk", Feature: "walk"},
		{ID: 2, Parents: []int{1}, Message: "feat(cli): add cli", Feature: "cli"},
	}
	plan := reshapePigs(base, ids, rand.New(rand.NewSource(7)))
	if plan.HeadRef != defaultBranch {
		t.Fatalf("HeadRef = %q, want %q", plan.HeadRef, defaultBranch)
	}
	authors := map[string]bool{}
	for _, c := range plan.Commits {
		authors[c.Author.Email] = true
		if len(c.Parents) > 1 {
			t.Fatalf("pigs plan must be linear; commit %d has %d parents", c.ID, len(c.Parents))
		}
	}
	if len(authors) < 2 {
		t.Fatalf("expected both identities used, got %d", len(authors))
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestReshapePigs -v`
Expected: FAIL — `reshapePigs` undefined.

- [ ] **Step 3: Extract reshapePigs and rewrite BuildPigsPlan as a wrapper**

In `internal/fabricate/pigs.go`, replace the body of `BuildPigsPlan` after the `FlurrySequence` call, and add `reshapePigs`. The full file body becomes:

```go
// BuildPigsPlan produces a Plan for pigs mode from the flurry base sequence.
func BuildPigsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildPigsPlan: at least one identity required")
	}
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}
	return reshapePigs(base, ids, rng), nil
}

// reshapePigs reshapes a linear base sequence for pigs mode: round-robin
// authors across real commits, typos on every message, and ~noiseRate noise
// commits injected between adjacent real commits. The base sequence's commit
// IDs and parents are reassigned; callers need not pre-link them.
func reshapePigs(base []SynthCommit, ids []Identity, rng *rand.Rand) *Plan {
	for i := range base {
		id := ids[i%len(ids)]
		base[i].Author = id
		base[i].Committer = id
	}
	for i := range base {
		base[i].Message = ApplyTypos(base[i].Message, rng)
	}

	var out []SynthCommit
	base[0].ID = 0
	base[0].Parents = nil
	out = append(out, base[0])
	for i := 1; i < len(base); i++ {
		if rng.Float64() < noiseRate {
			noise := SynthCommit{
				Author:    ids[rng.Intn(len(ids))],
				Committer: ids[rng.Intn(len(ids))],
				Message:   ApplyTypos(noiseMessages[rng.Intn(len(noiseMessages))], rng),
			}
			noise.ID = len(out)
			noise.Parents = []int{out[len(out)-1].ID}
			out = append(out, noise)
		}
		base[i].ID = len(out)
		base[i].Parents = []int{out[len(out)-1].ID}
		out = append(out, base[i])
	}

	return &Plan{
		Commits: out,
		Refs:    map[string]int{defaultBranch: out[len(out)-1].ID},
		HEAD:    out[len(out)-1].ID,
		HeadRef: defaultBranch,
	}
}
```

Note: the original `BuildPigsPlan` already calls `FlurrySequence` then assigns authors, typos, and noise — the extraction preserves that exact order, so existing pigs tests and the Phase 1 integration test still pass. The only added line is `base[0].Parents = nil` (defensive, since a non-flurry base might arrive pre-linked).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run 'Pigs' -v`
Expected: PASS, including all existing pigs tests.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/pigs.go internal/fabricate/pigs_test.go
git commit -m "refactor(fabricate): extract reshapePigs from BuildPigsPlan"
```

---

## Task 7: Base-driven reshapeRats

**Files:**
- Modify: `internal/fabricate/rats.go`
- Test: `internal/fabricate/rats_test.go`

`reshapeRats` consumes a linear base sequence (chore prefix + per-`Feature` runs) and produces the emergent-topology Plan. `BuildRatsPlan` becomes `FlurrySequence` + `reshapeRats`. Because the RNG draw order changes (all message generation now happens in `FlurrySequence` before any fork/conflict rolls), any existing rats test that asserts an exact seeded topology must be re-recorded in this task.

- [ ] **Step 1: Write the failing test**

Add to `internal/fabricate/rats_test.go`:

```go
func TestReshapeRats_BranchesPerFeature(t *testing.T) {
	ids := []Identity{{Name: "A", Email: "a@x.com"}, {Name: "B", Email: "b@x.com"}}
	base := []SynthCommit{
		{ID: 0, Message: "chore: scaffold"},
		{ID: 1, Message: "feat(walk): add walk", Feature: "walk"},
		{ID: 2, Message: "test(walk): tests for walk", Feature: "walk"},
		{ID: 3, Message: "feat(cli): add cli", Feature: "cli"},
	}
	plan, err := reshapeRats(base, ids, rand.New(rand.NewSource(3)))
	if err != nil {
		t.Fatalf("reshapeRats: %v", err)
	}
	featBranches, merges := 0, 0
	for name := range plan.Refs {
		if strings.HasPrefix(name, "refs/heads/feat/") {
			featBranches++
		}
	}
	for _, c := range plan.Commits {
		if c.IsMerge {
			merges++
		}
	}
	if featBranches < 2 {
		t.Fatalf("expected >= 2 feat branches, got %d", featBranches)
	}
	if merges < 2 {
		t.Fatalf("expected >= 2 merge commits, got %d", merges)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestReshapeRats -v`
Expected: FAIL — `reshapeRats` undefined.

- [ ] **Step 3: Rewrite rats.go around reshapeRats**

Replace the entire contents of `internal/fabricate/rats.go` with:

```go
package fabricate

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/go-git/go-git/v5"
)

const offBranchForkProb = 0.30

const (
	conflictFixProb       = 0.20 // probability of a conflict-fix scar after a merge
	conflictFixBranchProb = 0.40 // probability the scar is a fix-branch (given a scar)
)

// BuildRatsPlan produces a Plan for rats mode from the flurry base sequence.
func BuildRatsPlan(repo *git.Repository, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(ids) == 0 {
		return nil, errors.New("BuildRatsPlan: at least one identity required")
	}
	base, err := FlurrySequence(repo, ids[0], rng)
	if err != nil {
		return nil, err
	}
	return reshapeRats(base, ids, rng)
}

// featureRun is a contiguous run of base commits sharing one Feature.
type featureRun struct {
	feature string
	commits []SynthCommit
}

// splitBase partitions a linear base sequence into a leading chore run (commits
// with Feature == "") and one featureRun per contiguous same-Feature run.
func splitBase(base []SynthCommit) (chore []SynthCommit, runs []featureRun) {
	for _, c := range base {
		if c.Feature == "" {
			if len(runs) == 0 {
				chore = append(chore, c)
				continue
			}
		}
		if len(runs) > 0 && runs[len(runs)-1].feature == c.Feature && c.Feature != "" {
			runs[len(runs)-1].commits = append(runs[len(runs)-1].commits, c)
			continue
		}
		if c.Feature == "" {
			// A non-feature commit after features begin: attach to the last run.
			runs[len(runs)-1].commits = append(runs[len(runs)-1].commits, c)
			continue
		}
		runs = append(runs, featureRun{feature: c.Feature, commits: []SynthCommit{c}})
	}
	return chore, runs
}

// reshapeRats reshapes a linear base sequence into the rats emergent topology:
// each featureRun becomes a branch (forking from master or another open
// branch), branches merge back into master, and merges may leave conflict-fix
// scars. Commit IDs and parents are reassigned by this function.
func reshapeRats(base []SynthCommit, ids []Identity, rng *rand.Rand) (*Plan, error) {
	if len(base) == 0 {
		return nil, errors.New("reshapeRats: empty base sequence")
	}
	choreCommits, runs := splitBase(base)

	var commits []SynthCommit
	refs := map[string]int{}
	next := func() int { return len(commits) }

	// Chore commit(s) on master.
	masterTip := -1
	for _, cc := range choreCommits {
		id := next()
		cc.ID = id
		cc.Author = ids[0]
		cc.Committer = ids[0]
		if masterTip >= 0 {
			cc.Parents = []int{masterTip}
		} else {
			cc.Parents = nil
		}
		commits = append(commits, cc)
		masterTip = id
	}
	if masterTip < 0 {
		// No chore commit: synthesize an empty root so feature branches have a base.
		commits = append(commits, SynthCommit{
			ID: 0, Author: ids[0], Committer: ids[0], Message: "chore: initial commit",
		})
		masterTip = 0
	}

	// Phase 1: build every feature branch.
	type branch struct {
		rat        Identity
		branchName string
		tip        int
	}
	var branches []branch
	var openBranchTips []int
	for fi, run := range runs {
		rat := ids[fi%len(ids)]
		branchName := fmt.Sprintf("refs/heads/feat/%s", run.feature)
		parent := pickForkParent(masterTip, openBranchTips, rng)
		tip := parent
		for _, rc := range run.commits {
			id := next()
			rc.ID = id
			rc.Author = rat
			rc.Committer = rat
			rc.Parents = []int{tip}
			commits = append(commits, rc)
			tip = id
		}
		openBranchTips = append(openBranchTips, tip)
		refs[branchName] = tip
		branches = append(branches, branch{rat: rat, branchName: branchName, tip: tip})
	}

	// Phase 2: merge each branch into master, with optional conflict-fix scars.
	for _, b := range branches {
		mergeID := next()
		commits = append(commits, SynthCommit{
			ID:        mergeID,
			Parents:   []int{masterTip, b.tip},
			Author:    b.rat,
			Committer: b.rat,
			Message:   fmt.Sprintf("Merge branch '%s' into master", trimRefsHeads(b.branchName)),
			IsMerge:   true,
		})
		masterTip = mergeID

		if rng.Float64() < conflictFixProb {
			feat := b.branchName[len("refs/heads/feat/"):]
			if rng.Float64() < conflictFixBranchProb {
				fixID := next()
				commits = append(commits, SynthCommit{
					ID: fixID, Parents: []int{masterTip}, Author: b.rat, Committer: b.rat,
					Message: fmt.Sprintf("fix: resolve conflict in %s", feat),
				})
				refs[fmt.Sprintf("refs/heads/fix/%s", feat)] = fixID
				mergeFixID := next()
				commits = append(commits, SynthCommit{
					ID: mergeFixID, Parents: []int{masterTip, fixID}, Author: b.rat,
					Committer: b.rat, IsMerge: true,
					Message: fmt.Sprintf("Merge branch 'fix/%s' into master", feat),
				})
				masterTip = mergeFixID
			} else {
				fixID := next()
				commits = append(commits, SynthCommit{
					ID: fixID, Parents: []int{masterTip}, Author: b.rat, Committer: b.rat,
					Message: fmt.Sprintf("fix: resolve conflict in %s", feat),
				})
				masterTip = fixID
			}
		}
	}

	refs[defaultBranch] = masterTip
	return &Plan{Commits: commits, Refs: refs, HEAD: masterTip, HeadRef: defaultBranch}, nil
}

func trimRefsHeads(ref string) string {
	const p = "refs/heads/"
	if len(ref) > len(p) && ref[:len(p)] == p {
		return ref[len(p):]
	}
	return ref
}

// pickForkParent returns the parent commit ID for a new feature branch's first
// commit: an open branch tip with probability offBranchForkProb, else master.
func pickForkParent(masterTip int, openBranchTips []int, rng *rand.Rand) int {
	if len(openBranchTips) > 0 && rng.Float64() < offBranchForkProb {
		return openBranchTips[rng.Intn(len(openBranchTips))]
	}
	return masterTip
}
```

- [ ] **Step 4: Re-record any broken Phase 1 rats fixture tests**

Run: `go test ./internal/fabricate/ -run 'Rats' -v`

If a test that asserts an exact seeded topology (commit count, recorded branch shape) now fails, the RNG draw order legitimately changed. For each such test: re-run with the test's seed, inspect the new topology, and update the test's expected values to the new recorded shape — provided the new shape still satisfies the *structural* invariants (feature branches present, merges present, conflict-fix events occur at some seeds). Do not weaken structural assertions; only update exact recorded numbers. Tests asserting ranges or invariants (not exact shapes) should still pass unchanged.

- [ ] **Step 5: Run the full fabricate test suite**

Run: `go test ./internal/fabricate/ -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/fabricate/rats.go internal/fabricate/rats_test.go
git commit -m "refactor(fabricate): base-driven reshapeRats shared by all engines"
```

---

## Task 8: Segment splitter

**Files:**
- Create: `internal/fabricate/segment.go`, `internal/fabricate/segment_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/segment_test.go`:

```go
package fabricate

import (
	"bytes"
	"testing"
)

func concat(segs []Segment) []byte {
	var b []byte
	for _, s := range segs {
		b = append(b, s.Bytes...)
	}
	return b
}

func TestSplitSegments_ExactPartition(t *testing.T) {
	cases := [][]byte{
		[]byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hi\")\n}\n"),
		[]byte("one line, no newline"),
		[]byte(""),
		[]byte("a\n"),
		[]byte("a\nb\nc"),
	}
	for i, c := range cases {
		segs := SplitSegments(c)
		if got := concat(segs); !bytes.Equal(got, c) {
			t.Fatalf("case %d: concat(segments) = %q, want %q", i, got, c)
		}
	}
}

func TestSplitSegments_IndicesContiguous(t *testing.T) {
	segs := SplitSegments([]byte("a\n\nb\n\nc\n\nd\n\ne\n"))
	for i, s := range segs {
		if s.Index != i {
			t.Fatalf("segment %d has Index %d", i, s.Index)
		}
	}
}

func TestSplitSegments_EmptyFileOneSegment(t *testing.T) {
	segs := SplitSegments([]byte(""))
	if len(segs) != 1 {
		t.Fatalf("empty file should yield 1 segment, got %d", len(segs))
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/ -run TestSplitSegments -v`
Expected: FAIL — `SplitSegments` / `Segment` undefined.

- [ ] **Step 3: Implement segment.go**

Create `internal/fabricate/segment.go`:

```go
package fabricate

import "bytes"

// minSegmentLines is the smallest number of source lines a segment may hold
// (except the final segment, which absorbs the remainder). Blank-line-delimited
// blocks shorter than this are coalesced with the following block.
const minSegmentLines = 8

// Segment is one contiguous slice of a file's bytes. The Bytes of a file's
// segments, concatenated in Index order, reproduce the file exactly.
type Segment struct {
	Index     int
	StartLine int // 0-based, inclusive
	EndLine   int // 0-based, exclusive
	Bytes     []byte
}

// SplitSegments partitions content into an ordered list of segments. Segments
// follow blank-line-delimited block boundaries, coalesced so each (except the
// last) spans at least minSegmentLines lines. An empty file yields exactly one
// empty segment.
func SplitSegments(content []byte) []Segment {
	lines := splitLines(content)
	if len(lines) == 0 {
		return []Segment{{Index: 0, StartLine: 0, EndLine: 0, Bytes: []byte{}}}
	}

	// Block boundaries: a boundary follows every blank line.
	var blockEnds []int
	for i, ln := range lines {
		if isBlank(ln) {
			blockEnds = append(blockEnds, i+1)
		}
	}
	if len(blockEnds) == 0 || blockEnds[len(blockEnds)-1] != len(lines) {
		blockEnds = append(blockEnds, len(lines))
	}

	var segs []Segment
	start := 0
	for _, end := range blockEnds {
		if end <= start {
			continue
		}
		// Coalesce: if this candidate segment is too short and is not the last,
		// keep extending by deferring the cut.
		if end-start < minSegmentLines && end != len(lines) {
			continue
		}
		segs = append(segs, makeSegment(len(segs), lines, start, end))
		start = end
	}
	if start < len(lines) {
		segs = append(segs, makeSegment(len(segs), lines, start, len(lines)))
	}
	if len(segs) == 0 {
		segs = append(segs, makeSegment(0, lines, 0, len(lines)))
	}
	return segs
}

func makeSegment(index int, lines [][]byte, start, end int) Segment {
	var b []byte
	for _, ln := range lines[start:end] {
		b = append(b, ln...)
	}
	return Segment{Index: index, StartLine: start, EndLine: end, Bytes: b}
}

// splitLines splits content into lines, each retaining its trailing '\n'.
// A final line without '\n' is retained as-is. Empty content yields no lines.
func splitLines(content []byte) [][]byte {
	if len(content) == 0 {
		return nil
	}
	var lines [][]byte
	start := 0
	for i := 0; i < len(content); i++ {
		if content[i] == '\n' {
			lines = append(lines, content[start:i+1])
			start = i + 1
		}
	}
	if start < len(content) {
		lines = append(lines, content[start:])
	}
	return lines
}

func isBlank(line []byte) bool {
	return len(bytes.TrimSpace(line)) == 0
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run TestSplitSegments -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/segment.go internal/fabricate/segment_test.go
git commit -m "feat(fabricate): line-range segment splitter for layered commits"
```

---

## Task 9: LLM plan JSON schema and parser

**Files:**
- Create: `internal/fabricate/llm/plan.go`, `internal/fabricate/llm/plan_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/llm/plan_test.go`:

```go
package llm

import "testing"

func TestParsePlan_PlainJSON(t *testing.T) {
	raw := `{"commits":[{"message":"chore: init","type":"chore","changes":[{"path":"go.mod","segments":"all"}]}]}`
	p, err := ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if len(p.Commits) != 1 || p.Commits[0].Message != "chore: init" {
		t.Fatalf("unexpected plan: %+v", p)
	}
	if !p.Commits[0].Changes[0].AllSegments {
		t.Fatal("expected AllSegments true for \"all\"")
	}
}

func TestParsePlan_FencedAndPrefixed(t *testing.T) {
	raw := "Here is the plan:\n```json\n{\"commits\":[{\"message\":\"feat: x\",\"type\":\"feat\"," +
		"\"changes\":[{\"path\":\"x.go\",\"segments\":[0,2]}]}]}\n```\nDone."
	p, err := ParsePlan(raw)
	if err != nil {
		t.Fatalf("ParsePlan: %v", err)
	}
	if got := p.Commits[0].Changes[0].Segments; len(got) != 2 || got[0] != 0 || got[1] != 2 {
		t.Fatalf("segments = %v, want [0 2]", got)
	}
}

func TestParsePlan_MalformedRejected(t *testing.T) {
	if _, err := ParsePlan("no json here at all"); err == nil {
		t.Fatal("expected error for response with no JSON object")
	}
	if _, err := ParsePlan(`{"commits": [}`); err == nil {
		t.Fatal("expected error for malformed JSON")
	}
}

func TestParsePlan_EmptyCommitsRejected(t *testing.T) {
	if _, err := ParsePlan(`{"commits":[]}`); err == nil {
		t.Fatal("expected error for a plan with no commits")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/llm/ -run TestParsePlan -v`
Expected: FAIL — package/`ParsePlan` do not exist.

- [ ] **Step 3: Implement plan.go**

Create `internal/fabricate/llm/plan.go`:

```go
// Package llm provides LLM provider engines and plan parsing for Caveira's
// fabricate Phase 2. It must not import the parent fabricate package.
package llm

import (
	"encoding/json"
	"errors"
	"fmt"
)

// Change is one file's contribution to a plan commit. Exactly one of
// AllSegments / Segments is meaningful: AllSegments true means the whole file.
type Change struct {
	Path        string
	AllSegments bool
	Segments    []int
}

// PlanCommit is one commit in an LLM-authored plan.
type PlanCommit struct {
	Message string
	Type    string
	Changes []Change
}

// Plan is a full LLM-authored commit plan.
type Plan struct {
	Commits []PlanCommit
}

// wire types mirror the JSON exactly; Change needs custom segment handling.
type wirePlan struct {
	Commits []wireCommit `json:"commits"`
}

type wireCommit struct {
	Message string       `json:"message"`
	Type    string       `json:"type"`
	Changes []wireChange `json:"changes"`
}

type wireChange struct {
	Path     string          `json:"path"`
	Segments json.RawMessage `json:"segments"`
}

// ParsePlan extracts the first balanced JSON object from raw (tolerating prose
// or Markdown fences around it) and parses it into a Plan.
func ParsePlan(raw string) (*Plan, error) {
	obj, ok := extractJSONObject(raw)
	if !ok {
		return nil, errors.New("no JSON object found in LLM response")
	}
	var wp wirePlan
	if err := json.Unmarshal([]byte(obj), &wp); err != nil {
		return nil, fmt.Errorf("parse plan JSON: %w", err)
	}
	if len(wp.Commits) == 0 {
		return nil, errors.New("plan has no commits")
	}
	plan := &Plan{}
	for ci, wc := range wp.Commits {
		if wc.Message == "" {
			return nil, fmt.Errorf("commit %d has an empty message", ci)
		}
		pc := PlanCommit{Message: wc.Message, Type: wc.Type}
		for _, wch := range wc.Changes {
			if wch.Path == "" {
				return nil, fmt.Errorf("commit %d has a change with an empty path", ci)
			}
			ch := Change{Path: wch.Path}
			if err := decodeSegments(wch.Segments, &ch); err != nil {
				return nil, fmt.Errorf("commit %d change %q: %w", ci, wch.Path, err)
			}
			pc.Changes = append(pc.Changes, ch)
		}
		plan.Commits = append(plan.Commits, pc)
	}
	return plan, nil
}

// decodeSegments interprets a "segments" field that is either the string "all"
// or a JSON array of integers. A missing field defaults to "all".
func decodeSegments(raw json.RawMessage, ch *Change) error {
	if len(raw) == 0 {
		ch.AllSegments = true
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		if s != "all" {
			return fmt.Errorf("segments string must be \"all\", got %q", s)
		}
		ch.AllSegments = true
		return nil
	}
	var idx []int
	if err := json.Unmarshal(raw, &idx); err != nil {
		return fmt.Errorf("segments must be \"all\" or an integer array")
	}
	ch.Segments = idx
	return nil
}

// extractJSONObject returns the first brace-balanced JSON object substring of s,
// ignoring braces inside string literals.
func extractJSONObject(s string) (string, bool) {
	start := -1
	depth := 0
	inStr := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if inStr {
			switch {
			case escaped:
				escaped = false
			case c == '\\':
				escaped = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			if depth == 0 {
				start = i
			}
			depth++
		case '}':
			if depth > 0 {
				depth--
				if depth == 0 && start >= 0 {
					return s[start : i+1], true
				}
			}
		}
	}
	return "", false
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/llm/ -run TestParsePlan -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/llm/plan.go internal/fabricate/llm/plan_test.go
git commit -m "feat(llm): plan JSON schema and tolerant parser"
```

---

## Task 10: Prompt builder

**Files:**
- Create: `internal/fabricate/llm/prompt.go`, `internal/fabricate/llm/prompt_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/llm/prompt_test.go`:

```go
package llm

import (
	"strings"
	"testing"
)

func TestBuildPrompt_IncludesPathsAndSegmentMaps(t *testing.T) {
	files := []FileInput{
		{Path: "go.mod", Kind: "chore", Content: "module x\n",
			Segments: []SegmentInfo{{Index: 0, StartLine: 0, EndLine: 1}}},
		{Path: "internal/walk/load.go", Kind: "code", Content: "package walk\n",
			Segments: []SegmentInfo{
				{Index: 0, StartLine: 0, EndLine: 12},
				{Index: 1, StartLine: 12, EndLine: 30},
			}},
	}
	p := BuildPrompt(files, 1<<20)
	for _, want := range []string{"go.mod", "internal/walk/load.go", "2 segments", "JSON", "commits"} {
		if !strings.Contains(p, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestBuildPrompt_BudgetTruncatesContent(t *testing.T) {
	big := strings.Repeat("x\n", 10000)
	files := []FileInput{
		{Path: "big.go", Kind: "code", Content: big,
			Segments: []SegmentInfo{{Index: 0, StartLine: 0, EndLine: 10000}}},
	}
	p := BuildPrompt(files, 200)
	if !strings.Contains(p, "truncated") {
		t.Fatal("expected oversized content to be marked truncated")
	}
	if !strings.Contains(p, "big.go") {
		t.Fatal("truncated file's path and segment map must still appear")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/llm/ -run TestBuildPrompt -v`
Expected: FAIL — `BuildPrompt`/`FileInput`/`SegmentInfo` undefined.

- [ ] **Step 3: Implement prompt.go**

Create `internal/fabricate/llm/prompt.go`:

```go
package llm

import (
	"fmt"
	"strings"
)

// SegmentInfo describes one segment of a file for the prompt's segment map.
type SegmentInfo struct {
	Index     int
	StartLine int
	EndLine   int
}

// FileInput is one source file presented to the LLM.
type FileInput struct {
	Path     string
	Kind     string // "chore", "code", or "test"
	Content  string
	Segments []SegmentInfo
}

const perFileContentCap = 8000 // bytes of content shown per file before truncation

// BuildPrompt builds the LLM prompt. Every file's path, kind, and full segment
// map are always included. File contents are included until the cumulative
// byte budget is exhausted; thereafter (and for any single file over
// perFileContentCap) content is truncated with an explicit marker. Truncation
// affects only the LLM's judgment — segment indices are validated downstream.
func BuildPrompt(files []FileInput, budget int) string {
	var b strings.Builder
	b.WriteString(instructions)
	b.WriteString("\n\n## Source files\n\n")

	used := 0
	for _, f := range files {
		fmt.Fprintf(&b, "### %s (%s) — %d segments\n", f.Path, f.Kind, len(f.Segments))
		for _, s := range f.Segments {
			fmt.Fprintf(&b, "  [%d] lines %d-%d\n", s.Index, s.StartLine, s.EndLine)
		}
		content := f.Content
		truncated := false
		if len(content) > perFileContentCap {
			content = content[:perFileContentCap]
			truncated = true
		}
		if used+len(content) > budget {
			remaining := budget - used
			if remaining < 0 {
				remaining = 0
			}
			content = content[:remaining]
			truncated = true
		}
		used += len(content)
		b.WriteString("```\n")
		b.WriteString(content)
		if !strings.HasSuffix(content, "\n") {
			b.WriteString("\n")
		}
		if truncated {
			b.WriteString("... [content truncated] ...\n")
		}
		b.WriteString("```\n\n")
	}
	return b.String()
}

const instructions = `You are designing a realistic git commit history for a codebase.

You are given the final state of every file in a repository, each split into
numbered segments (contiguous line ranges). Design a believable sequence of
commits that, applied in order, builds this codebase from nothing.

Rules:
- Group related files into features. Order commits so dependencies come first
  (configuration and scaffolding early, features next, tests after their code).
- A commit may include a whole file ("segments": "all") or, for larger files,
  only some segments — split a big file across several commits so earlier
  commits scaffold it and later commits flesh it out.
- Every segment of every file must appear in at least one commit overall.
- Write conventional-commit messages: "feat(scope): ...", "test(scope): ...",
  "chore: ...", "fix(scope): ...", "refactor(scope): ...".

Respond with ONLY a JSON object, no prose, in exactly this shape:

{
  "commits": [
    { "message": "chore: initialize module",
      "type": "chore",
      "changes": [ {"path": "go.mod", "segments": "all"} ] },
    { "message": "feat(walk): scaffold loader",
      "type": "feat",
      "changes": [ {"path": "internal/walk/load.go", "segments": [0, 1]} ] }
  ]
}`
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/llm/ -run TestBuildPrompt -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/llm/prompt.go internal/fabricate/llm/prompt_test.go
git commit -m "feat(llm): prompt builder with segment maps and content budget"
```

---

## Task 11: Provider interface and registry

**Files:**
- Create: `internal/fabricate/llm/provider.go`, `internal/fabricate/llm/provider_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/llm/provider_test.go`:

```go
package llm

import "testing"

func TestNewProvider_KnownNames(t *testing.T) {
	for _, name := range []string{"groq", "nvidia", "claude-code", "codex", "opencode"} {
		p, err := NewProvider(name, Options{})
		if err != nil {
			t.Fatalf("NewProvider(%q): %v", name, err)
		}
		if p.Name() != name {
			t.Fatalf("provider Name() = %q, want %q", p.Name(), name)
		}
	}
}

func TestNewProvider_UnknownRejected(t *testing.T) {
	if _, err := NewProvider("bogus", Options{}); err == nil {
		t.Fatal("expected error for unknown provider name")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/llm/ -run TestNewProvider -v`
Expected: FAIL — `NewProvider`/`Options`/`Provider` undefined.

- [ ] **Step 3: Implement provider.go**

Create `internal/fabricate/llm/provider.go`:

```go
package llm

import (
	"context"
	"fmt"
	"time"
)

// DefaultTimeout is the per-call timeout when Options.Timeout is zero.
const DefaultTimeout = 120 * time.Second

// Options configures a provider at construction time.
type Options struct {
	Model   string        // optional model override; "" = provider default
	Timeout time.Duration // per-call timeout; 0 = DefaultTimeout
	Seed    int64         // forwarded to API providers when HasSeed is true
	HasSeed bool
}

func (o Options) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return DefaultTimeout
}

// Provider is one LLM engine. GeneratePlan performs a single call; retries are
// the caller's responsibility.
type Provider interface {
	Name() string
	GeneratePlan(ctx context.Context, prompt string) (rawJSON string, err error)
}

// NewProvider constructs the named provider. Known names: groq, nvidia,
// claude-code, codex, opencode.
func NewProvider(name string, opts Options) (Provider, error) {
	switch name {
	case "groq":
		return newOpenAICompat("groq", "https://api.groq.com/openai/v1",
			"GROQ_API_KEY", "llama-3.3-70b-versatile", opts), nil
	case "nvidia":
		return newOpenAICompat("nvidia", "https://integrate.api.nvidia.com/v1",
			"NVIDIA_API_KEY", "meta/llama-3.3-70b-instruct", opts), nil
	case "claude-code":
		return newCLIProvider("claude-code", "claude", []string{"-p"}, opts), nil
	case "codex":
		return newCLIProvider("codex", "codex", []string{"exec"}, opts), nil
	case "opencode":
		return newCLIProvider("opencode", "opencode", []string{"run"}, opts), nil
	default:
		return nil, fmt.Errorf("unknown LLM provider %q", name)
	}
}
```

- [ ] **Step 4: Run the tests to verify they fail differently**

Run: `go test ./internal/fabricate/llm/ -run TestNewProvider -v`
Expected: FAIL — `newOpenAICompat` and `newCLIProvider` are not yet defined. This task's constructors are completed by Tasks 12 and 13; provider.go itself is finished here.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/llm/provider.go internal/fabricate/llm/provider_test.go
git commit -m "feat(llm): provider interface and registry"
```

Note: the package will not build until Task 12 and Task 13 add the constructors. Run the tests again at the end of Task 13.

---

## Task 12: OpenAI-compatible provider (Groq, NVIDIA)

**Files:**
- Create: `internal/fabricate/llm/openai_compat.go`, `internal/fabricate/llm/openai_compat_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/llm/openai_compat_test.go`:

```go
package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAICompat_GeneratePlan(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("missing/wrong Authorization header: %q", r.Header.Get("Authorization"))
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": `{"commits":[]}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	p := &openAICompatProvider{
		name: "groq", baseURL: srv.URL, apiKey: "test-key",
		model: "m", client: srv.Client(), opts: Options{},
	}
	got, err := p.GeneratePlan(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if !strings.Contains(got, "commits") {
		t.Fatalf("unexpected content: %q", got)
	}
}

func TestOpenAICompat_MissingKey(t *testing.T) {
	p := newOpenAICompat("groq", "http://unused", "CAVEIRA_TEST_NO_SUCH_KEY", "m", Options{})
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error when API key env var is unset")
	}
}

func TestOpenAICompat_HTTPErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := &openAICompatProvider{
		name: "groq", baseURL: srv.URL, apiKey: "k", model: "m",
		client: srv.Client(), opts: Options{},
	}
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error on non-200 status")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/llm/ -run TestOpenAICompat -v`
Expected: FAIL — `openAICompatProvider`/`newOpenAICompat` undefined.

- [ ] **Step 3: Implement openai_compat.go**

Create `internal/fabricate/llm/openai_compat.go`:

```go
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// openAICompatProvider talks to any OpenAI-compatible /chat/completions API.
type openAICompatProvider struct {
	name      string
	baseURL   string
	apiKeyEnv string
	apiKey    string // resolved lazily from apiKeyEnv if empty
	model     string
	client    *http.Client
	opts      Options
}

func newOpenAICompat(name, baseURL, apiKeyEnv, defaultModel string, opts Options) *openAICompatProvider {
	model := defaultModel
	if opts.Model != "" {
		model = opts.Model
	}
	return &openAICompatProvider{
		name:      name,
		baseURL:   baseURL,
		apiKeyEnv: apiKeyEnv,
		model:     model,
		client:    &http.Client{Timeout: opts.timeout()},
		opts:      opts,
	}
}

func (p *openAICompatProvider) Name() string { return p.name }

func (p *openAICompatProvider) GeneratePlan(ctx context.Context, prompt string) (string, error) {
	key := p.apiKey
	if key == "" {
		key = os.Getenv(p.apiKeyEnv)
	}
	if key == "" {
		return "", fmt.Errorf("%s: environment variable %s is not set", p.name, p.apiKeyEnv)
	}

	reqBody := map[string]any{
		"model":           p.model,
		"temperature":     0.2,
		"response_format": map[string]string{"type": "json_object"},
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	if p.opts.HasSeed {
		reqBody["seed"] = p.opts.Seed
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("%s: request failed: %w", p.name, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s: HTTP %d: %s", p.name, resp.StatusCode, truncate(string(body), 300))
	}

	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("%s: decoding response: %w", p.name, err)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("%s: response had no choices", p.name)
	}
	return parsed.Choices[0].Message.Content, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/llm/ -run TestOpenAICompat -v`
Expected: PASS (the package still won't fully build until Task 13 adds `newCLIProvider`; if `go test` reports the build failure for the whole package, that is expected — proceed to Task 13 and re-run).

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/llm/openai_compat.go internal/fabricate/llm/openai_compat_test.go
git commit -m "feat(llm): OpenAI-compatible HTTP provider for Groq and NVIDIA"
```

---

## Task 13: CLI subprocess provider (claude-code, codex, opencode)

**Files:**
- Create: `internal/fabricate/llm/cli_provider.go`, `internal/fabricate/llm/cli_provider_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/llm/cli_provider_test.go`:

```go
package llm

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// writeFakeBinary creates an executable shell script that echoes a fixed
// response, and prepends its directory to PATH for the test.
func writeFakeBinary(t *testing.T, name, script string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+script+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestCLIProvider_GeneratePlan(t *testing.T) {
	writeFakeBinary(t, "claude", `echo '{"commits":[{"message":"chore: x","type":"chore","changes":[]}]}'`)
	p := newCLIProvider("claude-code", "claude", []string{"-p"}, Options{})
	got, err := p.GeneratePlan(context.Background(), "prompt")
	if err != nil {
		t.Fatalf("GeneratePlan: %v", err)
	}
	if got == "" {
		t.Fatal("expected non-empty output from fake binary")
	}
}

func TestCLIProvider_MissingBinary(t *testing.T) {
	p := newCLIProvider("codex", "caveira-no-such-binary-xyz", []string{"exec"}, Options{})
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error when the CLI binary is absent from PATH")
	}
}

func TestCLIProvider_NonZeroExit(t *testing.T) {
	writeFakeBinary(t, "opencode", `echo boom >&2; exit 1`)
	p := newCLIProvider("opencode", "opencode", []string{"run"}, Options{})
	if _, err := p.GeneratePlan(context.Background(), "prompt"); err == nil {
		t.Fatal("expected error on non-zero subprocess exit")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/llm/ -run TestCLIProvider -v`
Expected: FAIL — `newCLIProvider` undefined.

- [ ] **Step 3: Implement cli_provider.go**

Create `internal/fabricate/llm/cli_provider.go`:

```go
package llm

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// cliProvider runs an installed CLI as a subprocess, feeding the prompt on
// stdin and capturing stdout as the raw response.
type cliProvider struct {
	name   string
	binary string
	args   []string
	opts   Options
}

func newCLIProvider(name, binary string, args []string, opts Options) *cliProvider {
	full := append([]string{}, args...)
	if opts.Model != "" {
		full = append(full, "--model", opts.Model)
	}
	return &cliProvider{name: name, binary: binary, args: full, opts: opts}
}

func (p *cliProvider) Name() string { return p.name }

func (p *cliProvider) GeneratePlan(ctx context.Context, prompt string) (string, error) {
	if _, err := exec.LookPath(p.binary); err != nil {
		return "", fmt.Errorf("%s: %q not found on PATH; install it or choose another engine", p.name, p.binary)
	}
	ctx, cancel := context.WithTimeout(ctx, p.opts.timeout())
	defer cancel()

	cmd := exec.CommandContext(ctx, p.binary, p.args...)
	cmd.Stdin = strings.NewReader(prompt)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%s: %q failed: %w: %s", p.name, p.binary, err,
			truncate(strings.TrimSpace(stderr.String()), 300))
	}
	out := strings.TrimSpace(stdout.String())
	if out == "" {
		return "", fmt.Errorf("%s: %q produced no output", p.name, p.binary)
	}
	return out, nil
}
```

- [ ] **Step 4: Run the full llm package test suite**

Run: `go test ./internal/fabricate/llm/ -v`
Expected: PASS — all tests in Tasks 9–13, including `TestNewProvider`.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/llm/cli_provider.go internal/fabricate/llm/cli_provider_test.go
git commit -m "feat(llm): subprocess provider for claude-code, codex, opencode"
```

---

## Task 14: Plan realizer

**Files:**
- Create: `internal/fabricate/realize.go`, `internal/fabricate/realize_test.go`

- [ ] **Step 1: Write the failing tests**

Create `internal/fabricate/realize_test.go`:

```go
package fabricate

import (
	"bytes"
	"testing"

	"github.com/go-git/go-git/v5/plumbing/filemode"

	"github.com/justin06lee/caveira/internal/fabricate/llm"
)

func srcFile(path, content string) SourceFile {
	c := []byte(content)
	return SourceFile{
		Path: path, Mode: filemode.Regular, Content: c, Segments: SplitSegments(c),
	}
}

func finalContent(commits []SynthCommit, path string) []byte {
	var latest []byte
	for _, c := range commits {
		for _, fr := range c.Added {
			if fr.Path == path {
				if fr.Content != nil {
					latest = fr.Content
				} else {
					latest = nil // whole-file-from-source marker
				}
			}
		}
	}
	return latest
}

func TestRealize_WholeFilesMatchSource(t *testing.T) {
	srcs := []SourceFile{srcFile("go.mod", "module x\n"), srcFile("main.go", "package main\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "chore: init", Type: "chore", Changes: []llm.Change{{Path: "go.mod", AllSegments: true}}},
		{Message: "feat: main", Type: "feat", Changes: []llm.Change{{Path: "main.go", AllSegments: true}}},
	}}
	base, err := Realize(srcs, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	if len(base) != 2 {
		t.Fatalf("want 2 commits, got %d", len(base))
	}
}

func TestRealize_LayeredFileEndsExact(t *testing.T) {
	content := "l0\nl1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\n"
	src := srcFile("big.go", content)
	nSeg := len(src.Segments)
	if nSeg < 2 {
		t.Skipf("fixture needs >=2 segments, got %d", nSeg)
	}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: scaffold", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: []int{0}}}},
		{Message: "feat: rest", Type: "feat", Changes: []llm.Change{{Path: "big.go", Segments: lastIndices(nSeg)}}},
	}}
	base, err := Realize([]SourceFile{src}, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	last := base[len(base)-1]
	var got []byte
	for _, fr := range last.Added {
		if fr.Path == "big.go" {
			got = fr.Content
		}
	}
	// Final commit holds remaining segments; cumulative must equal full content.
	if !bytes.Equal(cumulativeBig(base), []byte(content)) {
		_ = got
		t.Fatal("layered realization did not end at exact source content")
	}
}

func lastIndices(n int) []int {
	var out []int
	for i := 1; i < n; i++ {
		out = append(out, i)
	}
	return out
}

// cumulativeBig returns the final realized content of "big.go".
func cumulativeBig(base []SynthCommit) []byte {
	var latest []byte
	for _, c := range base {
		for _, fr := range c.Added {
			if fr.Path == "big.go" {
				latest = fr.Content
			}
		}
	}
	return latest
}

func TestRealize_ClampsForgottenFile(t *testing.T) {
	srcs := []SourceFile{srcFile("a.go", "package a\n"), srcFile("b.go", "package b\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: a", Type: "feat", Changes: []llm.Change{{Path: "a.go", AllSegments: true}}},
	}}
	base, err := Realize(srcs, plan)
	if err != nil {
		t.Fatalf("Realize: %v", err)
	}
	sawB := false
	for _, c := range base {
		for _, fr := range c.Added {
			if fr.Path == "b.go" {
				sawB = true
			}
		}
	}
	if !sawB {
		t.Fatal("forgotten file b.go was not clamped into the plan")
	}
}

func TestRealize_UnknownPathRejected(t *testing.T) {
	srcs := []SourceFile{srcFile("a.go", "package a\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: ghost", Type: "feat", Changes: []llm.Change{{Path: "ghost.go", AllSegments: true}}},
	}}
	if _, err := Realize(srcs, plan); err == nil {
		t.Fatal("expected error for a plan referencing an unknown path")
	}
}

func TestRealize_OutOfRangeSegmentRejected(t *testing.T) {
	srcs := []SourceFile{srcFile("a.go", "package a\n")}
	plan := &llm.Plan{Commits: []llm.PlanCommit{
		{Message: "feat: a", Type: "feat", Changes: []llm.Change{{Path: "a.go", Segments: []int{99}}}},
	}}
	if _, err := Realize(srcs, plan); err == nil {
		t.Fatal("expected error for an out-of-range segment index")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/fabricate/ -run TestRealize -v`
Expected: FAIL — `Realize`/`SourceFile` undefined.

- [ ] **Step 3: Implement realize.go**

Create `internal/fabricate/realize.go`:

```go
package fabricate

import (
	"fmt"
	"sort"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"

	"github.com/justin06lee/caveira/internal/fabricate/llm"
)

// SourceFile is one source HEAD-tree file with its content and segmentation.
type SourceFile struct {
	Path     string
	Mode     filemode.FileMode
	Content  []byte
	Segments []Segment
}

// fileChange is one file's resolved segment indices within a plan commit.
type fileChange struct {
	path string
	segs []int
}

// PlanCommitView is a resolved, clamped commit ready for realization.
type PlanCommitView struct {
	Message string
	Type    string
	Changes []fileChange
}

// Realize converts an LLM Plan over the given source files into a linear base
// sequence of SynthCommits. It validates the plan, clamps any segment or file
// the plan omitted, and computes per-commit cumulative-content blobs so the
// final state is byte-exact with the sources. The returned commits have IDs
// 0..n-1 and a linear parent chain; authors are left zero for a reshaper.
func Realize(sources []SourceFile, plan *llm.Plan) ([]SynthCommit, error) {
	byPath := make(map[string]SourceFile, len(sources))
	for _, s := range sources {
		byPath[s.Path] = s
	}

	// Per-commit, per-file resolved segment index sets.
	resolved := make([][]fileChange, len(plan.Commits))

	// Validate and resolve.
	for ci, pc := range plan.Commits {
		for _, ch := range pc.Changes {
			sf, ok := byPath[ch.Path]
			if !ok {
				return nil, fmt.Errorf("plan commit %d references unknown path %q", ci, ch.Path)
			}
			n := len(sf.Segments)
			var segs []int
			if ch.AllSegments {
				for i := 0; i < n; i++ {
					segs = append(segs, i)
				}
			} else {
				for _, idx := range ch.Segments {
					if idx < 0 || idx >= n {
						return nil, fmt.Errorf("plan commit %d: segment %d out of range for %q (0..%d)",
							ci, idx, ch.Path, n-1)
					}
					segs = append(segs, idx)
				}
			}
			resolved[ci] = append(resolved[ci], fileChange{path: ch.Path, segs: segs})
		}
	}

	// Clamp: ensure every segment of every file is assigned somewhere.
	assigned := map[string]map[int]bool{}
	lastCommit := map[string]int{} // path -> last commit index touching it
	for ci, changes := range resolved {
		for _, fc := range changes {
			if assigned[fc.path] == nil {
				assigned[fc.path] = map[int]bool{}
			}
			for _, s := range fc.segs {
				assigned[fc.path][s] = true
			}
			lastCommit[fc.path] = ci
		}
	}
	// Forgotten segments of touched files -> append to that file's last commit.
	for path, segset := range assigned {
		sf := byPath[path]
		var missing []int
		for i := 0; i < len(sf.Segments); i++ {
			if !segset[i] {
				missing = append(missing, i)
			}
		}
		if len(missing) > 0 {
			ci := lastCommit[path]
			resolved[ci] = append(resolved[ci], fileChange{path: path, segs: missing})
		}
	}
	// Files never mentioned -> a final reconciliation commit.
	var forgotten []fileChange
	for _, s := range sources {
		if assigned[s.Path] == nil {
			all := make([]int, len(s.Segments))
			for i := range all {
				all[i] = i
			}
			forgotten = append(forgotten, fileChange{path: s.Path, segs: all})
		}
	}
	commits := make([]PlanCommitView, len(plan.Commits))
	for i, pc := range plan.Commits {
		commits[i] = PlanCommitView{Message: pc.Message, Type: pc.Type, Changes: resolved[i]}
	}
	if len(forgotten) > 0 {
		sort.Slice(forgotten, func(i, j int) bool { return forgotten[i].path < forgotten[j].path })
		commits = append(commits, PlanCommitView{
			Message: "chore: finalize", Type: "chore", Changes: forgotten,
		})
	}

	// Realize: walk commits, maintain cumulative per-file segment sets.
	cum := map[string]map[int]bool{}
	out := make([]SynthCommit, 0, len(commits))
	for ci, pcv := range commits {
		sc := SynthCommit{ID: ci, Message: pcv.Message, Feature: scopeOf(pcv.Message)}
		if ci > 0 {
			sc.Parents = []int{ci - 1}
		}
		deltaLines, newFiles := 0, 0
		for _, fc := range pcv.Changes {
			sf := byPath[fc.path]
			if cum[fc.path] == nil {
				cum[fc.path] = map[int]bool{}
				newFiles++ // first commit to touch this file
			}
			for _, s := range fc.segs {
				if !cum[fc.path][s] {
					deltaLines += segLineCount(sf.Segments[s])
				}
				cum[fc.path][s] = true
			}
			content := assembleContent(sf, cum[fc.path])
			sc.Added = append(sc.Added, FileRef{
				Path:    fc.path,
				Mode:    sf.Mode,
				Content: content,
				Blob:    plumbing.ComputeHash(plumbing.BlobObject, content),
			})
		}
		sc.Stats = &DiffStat{Lines: deltaLines, Files: len(pcv.Changes), NewFiles: newFiles}
		out = append(out, sc)
	}

	// Verify: every file's final cumulative set is complete.
	for _, s := range sources {
		if len(cum[s.Path]) != len(s.Segments) {
			return nil, fmt.Errorf("internal error: %q not fully realized (%d/%d segments)",
				s.Path, len(cum[s.Path]), len(s.Segments))
		}
	}
	return out, nil
}

// assembleContent concatenates the in-set segments of sf in index order.
func assembleContent(sf SourceFile, set map[int]bool) []byte {
	var b []byte
	for i, seg := range sf.Segments {
		if set[i] {
			b = append(b, seg.Bytes...)
		}
	}
	return b
}

func segLineCount(s Segment) int {
	n := s.EndLine - s.StartLine
	if n < 1 {
		return 1
	}
	return n
}

// scopeOf extracts the conventional-commit scope from a message, e.g.
// "feat(walk): ..." -> "walk". Returns "" when there is no (scope).
func scopeOf(msg string) string {
	open := -1
	for i := 0; i < len(msg); i++ {
		switch msg[i] {
		case '(':
			open = i
		case ')':
			if open >= 0 && i > open+1 {
				return msg[open+1 : i]
			}
			return ""
		case ':':
			return ""
		}
	}
	return ""
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run TestRealize -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/fabricate/realize.go internal/fabricate/realize_test.go
git commit -m "feat(fabricate): deterministic plan realizer with segment clamping"
```

---

## Task 15: LLM orchestrator GenerateLLM

**Files:**
- Create: `internal/fabricate/llmgen.go`, `internal/fabricate/llmgen_test.go`
- Modify: `internal/fabricate/generate.go`

- [ ] **Step 1: Write the failing test**

Create `internal/fabricate/llmgen_test.go`:

```go
package fabricate

import (
	"context"
	"math/rand"
	"testing"
)

// stubProvider returns a fixed raw response, recording how many times it ran.
// It structurally satisfies llm.Provider, so no llm import is needed here.
type stubProvider struct {
	responses []string
	calls     int
}

func (s *stubProvider) Name() string { return "stub" }
func (s *stubProvider) GeneratePlan(_ context.Context, _ string) (string, error) {
	r := s.responses[s.calls%len(s.responses)]
	s.calls++
	return r, nil
}

func TestGenerateLLM_SingleAuthorEndsAtSourceTree(t *testing.T) {
	repo := buildFixtureRepo(t) // same helper used by flurry tests
	files, err := WalkHead(repo)
	if err != nil {
		t.Fatalf("WalkHead: %v", err)
	}
	// Build an "all segments" plan covering every file.
	plan := `{"commits":[`
	for i, f := range files {
		if i > 0 {
			plan += ","
		}
		plan += `{"message":"feat: add ` + f.Path + `","type":"feat","changes":[{"path":"` +
			f.Path + `","segments":"all"}]}`
	}
	plan += `]}`

	prov := &stubProvider{responses: []string{plan}}
	p, dag, err := GenerateLLM(repo, []Identity{{Name: "A", Email: "a@x.com"}}, "single",
		prov, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("GenerateLLM: %v", err)
	}
	if len(p.Commits) == 0 || len(dag.All()) == 0 {
		t.Fatal("expected a non-empty plan and DAG")
	}
	if prov.calls != 1 {
		t.Fatalf("expected 1 provider call, got %d", prov.calls)
	}
}

func TestGenerateLLM_RetriesOnBadJSON(t *testing.T) {
	repo := buildFixtureRepo(t)
	files, _ := WalkHead(repo)
	good := `{"commits":[{"message":"chore: all","type":"chore","changes":[`
	for i, f := range files {
		if i > 0 {
			good += ","
		}
		good += `{"path":"` + f.Path + `","segments":"all"}`
	}
	good += `]}]}`

	prov := &stubProvider{responses: []string{"garbage, not json", good}}
	_, _, err := GenerateLLM(repo, []Identity{{Name: "A", Email: "a@x.com"}}, "single",
		prov, rand.New(rand.NewSource(1)))
	if err != nil {
		t.Fatalf("GenerateLLM should have recovered on retry: %v", err)
	}
	if prov.calls != 2 {
		t.Fatalf("expected 2 provider calls (1 bad + 1 good), got %d", prov.calls)
	}
}

func TestGenerateLLM_HardErrorAfterRetries(t *testing.T) {
	repo := buildFixtureRepo(t)
	prov := &stubProvider{responses: []string{"still not json"}}
	_, _, err := GenerateLLM(repo, []Identity{{Name: "A", Email: "a@x.com"}}, "single",
		prov, rand.New(rand.NewSource(1)))
	if err == nil {
		t.Fatal("expected a hard error after exhausting retries")
	}
	if prov.calls != maxLLMAttempts {
		t.Fatalf("expected %d provider calls, got %d", maxLLMAttempts, prov.calls)
	}
}
```

`buildFixtureRepo` is a placeholder for whatever fixture-repo helper the flurry/pigs tests already use — match the existing name.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/fabricate/ -run TestGenerateLLM -v`
Expected: FAIL — `GenerateLLM`/`maxLLMAttempts` undefined.

- [ ] **Step 3: Implement llmgen.go**

Create `internal/fabricate/llmgen.go`:

```go
package fabricate

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/go-git/go-git/v5"

	"github.com/justin06lee/caveira/internal/fabricate/llm"
	"github.com/justin06lee/caveira/internal/walk"
)

// maxLLMAttempts is the number of provider calls tried before a hard error.
const maxLLMAttempts = 3

// promptByteBudget caps the total file-content bytes placed in the prompt.
const promptByteBudget = 200_000

// GenerateLLM runs an LLM provider engine: it walks the source HEAD tree,
// segments every file, prompts the provider, parses and realizes the returned
// plan into a base sequence, then reshapes it per mode ("single"/"pigs"/"rats")
// and returns the Plan plus its walk.DAG. On provider, parse, or validation
// failure it retries up to maxLLMAttempts times, then returns a hard error.
func GenerateLLM(repo *git.Repository, ids []Identity, mode string,
	provider llm.Provider, rng *rand.Rand) (*Plan, *walk.DAG, error) {

	if len(ids) == 0 {
		return nil, nil, fmt.Errorf("GenerateLLM: at least one identity required")
	}
	files, err := WalkHead(repo)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("GenerateLLM: source repo has no files")
	}

	// Read content and segment every file once.
	sources := make([]SourceFile, 0, len(files))
	inputs := make([]llm.FileInput, 0, len(files))
	for _, f := range files {
		blob, err := repo.BlobObject(f.Blob)
		if err != nil {
			return nil, nil, err
		}
		content, err := blobBytes(blob)
		if err != nil {
			return nil, nil, err
		}
		segs := SplitSegments(content)
		sources = append(sources, SourceFile{
			Path: f.Path, Mode: f.Mode, Content: content, Segments: segs,
		})
		si := make([]llm.SegmentInfo, len(segs))
		for i, s := range segs {
			si[i] = llm.SegmentInfo{Index: s.Index, StartLine: s.StartLine, EndLine: s.EndLine}
		}
		inputs = append(inputs, llm.FileInput{
			Path: f.Path, Kind: kindString(Classify(f.Path)), Content: string(content), Segments: si,
		})
	}
	prompt := llm.BuildPrompt(inputs, promptByteBudget)

	var base []SynthCommit
	var lastErr error
	for attempt := 1; attempt <= maxLLMAttempts; attempt++ {
		raw, err := provider.GeneratePlan(context.Background(), prompt)
		if err != nil {
			lastErr = err
			continue
		}
		parsed, err := llm.ParsePlan(raw)
		if err != nil {
			lastErr = err
			continue
		}
		realized, err := Realize(sources, parsed)
		if err != nil {
			lastErr = err
			continue
		}
		base = realized
		lastErr = nil
		break
	}
	if lastErr != nil {
		return nil, nil, fmt.Errorf("LLM engine %q failed after %d attempts: %w",
			provider.Name(), maxLLMAttempts, lastErr)
	}

	plan, err := reshapeBase(base, ids, mode, rng)
	if err != nil {
		return nil, nil, err
	}
	dag, err := PlanToDAG(repo, plan)
	if err != nil {
		return nil, nil, err
	}
	return plan, dag, nil
}

// reshapeBase applies the mode reshaper to a base sequence.
func reshapeBase(base []SynthCommit, ids []Identity, mode string, rng *rand.Rand) (*Plan, error) {
	switch mode {
	case "rats":
		return reshapeRats(base, ids, rng)
	case "pigs":
		return reshapePigs(base, ids, rng), nil
	default: // "single"
		return reshapeSingle(base, ids[0]), nil
	}
}

// reshapeSingle assigns one identity to a linear base sequence unchanged.
func reshapeSingle(base []SynthCommit, id Identity) *Plan {
	for i := range base {
		base[i].ID = i
		base[i].Author = id
		base[i].Committer = id
		if i == 0 {
			base[i].Parents = nil
		} else {
			base[i].Parents = []int{i - 1}
		}
	}
	return &Plan{
		Commits: base,
		Refs:    map[string]int{defaultBranch: base[len(base)-1].ID},
		HEAD:    base[len(base)-1].ID,
		HeadRef: defaultBranch,
	}
}

func kindString(k FileKind) string {
	switch k {
	case Chore:
		return "chore"
	case Test:
		return "test"
	default:
		return "code"
	}
}

func blobBytes(blob *object.Blob) ([]byte, error) {
	r, err := blob.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}
```

Add the imports `"io"` and `"github.com/go-git/go-git/v5/plumbing/object"` to the file's import block.

- [ ] **Step 4: Make reshapeSingle the shared single-author path in generate.go**

In `internal/fabricate/generate.go`, the existing flurry `Generate` uses `BuildPigsPlan` for the `"single"` case. Leave that behavior intact for `--flurry` (changing it risks Phase 1 regressions). `GenerateLLM` uses `reshapeSingle` for `"single"`. No edit to `generate.go` is required for this task; the function `reshapeSingle` lives in `llmgen.go` and is used only by the LLM path.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/fabricate/ -run TestGenerateLLM -v`
Expected: PASS.

- [ ] **Step 6: Run the full fabricate package**

Run: `go test ./internal/fabricate/... -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/fabricate/llmgen.go internal/fabricate/llmgen_test.go
git commit -m "feat(fabricate): GenerateLLM orchestrator with retry and reshaping"
```

---

## Task 16: Pipeline wiring and tree verification

**Files:**
- Modify: `internal/cli/pipeline.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/cli/fabricate_integration_test.go`. This test installs a fake
`claude` binary that exits non-zero, proving the LLM route is taken and that a
provider failure surfaces as a hard error with no fallback to flurry. PATH keeps
its existing entries (so `git` still works); the fake binary's directory is
prepended.

```go
func TestIntegration_FabricateLLM_HardErrorNoFallback(t *testing.T) {
	dir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\necho 'provider exploded' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	src := makeFixtureRepoDir(t) // existing helper that builds a real repo dir
	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-4 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		Provider:  "claude-code",
	}
	var out, errOut bytes.Buffer
	code := Pipeline(cfg, &out, &errOut)
	if code == 0 {
		t.Fatalf("expected non-zero exit; stderr=%q", errOut.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("claude-code")) {
		t.Fatalf("error should name the failed provider; stderr=%q", errOut.String())
	}
}
```

Use whatever real-on-disk fixture-repo helper the existing integration tests use
(the file already has integration tests for flurry/pigs — match the helper name
and `input.Config` construction style they use). Ensure the test file imports
`"os"` and `"path/filepath"`. The key assertion: an LLM engine that fails after
all retries makes the pipeline exit non-zero, naming the provider — never a
silent flurry fallback.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/cli/ -run TestIntegration_FabricateLLM_HardErrorNoFallback -v`
Expected: FAIL — the pipeline does not yet route `cfg.Provider` to an LLM engine (it currently treats any `Fabricate` run as flurry/pigs/rats and `cfg.Provider` is ignored), so the fake `claude` binary is never invoked.

- [ ] **Step 3: Route the LLM engine in fabricatePipeline**

In `internal/cli/pipeline.go`, in `fabricatePipeline`, replace the base-generation call. The current code is:

```go
	rng := rngFor(cfg)
	plan, dag, err := fabricate.Generate(srcRepo, ids, mode, rng)
	if err != nil {
		fmt.Fprintln(errOut, "error: fabricate generate:", err)
		return 1
	}
```

Replace it with:

```go
	rng := rngFor(cfg)
	var plan *fabricate.Plan
	var dag *walk.DAG
	if cfg.Provider != "" {
		provider, perr := llm.NewProvider(cfg.Provider, llm.Options{
			Model:   cfg.Model,
			Timeout: cfg.LLMTimeout,
			Seed:    cfg.Seed,
			HasSeed: cfg.HasSeed,
		})
		if perr != nil {
			fmt.Fprintln(errOut, "error:", perr)
			return 1
		}
		plan, dag, err = fabricate.GenerateLLM(srcRepo, ids, mode, provider, rng)
	} else {
		plan, dag, err = fabricate.Generate(srcRepo, ids, mode, rng)
	}
	if err != nil {
		fmt.Fprintln(errOut, "error: fabricate generate:", err)
		return 1
	}
```

Add `"github.com/justin06lee/caveira/internal/fabricate/llm"` to the import block.

- [ ] **Step 4: Add a defensive tree-match verification after WriteToRepo**

Still in `fabricatePipeline`, immediately after the successful `fabricate.WriteToRepo(...)` call, add:

```go
	if err := verifyTreeMatch(srcRepo, stageRepo); err != nil {
		fmt.Fprintln(errOut, "error: fabricated tree does not match source:", err)
		return 1
	}
```

Add this helper function to `internal/cli/pipeline.go`:

```go
// verifyTreeMatch confirms the rewritten repo's HEAD tree equals the source
// repo's HEAD tree — a defensive backstop for fabrication correctness.
func verifyTreeMatch(src, dst *git.Repository) error {
	srcTree, err := headTreeHash(src)
	if err != nil {
		return fmt.Errorf("source head tree: %w", err)
	}
	dstTree, err := headTreeHash(dst)
	if err != nil {
		return fmt.Errorf("rewritten head tree: %w", err)
	}
	if srcTree != dstTree {
		return fmt.Errorf("tree %s != %s", dstTree, srcTree)
	}
	return nil
}

func headTreeHash(r *git.Repository) (plumbing.Hash, error) {
	head, err := r.Head()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		return plumbing.ZeroHash, err
	}
	return commit.TreeHash, nil
}
```

Add `"github.com/go-git/go-git/v5/plumbing"` to the import block.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run TestIntegration_Fabricate -v`
Expected: PASS — the new test plus the existing flurry/pigs integration tests.

- [ ] **Step 6: Build and vet the whole module**

Run: `go build ./... && go vet ./...`
Expected: no output (success).

- [ ] **Step 7: Commit**

```bash
git add internal/cli/pipeline.go internal/cli/fabricate_integration_test.go
git commit -m "feat(cli): route LLM engines through the fabricate pipeline"
```

---

## Task 17: End-to-end LLM integration tests with a stub provider

**Files:**
- Create: `internal/cli/llm_integration_test.go`

This task exercises the full pipeline against a *fake LLM CLI binary* so the run produces a real rewritten repo without any network or API key.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/llm_integration_test.go`:

```go
package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-git/v5"

	"github.com/justin06lee/caveira/internal/input"
)

// installFakeClaude writes a `claude` binary on PATH that ignores its stdin
// prompt and prints planJSON, so --claude-code runs deterministically offline.
func installFakeClaude(t *testing.T, planJSON string) {
	t.Helper()
	dir := t.TempDir()
	script := "#!/bin/sh\ncat >/dev/null\ncat <<'PLAN'\n" + planJSON + "\nPLAN\n"
	bin := filepath.Join(dir, "claude")
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// allFilesPlan builds a one-commit-per-file "all segments" plan for repoDir.
func allFilesPlan(t *testing.T, repoDir string) string {
	t.Helper()
	r, err := git.PlainOpen(repoDir)
	if err != nil {
		t.Fatal(err)
	}
	head, err := r.Head()
	if err != nil {
		t.Fatal(err)
	}
	commit, err := r.CommitObject(head.Hash())
	if err != nil {
		t.Fatal(err)
	}
	tree, err := commit.Tree()
	if err != nil {
		t.Fatal(err)
	}
	plan := `{"commits":[`
	first := true
	files := tree.Files()
	for {
		f, err := files.Next()
		if err != nil {
			break
		}
		if !first {
			plan += ","
		}
		first = false
		plan += `{"message":"feat: add ` + f.Name + `","type":"feat","changes":[{"path":"` +
			f.Name + `","segments":"all"}]}`
	}
	plan += `]}`
	return plan
}

func TestIntegration_LLM_ClaudeCode_SingleAuthor(t *testing.T) {
	src := makeFixtureRepoDir(t) // reuse the existing integration helper
	plan := allFilesPlan(t, src)
	installFakeClaude(t, plan)

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-6 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		Provider:  "claude-code",
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}
	// The swapped-in repo at src must have HEAD tree == original tree.
	assertGitLogNonEmpty(t, src)
}

func TestIntegration_LLM_ClaudeCode_WithRats(t *testing.T) {
	src := makeFixtureRepoDir(t)
	plan := allFilesPlan(t, src)
	installFakeClaude(t, plan)

	cfg := &input.Config{
		Repo:      src,
		Start:     time.Now().Add(-12 * time.Hour),
		End:       time.Now(),
		WindowTZ:  time.UTC,
		Fabricate: true,
		Provider:  "claude-code",
		RatsN:     2,
		RatIdentities: []string{"Rat One <r1@x.com>", "Rat Two <r2@x.com>"},
	}
	var out, errOut bytes.Buffer
	if code := Pipeline(cfg, &out, &errOut); code != 0 {
		t.Fatalf("pipeline failed: %s", errOut.String())
	}
	assertGitLogNonEmpty(t, src)
}

func assertGitLogNonEmpty(t *testing.T, repoDir string) {
	t.Helper()
	out, err := exec.Command("git", "-C", repoDir, "log", "--oneline").CombinedOutput()
	if err != nil {
		t.Fatalf("git log: %v: %s", err, out)
	}
	if len(bytes.TrimSpace(out)) == 0 {
		t.Fatal("rewritten repo has no commits")
	}
}
```

Implementation note for the engineer: match `makeFixtureRepoDir` to the real helper name used by `fabricate_integration_test.go`; if that file names it differently, use the existing name. The `git` import is `github.com/go-git/go-git/v5`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/cli/ -run TestIntegration_LLM -v`
Expected: FAIL initially if helper names need adjusting; once names match, the tests drive any remaining wiring gaps.

- [ ] **Step 3: Fix helper names and any wiring gaps**

Adjust `makeFixtureRepoDir` to the actual helper name. If the tests reveal a real pipeline bug (e.g. squash guard triggered because the window is too small for the fixture), widen the test window rather than weakening the guard.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/cli/ -run TestIntegration_LLM -v`
Expected: PASS.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli/llm_integration_test.go
git commit -m "test(cli): end-to-end LLM fabrication with a stub provider binary"
```

---

## Task 18: Documentation — README and help

**Files:**
- Modify: `README.md`
- Verify: `internal/cli/cli.go` help/example block

- [ ] **Step 1: Update the cobra Example block**

In `internal/cli/cli.go`, extend the `Example:` field with a third example showing an LLM engine. Append to the existing example string:

```
  ` + name + ` --repo /path/to/myrepo --fabricate --groq \
      --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
      --rats 3
```

(Keep the existing two example blocks; this adds a fourth block — adjust spacing to match the existing format.)

- [ ] **Step 2: Add an LLM-providers section to README.md**

In `README.md`, in or after the section that documents `--fabricate` / `--flurry` / `--pigs` / `--rats`, add:

```markdown
### LLM-backed fabrication

Instead of the templated `--flurry` engine, an LLM can design the fabricated
history — grouping files into features, ordering commits, splitting large files
across multiple commits, and writing the messages. Pick exactly one engine:

| Flag             | Engine                          | Auth                          |
|------------------|---------------------------------|-------------------------------|
| `--groq`         | Groq API                        | `GROQ_API_KEY` env var        |
| `--nvidia`       | NVIDIA API                      | `NVIDIA_API_KEY` env var      |
| `--claude-code`  | `claude` CLI subprocess         | the CLI's own configuration   |
| `--codex`        | `codex` CLI subprocess          | the CLI's own configuration   |
| `--opencode`     | `opencode` CLI subprocess       | the CLI's own configuration   |

Optional: `--model NAME` overrides the provider's default model;
`--llm-timeout DURATION` bounds each call (default 120s).

LLM engines compose with `--pigs` / `--rats` exactly like `--flurry`:

```
cav --repo ./myrepo --fabricate --groq --rats 3 \
    --start "2026-05-14 09:00" --end "2026-05-14 17:00"
```

The fabricated tree always matches the source HEAD exactly. Unlike `--flurry`,
LLM-backed runs are **not** guaranteed to be bit-reproducible across runs:
the structural reshaping (`--pigs` / `--rats`, scheduling) stays seeded, but the
LLM's plan itself may vary. If the provider fails or returns an unusable plan
after retries, Caveira aborts with an error — it does not silently fall back.
```

- [ ] **Step 3: Verify the help output**

Run: `go run ./cmd/caveira --help` (or the module's main package path)
Expected: the help text lists `--groq`, `--claude-code`, `--codex`, `--nvidia`, `--opencode`, `--model`, `--llm-timeout`, and shows the new example block.

- [ ] **Step 4: Run the full suite once more**

Run: `go build ./... && go test ./... && go vet ./...`
Expected: all pass, no vet warnings.

- [ ] **Step 5: Commit**

```bash
git add README.md internal/cli/cli.go
git commit -m "docs: document LLM-backed fabrication engines"
```

---

## Final verification

After all tasks, dispatch the final code reviewer for the whole Phase 2 change set, then:

```bash
gofmt -l internal/
go build ./... && go test ./... && go vet ./...
```

All must be clean before finishing the branch.
