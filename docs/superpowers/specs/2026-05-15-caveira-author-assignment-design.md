# Caveira — Random Author Assignment + `--earned` Design Spec

**Date:** 2026-05-15
**Status:** Approved for planning
**Scope:** Replace the round-robin commit-to-player assignment in the fabricators
with a seeded random draw (so fabricated histories aren't mechanically cyclic),
and add an `--earned` flag that weights that draw by each player's real
commit-count distribution in the source history.

## 1. Purpose

Caveira's fabricators assign commits to players (`--pigs`/`--rats`) by
**round-robin**: pigs cycle authors per commit (`0,1,2,0,1,2,…`), rats cycle one
rat per feature. The cyclic pattern is unrealistic — a real history never
alternates authors so mechanically.

This change makes assignment a **random draw**:

- **Default:** each player has equal probability. Over many commits the
  distribution is roughly even (law of large numbers) but realistically jittery,
  not cyclic.
- **`--earned`:** the draw is **weighted** by how many commits each player
  actually authored in the source repo's existing history — a heavy real
  contributor gets proportionally more fabricated commits.

## 2. Architecture

The reshapers (`reshapePigs`, `reshapeRats`) already hold the seeded `*rand.Rand`.
The default change is local to them: swap the index arithmetic for a random
draw. `--earned` threads one optional `weights []int` parameter (parallel to the
`ids` slice; `nil` means equal-probability uniform) through the fabricator entry
points down to the reshapers. A small weighted-pick helper performs the draw.

No new packages. Changes touch `internal/fabricate` (the reshapers, the
fabricator entry points, a pick helper), `internal/input/config.go`,
`internal/cli/cli.go`, and `internal/cli/pipeline.go`.

## 3. Random author assignment (default)

The round-robin index math is replaced by a seeded random draw:

- **`reshapePigs`** — a real commit's author is currently `ids[i % len(ids)]`. It
  becomes a uniform random draw. Author and committer of a real commit remain the
  same person. Noise commits already draw their author/committer randomly and
  independently — unchanged.
- **`reshapeRats`** — a feature's owning rat is currently `ids[fi % len(ids)]`
  (cyclic over features). It becomes a uniform random draw per feature. Merge and
  conflict-fix commits remain attributed to that feature's rat. The single
  chore/initial commit also gets a random draw instead of always `ids[0]`, for
  consistency.
- **`reshapeSingle`** — one player; a draw over one identity is that identity.
  Effectively unchanged.

All draws use the existing seeded `rng`, so a `--seed` run stays fully
reproducible; an unseeded run varies — consistent with existing noise/typo
behavior.

## 4. `--earned` flag — weighted assignment

### 4.1 Flag and validation

A new boolean CLI flag, `--earned`. Validation (`internal/input/config.go`,
`Config.Earned bool`):

- `--earned` requires `--fabricate`.
- `--earned` requires `--pigs N` or `--rats N` — single mode has one player, so
  weighting is meaningless there. `--earned` without `--pigs`/`--rats` is
  rejected with a clear error.

### 4.2 Behavior

When `--earned` is set, the per-commit (pigs) / per-feature (rats) author draw is
**weighted** by each player's real commit count rather than uniform:

- Real counts come from `DiscoverIdentities` (its `DiscoveredIdentity.Commits`
  field), with identities canonicalized through the repo's `.mailmap` exactly as
  the rest of identity resolution already does.
- For each resolved player, the weight is that player's real commit count.
- **A player with no real history** — supplied via a `--pig`/`--rat` flag, or
  typed at an interactive prompt, so absent from the discovered set — gets a
  weight equal to `round(mean of the discovered players' counts)`, with a minimum
  of 1. They are treated as an "average contributor."
- **If the source repo has no discovered identities at all** (every player was
  flag-supplied or typed), there is nothing to weight by: `--earned` falls back
  to the uniform draw and prints a one-line note to stderr.

Without `--earned`, the draw is uniform (§3).

## 5. Threading and structure

- **`weights []int`** — an optional slice, parallel to `ids` (same length, same
  order). `nil` means uniform. It is threaded through the fabricator entry
  points: `Generate`, `BuildPigsPlan`, `BuildRatsPlan`, into `reshapePigs` /
  `reshapeRats` / `reshapeSingle`.
- **`pickAuthor`** — a small helper in `internal/fabricate` that, given `ids`,
  `weights` (nil-able), and `rng`, returns a chosen player index: a uniform
  `rng.Intn(len(ids))` when `weights` is nil or all-zero, else a weighted draw
  over the cumulative weights.
- **`internal/input/config.go`** — `Earned bool` field; validation per §4.1.
- **`internal/cli/cli.go`** — register the `--earned` flag.
- **`internal/cli/pipeline.go`** — `fabricatePipeline`: when `cfg.Earned`, build
  the `weights` slice (call `DiscoverIdentities` for real counts, match each
  resolved player by canonical email, apply the mean fallback for non-discovered
  players, uniform fallback when no discovered identities exist) and pass it into
  `Generate`; otherwise pass `nil`.

## 6. Testing

- **`pickAuthor`** — uniform draw with `nil` weights covers all indices over many
  draws; weighted draw with non-uniform weights produces a distribution that
  clearly tracks the weights (heavy weight dominates); all-zero weights fall back
  to uniform; a single-element `ids` always returns index 0.
- **`reshapePigs` / `reshapeRats`** — with `nil` weights, over a large base
  sequence every player appears and the spread is roughly even; with skewed
  weights, the heavily-weighted player gets clearly more commits. Seeded runs are
  reproducible (same `rng` seed → identical plan).
- **Rewrite `TestPigsMode_TwoAuthors_RoundRobin`** — round-robin no longer
  exists; the test is rewritten to assert the new contract: with two players over
  a large base sequence, both authors are used and the split is roughly balanced
  (not an exact `0,1,0,1` cycle).
- **`--earned` validation** — accepted with `--fabricate` + `--pigs`/`--rats`;
  rejected without `--fabricate`, and without `--pigs`/`--rats`.
- **`--earned` flag parsing** — the flag reaches `Config.Earned`.
- **Weight computation** — the pipeline's weight builder maps discovered players
  to their real counts, gives non-discovered players the mean, and falls back to
  uniform (`nil`) when no identities are discovered.
- **Integration** — a fabricate `--earned` run on a fixture repo whose history
  has a lopsided real distribution (one author with many commits, another with
  few) → the fabricated history's author distribution clearly favors the heavy
  contributor.

## 7. Non-goals

- The `--unfair` idea (force exactly-equal counts) is dropped; equal-ish
  distribution is what the default uniform random draw already yields by law of
  large numbers.
- No CLI knob to tune the weighting curve — `--earned` is on/off; the weight is
  the raw real commit count.
- The retime (non-fabricate) mode is untouched.
