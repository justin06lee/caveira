# Caveira — Design Spec

**Date:** 2026-05-14
**Status:** Approved for planning

## 1. Purpose

Caveira is a Go CLI that takes a git repository (local clone or public URL) plus a target time window, and produces a rewritten copy of that repository whose commit timestamps fall inside the window. The new timestamps are derived per-commit from an inferred "difficulty" score, so the resulting history looks like a plausible burst of work compressed into the chosen window.

The original repository is preserved (renamed to `<name>.dead{,.N}`); the rewritten copy takes the original folder name.

## 2. User flow

1. User invokes `caveira` (or its alias `cav`) with a repo, a start, and an end.
2. Caveira either clones the URL or duplicates the local folder into `<name>.interrogating`.
3. Caveira scores each commit's difficulty from its diff stats, draws a duration per commit, and produces a schedule that fits the time window — scaling and (if necessary) squashing commits to fit.
4. Caveira rewrites the staged copy: every commit gets a new author/committer timestamp; squashed pairs become single commits; refs are rebuilt; old objects are pruned.
5. Caveira renames the original to `<name>.dead` (auto-versioning on collision) and the staged copy to `<name>`.
6. If `--push` was passed, force-pushes (with lease) to `origin`.

## 3. CLI surface

```
caveira --repo <path-or-url>
        --start <datetime>
        --end   <datetime>
        [--seed <int>]
        [--dry-run]
        [--push]
        [--push-protected]
        [--window-tz <IANA tz, default: system local>]
        [--out-dir <dir>]
```

`cav` is a byte-identical alias binary built from the same `main` package.

### Flags

| Flag | Required | Description |
|---|---|---|
| `--repo` | yes | Filesystem path or `https://`/`git@` URL. URLs are cloned into `--out-dir` (default `$CWD`). |
| `--start` | yes | Window start. Parsed with flexible formats (see §3.1). |
| `--end` | yes | Window end. Same parsing rules. |
| `--seed` | no | Integer seed for the per-commit duration draws. Default: unseeded (true randomness). |
| `--dry-run` | no | Print the schedule (table of old → new timestamps + difficulty + duration). Do not write to disk. |
| `--push` | no | After the swap, `git push --force-with-lease --all && --tags` to `origin`. |
| `--push-protected` | no | Required to push when any pushed branch is `main` or `master`. |
| `--window-tz` | no | IANA timezone used to interpret `--start`/`--end` strings that lack explicit offsets. Default: system local. |
| `--out-dir` | no | Parent directory for URL clones. Default: `$CWD`. Ignored when `--repo` is a path. |

### 3.1 Date/time input

Parsed via `github.com/araddon/dateparse` (or equivalent). Accepted forms include:

```
"2026-05-14 13:00"
"2026-05-14 1pm"
"5/14 1pm"          # current year inferred
"May 14 1pm"        # current year inferred
"tomorrow 5pm"      # relative; resolved against now in --window-tz
"now"
```

Strings without explicit timezone offsets are interpreted in `--window-tz`. Unparseable inputs error with a clear "try `YYYY-MM-DD HH:MM`" message.

### 3.2 Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | User error (bad args, window too small even after merging, path collision, source has no commits) |
| 2 | System error (clone failure, disk/io, git object corruption) |

## 4. Difficulty model

### 4.1 Score formula

Per commit:

```
score = lines_changed + files_touched * 10 + new_files * 25
```

- `lines_changed` = sum of insertions + deletions from `git diff --numstat`. Pure renames count as 0.
- `files_touched` = distinct file paths in the diff.
- `new_files` = files where the parent path did not exist.
- Root commits are diffed against the empty tree.
- **Merge commits are forced to `trivial` regardless of score.**

### 4.2 Buckets

| Difficulty   | `score` upper bound | Base (min) | Deviation (±min) |
|--------------|---------------------|------------|------------------|
| trivial      | 10                  | 5          | 3                |
| easy         | 50                  | 15         | 7                |
| medium       | 200                 | 30         | 13               |
| hard         | 600                 | 60         | 17               |
| substantial  | (greater)           | 90         | 23               |

Thresholds are constants in `internal/difficulty`; they're tunable in code, not at the CLI.

### 4.3 Duration draw

```
duration_minutes = base + uniform_int_inclusive(-deviation, +deviation)
```

Bidirectional. Seeded by `--seed` when supplied.

## 5. Scheduling

