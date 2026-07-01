<p align="center">
  <img src="docs/caveira.png" alt="Caveira" width="180" />
</p>

<h1 align="center">Caveira</h1>

<p align="center">
  <em>Retime a git repository's history to fit a chosen time window —<br/>
  or fabricate a fresh one. Inspect who's in it first.</em>
</p>

---

Caveira has three modes:

- **Retime** (default) — takes a repo plus a `--start` and `--end`, scores each
  commit's "difficulty" from its diff stats, draws a duration per commit, and
  produces a copy whose history fits inside the requested window — scaling and
  (if necessary) squashing commits to fit. Author and committer identities are
  preserved exactly; only timestamps change. Pass `--preserve` to keep every
  commit instead of squashing — new timestamps follow each commit's original
  chronological order and proportional spacing, compressed to fit the window.
  Pass `--leeches N` to also **scatter authorship**: the real commits, trees and
  messages are kept, but each commit's author is reassigned at random among N
  listed people plus the repo's original authors, so the history reads as
  collaborative.
- **Fabricate** (`--fabricate`) — synthesizes a brand-new commit history that
  ends at the source repo's exact HEAD tree.
- **Interrogate** (`cav interrogate`) — a read-only scan that reports the
  identities in a repo's history without modifying anything.

In retime and fabricate modes the original folder is renamed to
`<name>.dead[.N]` and the rewritten copy takes the original name.

## Install

```bash
make build          # produces bin/caveira and bin/cav
make install        # installs both into $GOBIN
```

Both binaries are the same program; `cav` is just a shorter alias.

## Quick start

```bash
caveira --repo /path/to/myrepo \
        --start "2026-05-14 13:00" \
        --end   "2026-05-14 17:00"
```

URL input clones the repo first, then rewrites the clone:

```bash
cav --repo https://github.com/u/myrepo.git \
    --start "tomorrow 9am" --end "tomorrow 5pm" \
    --seed 42 --dry-run
```

`--dry-run` prints the schedule and exits without touching anything.

Keep every commit (never squash), compressing the spacing to fit a narrow
window:

```bash
caveira --repo /path/to/myrepo \
        --start "2026-05-14 13:00" --end "2026-05-14 13:30" \
        --preserve
```

Inspect a repo's identities without changing anything:

```bash
cav interrogate --repo /path/to/myrepo
```

## Flags

| Flag               | Required | Description                                              |
|--------------------|----------|----------------------------------------------------------|
| `--repo`           | yes      | Filesystem path or `https://` URL                         |
| `--start`          | yes      | Window start (`2026-05-14 13:00`, `tomorrow 9am`, `now`)  |
| `--end`            | yes      | Window end                                                |
| `--seed`           |          | Integer seed for reproducible duration and fabrication draws |
| `--dry-run`        |          | Print the schedule, write nothing                         |
| `--push`           |          | Force-push `origin` after the swap                        |
| `--push-protected` |          | Allow `--push` to touch `main` / `master`                 |
| `--window-tz`      |          | IANA timezone for parsing `--start`/`--end` (default `Local`) |
| `--out-dir`        |          | Parent dir for URL clones (default `$CWD`)                |
| `--preserve`       |          | Never merge commits; keep all in original chronological order, compressing spacing to fit |
| `--fabricate`      |          | Synthesize a new commit history instead of retiming the source |
| `--pigs N`         |          | Chaotic single-branch fabrication with N people           |
| `--rats N`         |          | Branched fabrication with N people                        |
| `--pig "Name <email>"` |      | A pig identity; repeatable (requires `--pigs`)            |
| `--rat "Name <email>"` |      | A rat identity; repeatable (requires `--rats`)            |
| `--pick`           |          | Always open the interactive identity picker (requires `--pigs`/`--rats`/`--leeches`) |
| `--earned`         |          | Weight fabricated authorship by each player's real commit count (requires `--pigs`/`--rats`) |
| `--leeches N`      |          | Retime mode: scatter authorship across N people + the original authors |
| `--leech "Name <email>"` |    | A leech identity; repeatable (requires `--leeches`)       |

Run `caveira --help` for the live flag reference, or `cav interrogate --help`
for the interrogate subcommand.

## How it works

1. Walk every ref in the source repo, build the commit DAG, compute per-commit
   diff stats.
