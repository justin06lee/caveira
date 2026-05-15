# Caveira — Fabricate (Phase 1) Design Spec

**Date:** 2026-05-14
**Status:** Approved for planning
**Scope:** Phase 1 — `--fabricate` + `--flurry` (NLP-only) + `--pigs` / `--rats` workflow modes. Phase 2 (LLM providers: Groq, claude-code subprocess, Codex, NVIDIA, OpenCode, …) is a separate spec.

## 1. Purpose

Today, Caveira rewrites the timestamps of an existing commit history. Fabrication is a more aggressive mode: instead of preserving the source repo's commits and only rescheduling them, Caveira **synthesizes a new commit history** that ends at the same final tree as the source's HEAD.

Phase 1 ships:

- An NLP-only fabricator (`--flurry`) that walks the source HEAD tree, groups files into features by directory, and produces a believable sequence of feat / test / chore commits whose final state equals the source HEAD.
- Two workflow modes:
  - `--pigs N` — N people committing chaotically to a single linear branch, with sloppy noise commits and random typos in messages.
  - `--rats N` — N people working on emergent branched topologies, with occasional conflict-fix scars.
- Author identity resolution: from `--pig` / `--rat` flags first, then `.git` history, then interactive prompts; interactive picker when there are more candidates than needed.

The Phase 1 output integrates with the existing `walk.DAG` → `BuildDurations` → `Schedule` pipeline, reusing scaling and squashing as today.

## 2. User flow

1. User invokes `caveira` (or `cav`) with `--fabricate` and at minimum `--flurry`, a `--repo`, `--start`, and `--end`. Optionally `--pigs N` / `--rats N` and `--pig` / `--rat` identity flags.
2. Caveira clones / duplicates the source as today.
3. The fabricator walks the source HEAD tree, classifies files into chore / code / test groups, and emits a base sequence of feature commits.
4. The selected workflow (`pigs` chaotic single-branch, or `rats` emergent multi-branch) reshapes that sequence into a final DAG of synthetic commits with new author/committer metadata.
5. The existing scheduler scales and squashes to fit the window.
6. The rewriter writes the new commits, rebuilds refs, swaps folders, optionally pushes. Same final-stage behavior as the existing pipeline.

## 3. CLI surface

New flags layered on the existing CLI:

```
caveira ... [existing flags] ...
        --fabricate                       # required to enable fabrication
        [--flurry]                        # NLP-only mode (Phase 1 default)
        [--pigs N | --rats N]             # workflow + person count (default: 1 person, linear)
        [--pig "Name <email>" ...]        # repeatable, per-pig identity
        [--rat "Name <email>" ...]        # repeatable, per-rat identity (only with --rats)
```

### 3.1 Validation

- `--flurry`, `--pigs`, `--rats`, `--pig`, `--rat` all require `--fabricate`. Without it: refuse with a clear error.
- `--pigs` and `--rats` are mutually exclusive.
- `--pig` flags only valid with `--pigs`; `--rat` flags only valid with `--rats`.
- `--fabricate` without `--flurry` errors with `no LLM provider configured (see Phase 2)`.
- Without `--pigs` / `--rats`: defaults to a single author, linear history, no noise.

### 3.2 Help

`--help` documents these flags in the same table. Example block in the README is extended.

## 4. Author identity resolution

The resolver returns N `Identity{Name, Email}` records, where N = the value of `--pigs` / `--rats` (or 1 if neither).

Resolution order:

1. **Flag identities first.** Collect from `--pig` (with `--pigs`) or `--rat` (with `--rats`). M of them, M ≤ N.
2. **Source `.git` next.** Scan all reachable commits for unique `(Name, Email)` pairs by email (case-insensitive). Exclude identities already supplied via flags.
3. **If discovered + flag-supplied < N**, prompt interactively on stdin for each missing slot: ask Name then Email, re-prompt on empty input.
4. **If discovered (excluding flag-supplied) > the remaining slots needed**, show an interactive picker with one entry per discovered identity, sorted by commit count descending, and read a comma-separated selection.