Operates on the union of all commits reachable from any ref (branches + tags) in the source repo.

### 5.1 Topological schedule with overlap

For each commit `i` in topological order:

```
start_i = max(end_p for p in parents(i))    if parents(i) non-empty
        = window_start                       otherwise
end_i   = start_i + d_i
```

The new author and committer timestamp of commit `i` is `end_i`, expressed in commit `i`'s original timezone. Sibling branches naturally overlap in wall-clock time — both children of a commit `B` start at `end_B`.

Total span: `T = max(end_i across all commits) - window_start`. Goal: `T ≤ window_size`.

### 5.2 Scaling

If `T > window_size`, scale all `d_i` by `s ∈ (0, 1]`:

```
d_i'(s) = round( (base_i + offset_i) * s )
```

Span is the longest end-to-end path through the DAG; every edge scales by `s`, so span scales (near-)linearly with `s` (subject to integer rounding). Compute the candidate directly:

```
s = window_size / T,  clamped to [0.5, 1.0]
```

Recompute the schedule with the scaled durations. If rounding leaves the span slightly over, decrement `s` by one tick (`0.01`) and recompute; repeat at most a few times.

**Scaling floor:** `s = 0.5` corresponds to the minimum trivial duration (`base − deviation = 5 − 3 = 2` minutes) reaching 1 minute. If `s = 0.5` still doesn't fit, lock `s = 0.5` and proceed to merging.

### 5.3 Merging (squash)

Loop:

1. Among **linear** parent→child edges `(p, c)` — i.e. `p` has exactly one child and `c` has exactly one parent — pick the edge minimizing `min(d_p, d_c)`. Tiebreak by earliest original AuthorDate of `p` (deterministic given the input repo).
2. Squash:
   - **Surviving metadata** (message, author, committer): from whichever of `p`, `c` has the larger `d_*`.
   - **Surviving tree:** `c`'s tree (the later state — preserves all of `c`'s changes).
   - **Position in DAG:** the squashed commit replaces both. Its parents are `parents(p)`; its children are `children(c)`.
   - **Duration:** `max(d_p, d_c)`.
3. Recompute the schedule from §5.1. If `T ≤ window_size`, done.

**Deadlock fallback (linearization):** if no linear edges remain and `T > window_size`, collapse a branch point: find the smallest sibling branch (by sum of `d_i`) emerging from a branch point and squash it into its sibling, then re-run merging. Apply recursively as needed.

If even a fully linearized history can't fit (e.g. one giant commit's scaled-floor duration exceeds the window), hard-fail with:

```
caveira: cannot fit history into requested window even after maximum
merging. Minimum feasible span: <N> minutes. Widen the window.
```

## 6. Git rewrite mechanics

Implemented with `github.com/go-git/go-git/v5`. The destination is a fresh repo seeded by copying the source, then having its history rebuilt.

### 6.1 Rewrite procedure

1. Open source repo. Walk all refs; collect the set of all reachable commit OIDs.
2. Build the commit DAG in memory: per node, `oid`, `parents[]`, `tree`, `author`, `committer`, `message`, plus computed `lines_changed`, `files_touched`, `new_files`.
3. Compute `difficulty` and draw `duration` for each commit (merges forced trivial).
4. Run the scheduler (§5). Apply any squashes to the in-memory DAG, producing a `rewrite_plan`: for each surviving commit, the new `(parents, tree, author, committer, message, time)`.
5. Copy the source folder to `<name>.interrogating`.
6. In the destination repo: delete every ref. In topological order, write new commit objects from the plan, maintaining an `old_oid → new_oid` map so children reference the right new parent OIDs.
7. Recreate refs:
   - Branches → new tip OIDs.
   - Lightweight tags → new OIDs.
   - Annotated tags → new tag objects, same tagger/message, retargeted, tagger date set to new commit date.
8. Reset working tree to match new HEAD.
9. `git gc --prune=now --aggressive` to delete the orphaned old commit objects.

### 6.2 Author/committer/timestamps

- **Author name + email:** preserved exactly.
- **Committer name + email:** preserved exactly.
- **AuthorDate + CommitterDate:** both set to the new scheduled `end_i`, in the commit's original timezone.
- **GPG signatures:** `gpgsig` headers are stripped; rewriting invalidates them. A single warning is printed if any source commit was signed.

## 7. Folder operations

### 7.1 Layout

**Path input** (`--repo /path/to/myrepo`):

