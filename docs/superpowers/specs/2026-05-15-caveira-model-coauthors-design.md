# Caveira — Model Co-Authorship Design Spec

**Date:** 2026-05-15
**Status:** Approved for planning
**Scope:** Make Caveira's fabricator aware of AI coding-model identities found in the
source repo's history — exclude them from the human "player" pool, and instead
surface them as `Co-Authored-By:` trailers on fabricated commits, weighted by each
player's real-world model usage.

## 1. Purpose

When Caveira fabricates a history (`--fabricate`, optionally with `--pigs N` /
`--rats N`), it resolves the "players" (pigs/rats) by scanning the source repo's
commit history for unique identities. Today every identity is treated as a person.

In practice a repo's history often contains AI coding agents — Claude Code, Codex,
GitHub Copilot, and similar — appearing as authors, committers, or
`Co-Authored-By:` trailers. Counting an agent as a "person" is wrong: it consumes
a `--pigs`/`--rats` slot and shows up as a standalone fabricated author.

This feature makes Caveira **model-aware**:

- AI coding models are detected and **excluded from the player pool** — never a
  pig/rat, never a slot, never in the interactive picker.
- Detected models still appear in the fabricated history, but **only as
  `Co-Authored-By:` trailers** alongside a real human author — never as the sole
  author of a commit.
- A model co-authors a commit more often on documentation/chore commits, plus a
  baseline chance elsewhere — at a rate driven by how much that specific player
  actually used models in the real history.
- **Only models that actually appear in the source history are used.** Caveira
  never invents a model. If the source has no models, output is unchanged.
- When multiple models are detected, the model chosen to co-author a given
  player's commit is weighted by that player's real-world usage mix of those
  models.

## 2. Architecture

Co-authorship is a **separate post-processing pass**, not woven into the
fabricators. The fabricator (`flurry` base → `reshapePigs` / `reshapeRats` /
single) builds the `Plan` exactly as it does today. Then a new pass walks the
finished plan and appends `Co-Authored-By:` trailers.

This keeps `reshapePigs` / `reshapeRats` untouched, isolates all model logic in
focused units, and works uniformly across pigs, rats, and single mode because all
three produce a `Plan`. The rejected alternative — embedding co-author logic in
each fabricator — would triplicate the logic.

Two inputs feed the pass:

1. A **classifier** that decides whether an identity is an AI coding model.
2. A **`ModelReport`** computed by scanning the source history: the set of
   detected models, and a per-player profile of model usage.

The pass, `InjectCoAuthors`, consumes the `ModelReport` and mutates the plan's
commit messages.

## 3. Model detection

`internal/fabricate/model.go` provides `IsModel(Identity) bool`:

- **Recognized list** — a built-in set of known coding-agent identities matched on
  name and email. Includes at least: Claude Code (`noreply@anthropic.com`, names
  containing "Claude"), Codex / OpenAI agents, GitHub Copilot
  (`copilot@github.com`, `[bot]`-suffixed names), Cursor, Devin, Aider, opencode.
- **Heuristic fallback** — an identity not on the list is still classified as a
  model if its name or email (case-insensitive) contains an agent token —
  `claude`, `codex`, `copilot`, `cursor`, `aider`, `gpt`, `devin` — or ends with a
  `[bot]` suffix, or uses a known AI-vendor email domain.
- Classification is applied to every identity that appears in source history in
  **any** position: author, committer, or `Co-Authored-By:` trailer.

The recognized list and heuristic token set are constants in `model.go`.

## 4. Source-history measurement — `ModelReport`

`internal/fabricate/modelreport.go` provides the `ModelReport` type and
`ScanModelReport(repo)` which walks every reachable commit in the source repo.

`ModelReport` contains:

- **`Models`** — the set of distinct model identities (per §3) that appear
  anywhere in history. If empty, co-authorship is skipped entirely (§6) and
  Caveira behaves exactly as before this feature.
- **Per-player profile**, keyed by player (human author) email, lowercased:
  - For each **human-authored** commit, the author is the player; any model
    identity appearing in that commit's **committer slot or `Co-Authored-By:`
    trailers** counts as a model used on that commit.
  - `Rate` = (player's commits that had ≥1 model) / (player's total commits).
    Drives *how often* a model co-authors that player's fabricated commits.
  - `Mix` = per-model fractions: of the player's model-assisted commits, the
    fraction in which each model M appeared. Drives *which* model is chosen.

Rules and edge cases:

- Commits authored directly by a model (no human author) are **ignored** for
  player measurement — there is no player to attribute them to. (Such a model is
  still added to `Models` so it remains co-author-eligible.)
- `Co-Authored-By:` trailers are parsed from the commit message body as standard
  git trailer lines (`Co-Authored-By: Name <email>`).
- A player who never used a model has `Rate = 0` and an empty `Mix` — they will
  never receive a model co-author.
- The scan considers all reachable commits from all refs, like
  `DiscoverIdentities` does today.

`ScanModelReport` is a distinct pass from `DiscoverIdentities`; the two walk
history independently for clarity. (A future optimization could merge the walks;
not required here.)

## 5. Player resolution

`DiscoverIdentities` (`internal/fabricate/identity.go`) changes so detected
models are **filtered out of the auto-discovered pool**:

- Auto-discovered candidate identities exclude anything `IsModel` flags. A model
  is never offered as a pig/rat and never appears in the interactive picker.
