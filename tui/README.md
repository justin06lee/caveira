# caveira tui

The terminal client for caveira, written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) v2 and Lip Gloss v2.

## What it does so far

On launch it decides between three screens:

- **No saved token** — offers *Log in* or *Sign up*. Either one starts a device login: the CLI asks the backend for a code, opens your browser at `/login` (or `/signup`) with the approval page queued behind it, and waits. Approving in the browser hands the CLI a session token, saved `0600` at `~/.caveira/auth.json`.
- **Signed in, no subscription** — draws the plan cards from `GET /api/plans` and opens Stripe Checkout for the one you pick. When the backend has no Stripe key it is in dev billing mode and the plan activates on the spot.
- **Signed in and paid** — the placeholder session screen, which is where the agent itself will live.

## Run

From this folder, against a backend:

```sh
CAVEIRA_API_URL=http://localhost:3000 go run .
```

`CAVEIRA_API_URL` defaults to `https://caveira.dev`, which does not exist yet, so point it at your local `web-client` while developing. `q` or `Ctrl+C` quits; `s` on the session screen signs out and deletes the saved token.

## Install

From the repo root, `make` builds the binary and installs it as `caveira` in `~/.local/bin`. Set `BINDIR` to install somewhere else, for example `make BINDIR=/usr/local/bin`. `make update` removes the old binary and installs a fresh build.

## Layout

- `main.go` — loads the saved token and starts the program.
- `internal/config` — `~/.caveira/auth.json` and the `CAVEIRA_API_URL` setting.
- `internal/api` — HTTP client for the shared backend; the token rides as a bearer.
- `internal/ui` — the Bubble Tea model, one file each for state, views, styles, and commands.
