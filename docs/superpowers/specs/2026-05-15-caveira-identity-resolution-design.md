# Caveira — Identity Resolution: `.mailmap` + `--pick` Design Spec

**Date:** 2026-05-15
**Status:** Approved for planning
**Scope:** Two improvements to how Caveira's fabricator resolves "player" identities
from a repo's history: (1) honor a repository's `.mailmap` file so identities
that drifted across `user.name`/`user.email` changes are unified, and (2) add a
`--pick` flag that always opens an interactive curation step so the user can
hand-select players every run.

## 1. Purpose

When Caveira fabricates a history (`--fabricate`, with `--pigs N` / `--rats N`),
it resolves the players by scanning the source repo's commit history for unique
identities (`DiscoverIdentities`). Two pain points motivate this work:

1. **Identity drift.** A single human often appears under several
   `name <email>` combinations because their git config changed over time.
   Caveira keys identities by email, so two emails for one person count as two
   players — and that person's model-usage profile is split across the two.
2. **No explicit control.** The interactive picker only appears when the repo
   has *more* discovered identities than requested. The user cannot, on demand,
   hand-pick exactly which people become players.

This spec adds `.mailmap` support (the git-standard fix for drift, no history
rewrite) and a `--pick` flag (explicit, always-on curation).

## 2. Architecture

Both changes are localized to identity resolution — a new
`internal/fabricate/mailmap.go`, changes to `internal/fabricate/identity.go` and
`modelreport.go`, a CLI flag, and pipeline wiring. There are no architectural
forks. The retime (non-fabricate) path is untouched.

A `Mailmap` value is loaded once per run from the repo's `.mailmap` file and
threaded into every function that reads identities from history, so
canonicalization is applied consistently.

## 3. `.mailmap` support

### 3.1 Source

The pipeline reads `<repo>/.mailmap` — the working-tree file at the repo root,
the same file `git` checks first. If the file is absent, the `Mailmap` is empty
and every function behaves exactly as before this feature.

### 3.2 Parsing

`internal/fabricate/mailmap.go` provides:

- `ParseMailmap(content []byte) *Mailmap` — parses the four standard `.mailmap`
  line forms:
  1. `Proper Name <proper@email>` — canonical name for that (proper) email.
  2. `<proper@email> <commit@email>` — map a commit email to a proper email.
  3. `Proper Name <proper@email> <commit@email>` — map a commit email to a
     proper name **and** email.
  4. `Proper Name <proper@email> Commit Name <commit@email>` — map a specific
     `(commit name, commit email)` pair to a proper name and email.

  Lines beginning with `#` (after optional whitespace) and blank lines are
  ignored. Unparseable lines are skipped.

- `(*Mailmap).Canonical(id Identity) Identity` — returns the canonical identity
  for `id`. It is **nil-safe**: a nil `*Mailmap` returns `id` unchanged.
  Resolution matches git's precedence: a form-4 `(name,email)` rule is the most
  specific; otherwise an email-keyed rule applies; a form-1 rule supplies the
  canonical name for an already-canonical email. Email matching is
  case-insensitive.

### 3.3 Where it is applied

Every identity Caveira derives from history is passed through
`Mailmap.Canonical` **before** it is keyed, counted, or compared:

- **`DiscoverIdentities`** (`identity.go`) — the `--pigs`/`--rats` candidate
  scan. Canonicalizing author identities means a person with two emails unified
  by `.mailmap` collapses to **one** discovered player.
- **`ScanModelReport`** (`modelreport.go`) — the model-usage measurement.
  Canonicalizing author, committer, and `Co-Authored-By:` identities means a
  drifted player gets **one** merged profile (`Rate`/`Mix`) rather than two
  partial ones, and a model with drift is one entry in `Models`.

### 3.4 Threading

The pipeline loads the `Mailmap` once and passes it (as `*Mailmap`, nil-safe)
into `DiscoverIdentities`, `ResolveIdentities`, and `ScanModelReport`. These are
internal-package signature changes; `internal/cli/pipeline.go` is the only
caller.

## 4. `--pick` flag

### 4.1 Flag and validation

A new boolean CLI flag, `--pick`. Validation (`internal/input/config.go`,
`Config.Pick bool`):