- The `--pigs N` / `--rats N` count therefore resolves against humans only — a
  repo with "3 humans + Claude Code" yields 3 human candidates.
- **Flag-supplied identities** (`--pig "Name <email>"`, `--rat ...`) are taken
  as-is and are *not* model-filtered — if the user explicitly names an identity,
  Caveira trusts it. Model filtering applies only to the automatic `.git` scan.

## 6. Co-author injection — `InjectCoAuthors`

`internal/fabricate/coauthor.go` provides
`InjectCoAuthors(plan *Plan, report *ModelReport, rng *rand.Rand)`, run as a
post-pass on the final plan. If `report.Models` is empty it is a no-op.

For each commit in `plan.Commits`:

1. **Identify the player** — the commit's author. Look up the player's profile in
   the `ModelReport` by lowercased email. If there is no profile, or `Rate == 0`,
   skip the commit.
2. **Classify the commit** by its changed files (`Added`), using the existing
   `Classify`:
   - **chore/doc commit** — every file in `Added` classifies as Chore (README,
     `*.md`, config) → elevated probability. (flurry's chore commit contains only
     chore files; feature commits contain only code or only test, so this test is
     unambiguous for fabricated commits.)
   - **code/test commit** — otherwise → base probability.
   - **merge commit** (`IsMerge`) or **empty commit** (no `Added`, e.g. a pigs
     noise commit) → skipped; a merge or "wip" noise commit is not authored
     content.
3. **Draw** — model-co-author probability `p = min(1.0, Rate × typeFactor)`,
   where `typeFactor = 1.5` for chore/doc commits and `1.0` for code/test commits.
   `typeFactor` values are constants in `coauthor.go`. Roll the seeded RNG
   against `p`.
4. **If it fires** — pick exactly one model, weighted by the player's `Mix` (a
   player who used Claude 80% / Codex 20% picks Claude ~80% of the time). Append a
   single `Co-Authored-By: <Model Name> <model email>` trailer to the commit
   message — a proper git trailer: a blank line after the message body, then the
   trailer line.
5. **At most one** model co-author per commit.

Determinism: every draw uses the seeded RNG, so `--seed` runs stay reproducible.
Ordering: `InjectCoAuthors` runs *after* the fabricator (so pigs-mode message
typos, applied during reshaping, never corrupt the appended trailer) and *after*
scheduling/squashing (so trailers land on exactly the commits that will be
written). It runs immediately before `WriteToRepo`.

## 7. Pipeline integration

In `internal/cli/pipeline.go`, `fabricatePipeline`:

- Computes the `ModelReport` once via `ScanModelReport(srcRepo)`, right after the
  source repo is opened. This runs for **all** fabricate modes — pigs, rats, and
  single.
- Calls `InjectCoAuthors(plan, report, rng)` on the final plan just before
  `WriteToRepo` — after `Generate`, after scheduling, and after any pigs squash
  handling.

Single mode: the player is the `git config user.{name,email}` identity. It gets a
model profile only if that email also appears as an author in the source history
(true when fabricating your own repo, not when you cloned someone else's). No
match → `Rate = 0` → no model co-authors, the correct graceful fallback.

**No new CLI flags.** Co-authorship is automatic, exactly like the existing
noise-commit and typo behavior: models in the source history surface as
co-authors; no models → no change. This satisfies "only ones in the commit
history should show up."

## 8. Project structure

New files in `internal/fabricate/`:

```
internal/fabricate/
├── model.go              # IsModel classifier: recognized list + heuristic
├── model_test.go
├── modelreport.go        # ModelReport type, ScanModelReport, Co-Authored-By parsing
├── modelreport_test.go
├── coauthor.go           # InjectCoAuthors post-pass + trailer formatting
└── coauthor_test.go
```

Modified files:

- `internal/fabricate/identity.go` — `DiscoverIdentities` filters out models.
- `internal/cli/pipeline.go` — `fabricatePipeline` computes the `ModelReport` and
  runs `InjectCoAuthors` before `WriteToRepo`.

## 9. Testing

- **Classifier** (`model_test.go`): known agents (Claude Code, Codex, Copilot,
  …) classified as models; ordinary humans are not; the heuristic catches
  name/email variants not on the recognized list.
- **Scan** (`modelreport_test.go`): an in-memory repo whose commits carry model
  `Co-Authored-By:` trailers and model committers → assert the detected `Models`
  set and each player's `Rate` and `Mix`. Cover a player with zero model usage
  (`Rate == 0`).
- **Injection** (`coauthor_test.go`):
  - Trailers are appended with the correct `Co-Authored-By: Name <email>` format.
  - The chosen model follows the player's `Mix` weighting.
  - chore/doc commits receive the elevated rate; merge commits and empty noise
    commits are skipped.
  - Seeded `--seed` reproducibility — identical output across runs.
  - **No models in the source → the plan is byte-identical to today** (no
    regression for repos without AI agents).
- **Integration** (`internal/cli/`): a fabricate run on a fixture repo whose
  history contains a model co-author → fabricated output commits carry
  `Co-Authored-By:` trailers, and the model never appears in the player pool.

## 10. Non-goals

- No CLI flag to enable/disable or tune co-authorship — it is automatic, and the
  rates/tokens are code constants (consistent with noise/typo behavior).
- Caveira never invents a model not present in the source history.
- Models are never standalone authors of fabricated commits — only co-authors.
- No more than one model co-author per commit.
- Merge and empty noise commits never receive model co-authors.