```
Caveira needs 3 identities. 1 supplied via --pig. Found 5 in .git:
  [1] Alice Cooper <alice@example.com>     (42 commits)
  [2] Bob Marley <bob@example.com>         (31 commits)
  [3] Carol Danvers <carol@example.com>    (18 commits)
  [4] Dave Grohl <dave@example.com>        (11 commits)
  [5] Eve Polastri <eve@example.com>       (4 commits)
Pick 2 (comma-separated, e.g. `1,3`):
```

### 4.1 Single-author default

When neither `--pigs` nor `--rats` is set, N = 1 and the identity comes from `git config user.name` + `git config user.email`. If either is unset, error with `configure git config user.{name,email} or pass --pig`.

### 4.2 Determinism

When the user passes exactly N identities via flags, no `.git` scan, no prompt, no picker. Interactive flow only kicks in when the count requires it. Scripted / CI invocations stay scripted.

## 5. Flurry engine (NLP-only fabrication)

The fabricator walks the source HEAD tree and synthesizes a sequence of commits that, applied in order to an empty tree, reach the source HEAD tree exactly.

### 5.1 File classification

For each path in the source HEAD tree:

- **chore**: top-level files (no directory prefix) matching any of:
  - `README*`, `Makefile`, `*.md`, `LICENSE*`
  - `go.mod`, `go.sum`
  - `package.json`, `package-lock.json`, `yarn.lock`, `pnpm-lock.yaml`
  - `Cargo.toml`, `Cargo.lock`
  - `pyproject.toml`, `setup.py`, `setup.cfg`, `requirements*.txt`, `Pipfile*`
  - `Dockerfile`, `.dockerignore`
  - `.gitignore`, `.gitattributes`, `.editorconfig`
  - Any other top-level dotfile.
- **test**: path or filename matches any of:
  - `*_test.{ext}`, `test_*.{ext}`, `*.test.{ext}`, `*.spec.{ext}`
  - Contains `/test/`, `/tests/`, `/__tests__/`, `/spec/` anywhere in its path.
- **code**: everything else.

### 5.2 Feature grouping

Group `code` and `test` files by **top-level directory**.

- `internal/walk/load.go` → feature `internal/walk`
- `cmd/caveira/main.go` → feature `cmd/caveira`
- A file directly under repo root (and not a chore) → feature `.` (root feature)
- A directory containing only test files is still a feature; it gets only a test commit.

Within a feature, the code set and test set are computed separately.

### 5.3 Commit sequence

In this order:

1. **Chore commit:** `chore: project scaffolding`. Diff = all chore files added.
2. **For each feature**, sorted alphabetically by directory name:
   - **Code commit:** `feat(<name>): add <name>` (or one of a small variation pool: `add` / `introduce` / `scaffold`). Diff = all code files in the feature added.
   - **Test commit** (only if test set non-empty): `test(<name>): tests for <name>`. Diff = all test files in the feature added.

Where `<name>` is the basename of the feature directory (e.g., `walk` for `internal/walk`).

All fabricated commits are purely additive. The final commit's tree equals the source HEAD tree exactly. No commit modifies a file added by an earlier commit (Phase 1 constraint).

### 5.4 Variation pool

A small templated message pool, with the variation seeded by `--seed`:

| Kind  | Variations                                                       |
|-------|------------------------------------------------------------------|
| chore | `chore: project scaffolding`, `chore: initial scaffolding`        |
| code  | `feat(<n>): add <n>`, `feat(<n>): introduce <n>`, `feat(<n>): scaffold <n>` |
| test  | `test(<n>): tests for <n>`, `test(<n>): add tests for <n>`        |

Stays templated. Sophisticated message synthesis is Phase 2 (LLM-backed).

### 5.5 Edge cases

- Repo with only chore files → just the chore commit. The schedule then has a single commit; the scheduler handles that as today.
- Repo with one file at root → that file becomes the root feature; gets a single code commit.
- Empty repo → error.

## 6. Pigs mode (chaotic single-branch)

Reshapes the flurry sequence (§5.3) for N pigs.

### 6.1 Author assignment

For each non-noise commit in the sequence, pick an author **round-robin** through the N pig identities. Round-robin starts at pig 0; this is deterministic given `--seed`.

### 6.2 Noise injection