```
/path/to/myrepo                 (source, untouched until final rename)
/path/to/myrepo.interrogating   (staging; built by Caveira)
/path/to/myrepo.dead[.N]        (post-swap: original)
/path/to/myrepo                 (post-swap: rewritten)
```

**URL input** (`--repo https://github.com/u/myrepo.git`):

- Clone into `<out-dir>/myrepo` (basename, `.git` stripped).
- Proceed identically to the path-input case with `<out-dir>/myrepo` as the source.

### 7.2 Pre-flight checks

| Condition | Behavior |
|---|---|
| `<name>.interrogating` exists | Exit 1: "remove or rename before retrying" |
| `<name>.dead` exists | Auto-version: use `<name>.dead.1`, `.dead.2`, … (first free index) |
| Source has no commits | Exit 1 |
| Source working tree dirty | Warn, proceed (only committed history is read) |
| `--start >= --end` | Exit 1 |
| `--start` or `--end` unparseable | Exit 1 with format hint |

### 7.3 Atomicity and rollback

- Rewriting happens entirely inside `<name>.interrogating`. The source folder is untouched until both final renames succeed.
- Final renames are sequenced: source → `<name>.dead[.N]`, then `<name>.interrogating` → source's old name.
- If the second rename fails (rare; cross-filesystem, permission change), the tool attempts to roll back the first.
- On any earlier failure, `<name>.interrogating` is left in place for inspection; the user removes it before retrying.

### 7.4 Push behavior

- `--push` runs, in order:
  - `git push --force-with-lease --all origin`
  - `git push --force-with-lease --tags origin`
- If any branch being pushed is named `main` or `master`, refuses unless `--push-protected` is also passed.
- `--force-with-lease` (not `--force`) so the tool refuses to overwrite truly unexpected upstream state.

## 8. Output

### 8.1 Dry-run output

A table per commit:

```
SHA(short)  Difficulty   Duration  Original time              New time
abc1234     trivial      4m        2025-11-02T14:31:00-04:00  2026-05-14T13:04:00-04:00
def5678     hard         77m       ...                        ...
...

Span: 02h 14m (window: 04h 00m). Scaling: s=1.00. Squashes: 0.
```

### 8.2 Normal-run summary

```
Source:        /path/to/myrepo
Rewritten:     /path/to/myrepo          (was .interrogating)
Original kept: /path/to/myrepo.dead.2
Commits:       142 → 138 (4 squashed)
Span:          03h 51m within 04h 00m window
Scaling:       s=0.85
Pushed:        no
```

## 9. Project structure

```
caveira/
├── cmd/
│   ├── caveira/main.go          # entry point
│   └── cav/main.go              # alias, calls into cmd/caveira
├── internal/
│   ├── input/                   # CLI parsing, date parsing
│   ├── repo/                    # source clone/copy, folder swap, push
│   ├── walk/                    # DAG load via go-git, diff stats per commit
│   ├── difficulty/              # score formula + bucket assignment
│   ├── schedule/                # topological scheduler, scaling, merging
│   ├── rewrite/                 # write new commit objects, rebuild refs
│   └── report/                  # dry-run table, summary output
├── testdata/
│   └── fixtures/                # small bare repos for integration tests
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### 9.1 Dependencies

- `github.com/go-git/go-git/v5` — git object/ref manipulation.
- `github.com/araddon/dateparse` — flexible date parsing.
- `github.com/spf13/cobra` — CLI plumbing.
- stdlib `math/rand` (seeded) for duration draws.
- `golang.org/x/sync/errgroup` for parallel diff computation.

### 9.2 Testing

- Unit tests per `internal/*` package.
- Integration tests against fixture bare repos in `testdata/` covering:
  - Single linear chain (matches the example in the original brief).
  - Branched-and-merged DAG.
  - Window too narrow → scaling.
  - Window much too narrow → scaling + merging.
  - Window impossibly narrow → hard fail.
  - URL clone path (mocked transport).
- Property test for the scheduler: any valid input either produces a schedule with `span ≤ window` or hard-fails predictably.

## 10. Non-goals (v1)

- Private repo cloning via SSH or token auth (the user can pre-clone locally and pass a path).
- Rewriting history in place on the source folder.
- A `--config` file or per-difficulty user-overrides at the CLI; thresholds and durations live in `internal/difficulty`.
- A TUI/interactive mode.
- Anything beyond `origin` for `--push`.