2. Score each commit (`lines + 10·files + 25·new_files`), bucket it into
   `trivial / easy / medium / hard / substantial`, draw a duration.
3. Schedule commits in topological order; sibling branches overlap in
   wall-clock time so parallel work looks parallel.
4. If the span exceeds the window, scale every duration uniformly (floor:
   `s = 0.5`, where the minimum trivial duration reaches 1 minute).
5. If scaling alone can't fit, squash the cheapest adjacent linear edges,
   then linearize branch points if needed, until the window fits.
   With `--preserve`, steps 1–5 change entirely: durations and topological
   scheduling are not used. Instead each commit's new timestamp follows its
   **original author-date order and proportional spacing**, linearly compressed
   into the window (scale `min(1, window/original_span)` — a window wider than
   the original span keeps the real gaps rather than stretching them). This
   keeps the history looking exactly like the source — including rebased
   histories whose commits were authored out of topological order — just shifted
   into the new window. It fails only if the window is shorter than one second
   per commit.
6. Duplicate the source folder, write the new commits, rebuild refs, swap
   the rewritten copy into the original location and rename the original to
   `<name>.dead`.

Full design: [`docs/superpowers/specs/2026-05-14-caveira-design.md`](docs/superpowers/specs/2026-05-14-caveira-design.md).

## Leeches (scatter authorship)

`--leeches N` is a retime-mode option that latches new authors onto the existing
history. The real commits, trees, messages, branches and tags are all kept and
retimed exactly as normal retiming — but each commit's author **and** committer
are reassigned at random, drawn uniformly from a pool of the N resolved leeches
**plus** the repo's original authors. The result reads like everyone worked on
it together: names are scattered, not rotated, and the originals stay in the mix.

```bash
caveira --repo /path/to/myrepo \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
        --leeches 3 \
        --leech "Alice <a@x.com>" --leech "Bob <b@x.com>" --leech "Carol <c@x.com>"
```

- Identity resolution mirrors `--pigs`/`--rats`: `--leech` flags first, then the
  `.git` history, then interactive prompts for any shortfall; `--pick` forces the
  picker. A repo's `.mailmap` is honored, and AI coding agents are never drawn as
  authors.
- The draw is deterministic under `--seed`, and the run prints a per-identity
  commit count. `--dry-run` shows the scatter without writing.
- Composes with `--preserve` (keep every commit, compress spacing, and scatter
  authors) and with `--push`.
- `--leeches` retimes existing commits, so it cannot be combined with
  `--fabricate`; use `--pigs`/`--rats` to scatter authorship in fabricated
  history instead.

## Fabrication mode

`--fabricate` synthesizes a new commit history from scratch using the source
repo's HEAD tree as the target. The built-in templated engine groups files by
top-level directory, classifies each file as chore / code / test, and emits a
sequence of `chore: …`, `feat(<dir>): …`, `test(<dir>): …` commits that end at
the source's exact HEAD tree. Runs are deterministic under `--seed`.

```bash
# Single-author (uses git config user.*)
caveira --repo /path/to/myrepo --fabricate \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00"

# Three pigs: chaotic single-branch with noise commits and message typos
caveira --repo /path/to/myrepo --fabricate \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
        --pigs 3 \
        --pig "Alice <a@x.com>" --pig "Bob <b@x.com>" --pig "Carol <c@x.com>"

# Two rats: emergent feature branches, off-branch forks, occasional conflict-fix scars
caveira --repo /path/to/myrepo --fabricate \
        --start "2026-05-14 09:00" --end "2026-05-14 17:00" \
        --rats 2 \
        --rat "Alice <a@x.com>" --rat "Bob <b@x.com>"
```

`--pigs N` and `--rats N` reshape the base sequence without changing the final
tree. Each commit's author is drawn at random from the players — pigs draw per
commit, rats per feature — so authorship looks jittery rather than mechanically
cyclic. Over many commits the split is roughly even.

### Release tags

Fabricated histories also get **annotated semver release tags** (`v0.1.0`,
`v0.1.1`, `v1.0.0`, …) placed along the main branch — roughly one every few
commits, with natural release messages ("Release v0.2.0"). Names and placement
are templated (no LLM) and deterministic under `--seed`; the tagger is the
author of the tagged commit, and each tag's date matches that commit's scheduled
time. Short histories (fewer than one release interval of commits) get no tags.
In `--pigs` mode, if a tagged commit is squashed to fit a narrow window the tag
follows onto the surviving commit.

