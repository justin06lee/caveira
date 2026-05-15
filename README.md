# Caveira

Rewrite a git repository's commit timestamps to fit a chosen time window.

Caveira takes a repo plus a `--start` and `--end`, scores each commit's
"difficulty" from its diff stats, draws a duration per commit, and produces
a copy whose history fits inside the requested window — scaling and (if
necessary) squashing commits to fit. The original folder is renamed to
`<name>.dead[.N]`; the rewritten copy takes the original name.

## Build

```bash
make build
```

Two byte-identical binaries are produced: `bin/caveira` and `bin/cav`.

## Usage

```bash
caveira --repo /path/to/myrepo \
        --start "2026-05-14 13:00" \
        --end   "2026-05-14 17:00"

cav --repo https://github.com/u/myrepo.git \
    --start "tomorrow 9am" \
    --end   "tomorrow 5pm" \
    --seed 42 \
    --dry-run
```

See `docs/superpowers/specs/2026-05-14-caveira-design.md` for the full design.

## Notes

- Caveira preserves each commit's author and committer identity. Only
  timestamps change.
- GPG signatures on source commits are stripped — rewriting invalidates them.
- `--push` uses go-git's `Force: true`, which is not strictly `--force-with-lease`.