After the flurry sequence is built, inject noise commits between real commits at roughly a **15% rate** (≈1 noise commit per ~7 real ones). Noise commits:

- **Diff**: empty (no tree change).
- **Message**: drawn uniformly from a fixed pool — `wip`, `fix`, `fix typo`, `revert`, `more changes`, `stuff`, `todo`, `wip2`, `idk`, `actually fix`, `lint`, `fmt`.
- **Position**: spread roughly uniformly across the gaps in the sequence; the seeded RNG decides which gaps get noise.
- **Author**: random pig (not round-robin).
- **End of sequence**: the sequence ends on a real commit, not noise, so HEAD points at a feature commit.

### 6.3 Typos on any message

In pigs mode, every commit's message (real or noise) may be transformed by typos. Per commit, roll:

- 0 typos: ≈70%
- 1 typo: ≈25%
- 2 typos: ≈5%

Each typo applies one of four transformations, picked uniformly:

- **Adjacent character swap** at a random position.
- **Character drop** at a random position.
- **Character double** at a random position.
- **Keyboard-neighbor substitution** (QWERTY layout) at a random position.

Positions are chosen uniformly across the entire message. Conventional-commit prefixes (`feat(walk):`) are not protected — pigs are sloppy on everything.

All probabilities are constants in `internal/fabricate/pigs.go`. Seeded by `--seed`.

### 6.4 Output shape

A linear DAG on `master` (or `main`, whichever the source uses). No branches, no merges.

## 7. Rats mode (multi-branch, emergent topology)

Reshapes the flurry sequence (§5.3) for N rats working on feature branches with emergent topology.

### 7.1 Per-feature events

For each feature in the flurry sequence (in flurry order):

1. **Assigned rat**: round-robin through the N rats. If `N > number of features`, surplus rats are idle in Phase 1 (no commits attributed to them) — see §7.4.
2. **Fork point roll**:
   - With probability ≈70%, the new feature branch forks from `master`'s current tip.
   - With probability ≈30%, if any feature branch is currently **open** (started but not yet merged), the new branch forks from that branch's current tip. This produces the off-branch-off-branch topology — "feature B was built on top of feature A in progress."
3. **Commits**: the code commit and the test commit (if any) land on the new branch, authored by the feature's rat.
4. **Merge timing**: a small random offset past the last commit on this branch. Branches do not merge in feature order — their lifespans overlap, producing concurrent active sets.

### 7.2 Conflict fix scars

After each feature branch merges:

- With probability ≈20%, append a `fix: resolve conflict in <feature>` commit on `master`. Empty diff.
- With probability ≈8% (i.e., 40% of conflict events), instead of a single inline fix commit, spawn a `fix/<feature>` branch with 1–2 small conflict-fix commits and merge it back. Models "had to make a hotfix branch."

### 7.3 No noise commits

Rats are organized. No `wip`/`fix typo` noise. No typo layer. Messages stay clean per the §5.4 templates.

### 7.4 Merge commits

Each merge from a feature branch into `master` produces a merge commit with two parents:

- **Message**: `Merge branch 'feat/<name>' into master`.
- **Author / Committer**: the feature's assigned rat.

When `N > number of features`, the surplus rats remain idle in Phase 1 (no commits attributed to them). A future phase can assign merges or hotfix duty to surplus rats; the round-robin scheme is intentionally simple here.

### 7.5 Branch naming and refs

- Feature branches: `feat/<name>` (e.g., `feat/walk`, `feat/cli`).
- Conflict-fix branches: `fix/<name>`.
- All branches end at their merge point and survive in the rewritten repo as refs that the user can delete.

### 7.6 Probabilities

Constants in `internal/fabricate/rats.go`, seeded by `--seed`:

| Event                                             | Probability                              |
|---------------------------------------------------|------------------------------------------|
| Fork from another open branch instead of master   | 30% (if any open branch exists)          |
| Conflict-fix commit after a merge                 | 20%                                      |
| Spawn fix-branch instead of inline fix commit     | 8% (40% of the conflict events)          |

### 7.7 Resulting DAG

The rats engine emits a `walk.DAG` whose shape varies per seed. Example for 3 features + 2 rats:

```
master:           *
                  |
                  * (chore)
                 /|
                * |    feat/walk (rat 0)
                * |
                |/
                *      Merge feat/walk
                |\___
                *    \   feat/cli (rat 1)
                |     |
                |     *  feat/cli-helpers (rat 0, forked off feat/cli)
                |     *
                |    /|
                |   * |
                |   |/
                |   *      Merge feat/cli-helpers
                |  /
                | /
                |/
                *      Merge feat/cli
                *      fix: resolve conflict in cli
```

## 8. Scheduler integration

Both pigs and rats emit a `walk.DAG`. The existing pipeline then handles everything:

- `BuildDurations` scores synthetic diffs (real diffs for code/test commits, zero-stat for noise / conflict-fix / merge commits → trivial bucket).
- `Schedule` runs as today, including scaling (`s ∈ [0.5, 1]`) and squash / linearization fallbacks if the window is narrow.
- `rewrite.Apply` + `RebuildRefs` write the new commits and rebuild branch / tag refs.
- The folder swap and optional push are unchanged.

No new scheduling code. The only addition to the rewrite path: surface the **fabricated branches** (`feat/<name>`, `fix/<name>`) as refs in the destination repo via `RebuildRefs`.

## 9. Project structure

New package `internal/fabricate`. Internal layout:

```
internal/fabricate/
├── classify.go            # File classification (chore/code/test)
├── classify_test.go
├── group.go               # Feature grouping by top-level directory
├── group_test.go
├── flurry.go              # Base flurry sequence (chore + per-feature commits)
├── flurry_test.go
├── pigs.go                # Pigs mode: author RR, noise injection, typos
├── pigs_test.go
├── rats.go                # Rats mode: emergent topology, fork points, conflict scars
├── rats_test.go
├── identity.go            # Identity resolution (flags, .git, prompts, picker)
├── identity_test.go
├── templates.go           # Message templates and variation pool
└── typos.go               # Typo transformations (used by pigs.go)
```

Wire-in points:

- `internal/cli/cli.go` — new flags + validation.
- `internal/input/config.go` — `Fabricate`, `Flurry`, `PigsN`, `RatsN`, `PigIdentities`, `RatIdentities` fields. Validate function gains the cross-flag rules.
- `internal/cli/pipeline.go` — when `cfg.Fabricate`, replace the `walk.Load(srcRepo)` step with `fabricate.Generate(srcRepo, cfg)` which returns a `(*walk.DAG, error)`. `BuildDurations`, `Schedule`, and `rewrite.Apply` continue downstream unchanged.

## 10. Testing

- Unit tests per `internal/fabricate/*` package file.
- Fixture-based integration tests in `internal/cli/`:
  - Pigs + single rat: assert linear DAG, real-author-only round-robin, and 0 noise commits at N=1.
  - Pigs + 2 pigs: assert 2 distinct authors interleave, at least one noise commit appears for fixtures large enough.
  - Pigs + typo seeding: with a fixed seed, assert at least one typo lands in the output.
  - Rats + 2 rats: assert at least 2 feature branches, at least 1 merge commit.
  - Rats + seeded emergent topology: with a known seed, assert the topology matches a recorded fixture shape (off-branch fork present in one variant).
  - Conflict-fix events present at expected seed.
- Property tests:
  - Final tree equals source HEAD tree for both pigs and rats.
  - Total reachable commit count matches `flurry_count + noise_count + merge_count + conflict_fix_count`.
  - All author/committer signatures come from the resolved identity set.

## 11. Non-goals (Phase 1)

- LLM providers (`--groq`, `--claude-code`, `--codex`, `--nvidia`, `--opencode`). These come in Phase 2 with their own design pass.
- Smarter feature grouping (e.g., import-graph clustering, function-level grouping). Phase 2.
- Real merge-conflict diffs (where two commits actually modify the same line and a fix commit resolves the diff). Phase 1 simulates conflicts purely with cosmetic fix commits.
- Layered scaffold → impl → tests within a single file (requires content modification within a feature). Phase 2.
- Configurable noise / conflict / fork probabilities at the CLI. These are constants in code for Phase 1.
- Non-Git VCS support.
