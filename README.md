<p align="center">
  <img src="docs/caveira.png" alt="Caveira" width="180" />
</p>

<h1 align="center">Caveira</h1>

<p align="center">
  <em>Interrogate a git repository's commit history.<br/>
  Rewrite its timestamps to fit a chosen time window.</em>
</p>

---

Caveira takes a repo plus a `--start` and `--end`, scores each commit's
"difficulty" from its diff stats, draws a duration per commit, and produces
a copy whose history fits inside the requested window — scaling and (if
necessary) squashing commits to fit. The original folder is renamed to
`<name>.dead[.N]`; the rewritten copy takes the original name.

Author and committer identities are preserved exactly. Only timestamps change.

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

## Flags

| Flag               | Required | Description                                              |
|--------------------|----------|----------------------------------------------------------|
| `--repo`           | yes      | Filesystem path or `https://` URL                         |
| `--start`          | yes      | Window start (`2026-05-14 13:00`, `tomorrow 9am`, `now`)  |
| `--end`            | yes      | Window end                                                |
| `--seed`           |          | Integer seed for reproducible duration draws              |
| `--dry-run`        |          | Print the schedule, write nothing                         |
| `--push`           |          | Force-push `origin` after the swap                        |
| `--push-protected` |          | Allow `--push` to touch `main` / `master`                 |
| `--window-tz`      |          | IANA timezone for parsing `--start`/`--end` (default `Local`) |
| `--out-dir`        |          | Parent dir for URL clones (default `$CWD`)                |

Run `caveira --help` for the live flag reference.

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
6. Duplicate the source folder, write the new commits, rebuild refs, swap
   the rewritten copy into the original location and rename the original to
   `<name>.dead`.

Full design: [`docs/superpowers/specs/2026-05-14-caveira-design.md`](docs/superpowers/specs/2026-05-14-caveira-design.md).

## Notes & limitations

- Author and committer name/email are preserved. Both timestamps are rewritten
  to the new schedule, in each commit's original timezone.
- GPG signatures are stripped — rewriting invalidates them. A warning is
  printed when source commits are signed.
- `--push` uses go-git's `Force: true`. go-git does not yet support
  `--force-with-lease` directly.
- Merge commits are forced to the `trivial` bucket regardless of score.
- If `<name>.dead` already exists, the original is auto-versioned to
  `.dead.1`, `.dead.2`, etc.

## Development

```bash
make test            # go test ./...
make vet             # go vet ./...
make fmt             # gofmt -w .
make clean           # rm -rf bin
```