Retime mode (including `--preserve`) keeps the source repo's real tags: both
lightweight and annotated tags are retargeted onto their rewritten commits, and
an annotated tag's tagger date is retimed to its commit's new timestamp so the
tag moves into the window instead of keeping its original (now out-of-window)
date. Tagger name and email are preserved.

### Identities

When `--pigs N` / `--rats N` is set, Caveira resolves N players:

1. Identities supplied via `--pig` / `--rat` are used first.
2. For any shortfall, Caveira scans the `.git` history for author identities
   and prompts interactively for any still missing. If more are found than
   needed, it shows a picker.
3. `--pick` always opens the interactive picker, so you can hand-select players
   every run.

A repository's `.mailmap` is honored: identities that drifted across multiple
`name`/`email` pairs are unified into one player. Use
`cav interrogate --emit-mailmap` to bootstrap one.

`--earned` weights the random author draw by each player's real commit count in
the source history — a heavy real contributor gets proportionally more
fabricated commits. Without `--earned`, every player is equally likely.

AI coding agents (Claude, Copilot, Codex, …) found in the source history are
**never** counted as players. Instead, fabricated commits credit them with
`Co-Authored-By:` trailers, weighted to mirror each player's real model-usage
mix.

## Interrogate

`cav interrogate` is a read-only subcommand — it scans a repo and reports who is
in its history without modifying anything.

```bash
cav interrogate --repo /path/to/myrepo
```

```
Identities in /path/to/myrepo  (.mailmap applied)

Players (2):
  justin06lee <hi@justin06lee.dev>            131 commits
  justin06lee <justin.leehuiyun@gmail.com>      1 commit

AI models (excluded from players — co-author only):
  Claude <noreply@anthropic.com>
```

Human identities are listed with commit counts (keyed by email, with `.mailmap`
applied). AI coding agents found as `Co-Authored-By:` trailers are listed
separately — they are never counted as players.

`--emit-mailmap` prints a `.mailmap` skeleton instead of the report — one line
per identity, ready to edit:

```bash
cav interrogate --repo /path/to/myrepo --emit-mailmap > .mailmap
```

Caveira never guesses that two emails belong to the same person; you merge the
lines yourself onto a canonical one. interrogate flags: `--repo` (required),
`--out-dir` (parent dir for URL clones), `--emit-mailmap`.

## Notes & limitations

- In retime mode, author and committer name/email are preserved. Both
  timestamps are rewritten to the new schedule, in each commit's original
  timezone.
- GPG signatures are stripped — rewriting invalidates them. A warning is
  printed when source commits are signed.
- `--push` uses go-git's `Force: true`. go-git does not yet support
  `--force-with-lease` directly.
- Merge commits are forced to the `trivial` bucket regardless of score.
- If `<name>.dead` already exists, the original is auto-versioned to
  `.dead.1`, `.dead.2`, etc.
- In `--fabricate` mode, the synthesized history reproduces the source's exact
  final tree; the original commits and their messages are discarded.
- `--pigs` and `--rats` are mutually exclusive. Without either, `--fabricate`
  uses a single author from `git config user.*`.
- When the time window is too small for the fabricated commits, `--pigs`
  squashes linear edges to fit; single and `--rats` modes refuse instead (widen
  `--start`/`--end`), since squashing fabricated branch history there defeats
  the purpose.
- `--preserve` never squashes or linearizes; it re-times every commit in its
  original author-date order and proportional spacing, compressed into the
  window, so every commit survives and the history keeps its original shape
  (rebased, out-of-topological-order author dates and all). It works in both
  retime and fabricate modes, and fails only when the window is shorter than one
  second per commit. Because git timestamps are second-granular, that's the hard
  limit — widen the window rather than expecting sub-second spacing.
- `interrogate` never modifies the repository, and `--emit-mailmap` prints to
  stdout — it does not write `.mailmap` for you.

## Development

```bash
make test            # go test ./...
make vet             # go vet ./...
make fmt             # gofmt -w .
make clean           # rm -rf bin
```
