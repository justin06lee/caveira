# caveira tui

The terminal client for caveira, written in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) v2 and Lip Gloss v2.

## Run

From this folder:

```sh
go run .
```

Press `q` or `Ctrl+C` to quit.

## Install

From the repo root, `make` builds the binary and installs it as `caveira` in `~/.local/bin`. Set `BINDIR` to install somewhere else, for example `make BINDIR=/usr/local/bin`. `make update` removes the old binary and installs a fresh build.