- `--pick` requires `--fabricate`.
- `--pick` requires `--pigs N` or `--rats N` — it curates the discovered player
  pool, and single mode (neither `--pigs` nor `--rats`) has no pool. `--pick`
  without `--pigs`/`--rats` is rejected with a clear error.

### 4.2 Behavior

When `--pick` is set, identity resolution always runs an interactive
**curation** step, replacing today's auto-use / picker-only-when-too-many /
prompt-when-too-few logic:

1. Flag-supplied `--pig`/`--rat` identities are taken first, exactly as today —
   they pre-fill slots. Curation fills the `remaining = N - len(flagIDs)` slots.
2. Caveira prints every discovered identity (excluding any already supplied via
   flags) — numbered, with commit counts, model-filtered, and
   `.mailmap`-canonicalized. The user selects **any subset**, from 0 up to
   `remaining`, as comma-separated indices; empty input selects none.
3. If the selection fills fewer than `remaining` slots, Caveira then prompts the
   user to type the remaining people as `Name <email>`, reusing the existing
   prompt flow (`promptIdentities`).
4. Selecting more than `remaining` entries is an error.

This is "curate freely": on a solo repo the user can pick 0 discovered
identities and type all `N` fresh, or pick 1 and type the rest — full control
every run.

When `--pick` is **not** set, identity resolution is unchanged from today.

### 4.3 `curateIdentities`

A new function in `identity.go`, `curateIdentities`, implements the
variable-count picker. It differs from the existing `pickIdentities` (which
requires *exactly* `k` picks): `curateIdentities` accepts a selection of 0 up to
`max` entries, validates each index is in range, rejects duplicates and
over-selection, and returns the chosen identities.

`ResolveIdentities` gains a `pick bool` parameter. When `pick` is true it runs
the curate path (flags → `curateIdentities` → `promptIdentities` for any
shortfall); when false it runs the existing path.

## 5. Project structure

**New file:**

```
internal/fabricate/
├── mailmap.go        # Mailmap type, ParseMailmap, Canonical
└── mailmap_test.go
```

**Modified files:**

- `internal/fabricate/identity.go` — `DiscoverIdentities` and `ResolveIdentities`
  take a `*Mailmap`; identities are canonicalized; `ResolveIdentities` takes a
  `pick bool`; new `curateIdentities`.
- `internal/fabricate/modelreport.go` — `ScanModelReport` takes a `*Mailmap` and
  canonicalizes author/committer/co-author identities.
- `internal/input/config.go` — `Pick bool` field; validation for `--pick`.
- `internal/cli/cli.go` — register the `--pick` flag.
- `internal/cli/pipeline.go` — `fabricatePipeline` reads `<srcPath>/.mailmap`,
  parses it once, and threads the `*Mailmap` and `cfg.Pick` into the identity
  calls.

## 6. Testing

- **`mailmap_test.go`** — `ParseMailmap` handles all four line forms and `#`
  comments; `Canonical` maps name and email correctly for each form; a nil
  `*Mailmap` is a passthrough; email matching is case-insensitive.
- **`identity_test.go`** — `DiscoverIdentities` with a `Mailmap` collapses a
  two-email person into one identity; `curateIdentities` selects a subset,
  accepts an empty selection, and errors on over-selection and out-of-range
  indices; `ResolveIdentities` with `pick=true` runs the curation flow (flags →
  curate → prompt shortfall).
- **`modelreport_test.go`** — `ScanModelReport` with a `Mailmap` merges a
  drifted player's commits into one `PlayerProfile`.
- **`config_test.go`** — `--pick` validates only with `--fabricate` +
  `--pigs`/`--rats`; rejected otherwise.
- **`cli_test.go`** — the `--pick` flag parses and reaches `Config.Pick`.
- **Integration** (`internal/cli/`) — a fabricate run on a fixture repo whose
  history has a drifted identity and a `.mailmap` unifying it → the fabricated
  output reflects one unified person.

## 7. Non-goals

- Caveira does not read `.mailmap` from a committed blob or honor the
  `mailmap.file` / `mailmap.blob` git config — only the working-tree
  `<repo>/.mailmap` file.
- Caveira does not rewrite repo history to unify identities; `.mailmap` is a
  non-destructive mapping.
- `--pick` is not offered for single mode (no discovered pool to curate).
- No automatic guessing that two emails belong to one person — unification is
  explicit, declared by the user's `.mailmap`.
