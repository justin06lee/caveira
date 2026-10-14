<div align="center">

<img src="assets/caveira-icon.svg" alt="caveira" width="160" />

# caveira

**A Claude Code–style coding agent for abliterated models.**<br>
*Your model, your machine, no refusals.*

</div>

---

caveira is an agentic coding assistant in the spirit of Claude Code, built to drive abliterated open-weight models, meaning models whose refusal behavior has been removed. You give it a task in your project; it reads and searches the code, edits files, runs commands, checks its own work, and tells you what it did. It does not second-guess what you asked for. The only pushback it gives is technical.

It comes as a terminal client and a desktop app, which share one agent and one set of sessions. The caveira website, [caveira.ai](https://github.com/justin06lee/caveira.ai), holds the accounts the clients sign in to, and runs their model requests; hosted sessions come later.

## Quick start

```sh
curl -fsSL https://caveira.ai/install.sh | sh   # installs `caveira` (and `cav`) into ~/.local/bin
cd your/project && caveira                       # then /login to sign in to your caveira account
```

The script downloads the latest release built for your machine (macOS or Linux, Apple silicon/arm64 or x86-64), checks it against the release's checksums, and puts it in `~/.local/bin`; run it again to update. `CAVEIRA_VERSION=v0.2.0` installs a given release and `CAVEIRA_INSTALL_DIR` another folder. From a clone, `make` builds and installs both clients instead, the app included (see below).

`/login` opens the caveira site in your browser; once you connect this machine there, the session carries on through your account, with no key to paste anywhere. On a machine you reach over SSH, `caveira login` prints the page and the code instead. Without an account the terminal client sends nothing; an abliteration.ai key of your own does not stand in for one. For development, `--base-url` (or `CAVEIRA_BASE_URL`) points it at another OpenAI-compatible server, a local model say, with that server's own key.

In the app, open a folder, sign in to your caveira account (it opens the caveira site in the browser), and type. The app runs on abliteration.ai's models alone, through the account, so it has no field for a key; a chat there runs on the account's plan, Free or paid, bought on the caveira site.

Signed in, caveira's model requests go through your caveira account, which runs them on [abliteration.ai](https://abliteration.ai). Any other endpoint that speaks the same OpenAI-compatible protocol works too; a local Ollama is one flag away:

```sh
caveira --base-url http://localhost:11434/v1 --model llama3.2:latest
```

Run `caveira`, or `cav` for short. It opens on the pixel skull, dissolving in while everything loads, with the prompt centred under it; the first message slides the prompt to the bottom and the conversation takes over. Type what you want. `esc` interrupts, `/help` lists the commands, `caveira -c` continues the last session for the directory, and `caveira -p "…"` runs a task without the interface and prints the result. The full reference, including configuration, is in [tui/README.md](tui/README.md).

The desktop app is the same agent in a quiet window: projects and their chats in a sidebar, the conversation with its tool calls as single lines that open to show output and diffs, approvals as cards in the transcript, and a composer that switches model and effort. It reads the same settings and sessions as the terminal client, so a chat started in one continues in the other. On first launch it offers to bring over your chats and projects from Claude Code, Codex, and OpenCode, and projects open from a type-to-filter folder picker with tab completion. See [desktop/README.md](desktop/README.md).

## How it works

Every turn, caveira sends the conversation and its tool definitions to the model and streams the reply. When the model calls a tool, caveira runs it, appends the result, and asks again, until the model answers in words. The tools are the same seven that make Claude Code useful: read a file with line numbers, write one, replace an exact string (with a diff back), run a shell command, find files by glob, search contents by regex, list a directory.

The system prompt is caveira's own, written for abliterated models: it tells the model to do the task as asked without moralizing or checking in, and then spends most of its words on what makes an agent good at code. Understand before changing, keep edits minimal and in the project's style, read before editing, verify with the build and the tests, fix root causes rather than symptoms, report faithfully, be brief. Instructions from `CAVEIRA.md`, `AGENTS.md`, or `CLAUDE.md` in the project are appended to it.

Conversations are saved after every turn under `~/.caveira/sessions/`, and when one approaches the model's context window the agent summarizes it into a handoff note and carries on.

## Layout

- `core/` is the agent itself, shared by both clients: the OpenAI-compatible client (`llm`), the tools, the system prompt, the turn loop and sessions (`agent`), settings (`config`), and reading other agents' chats into caveira sessions (`importer`).
- `tui/` is the terminal client, in Go with Bubble Tea v2. See [tui/README.md](tui/README.md).
- `desktop/` is the desktop app, in Go with Wails v2 and a React and TypeScript frontend built with bun. See [desktop/README.md](desktop/README.md).
- `assets/` holds the app icon: a softly rounded, pure-black skull eye socket on an off-white (`#F8F7F2`) squircle.

`make` (or `make update`, the same thing) brings every caveira on this machine up to date: it builds both clients first, so a build that fails leaves the installed ones working, then quits the app, swaps in `caveira` with a `cav` link beside it and the app, points any older copy of caveira on your `PATH` (an old `go install` in `~/go/bin`, say) at the new one, and opens the app. A Go module cache that something has emptied part of is repaired on the way. `make install` does all that but opening the app, `make build` only builds, and `make test` runs the Go tests of all three modules. Set `BINDIR` to install the terminal client somewhere other than `~/.local/bin`, and `APPDIR` for the macOS app.

Pushing a tag like `v0.2.0` runs the release workflow (`.github/workflows/release.yml`): it tests `core` and `tui`, builds the terminal client for macOS and Linux on arm64 and x86-64 with `make dist`, and publishes a GitHub release with a `caveira_<os>_<arch>.tar.gz` for each and a `checksums.txt`, which is what `install.sh` downloads; then a macOS runner builds the app with `make dmg`, for Apple silicon and Intel in one, and attaches `caveira.dmg`, which [caveira.ai/download/macos](https://caveira.ai/download/macos) hands out. `make dist dmg` builds the same files into `dist/` locally. The app is signed ad hoc, not with a Developer ID, so the first time it opens macOS asks for Open Anyway in System Settings → Privacy & Security.

## Linux

caveira builds and runs on Linux too (tested on Ubuntu 24.04, arm64, under GNOME). You need Go (the modules ask for 1.27; an older `go` fetches it), [bun](https://bun.sh), and the headers Wails builds the app against:

```sh
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev \
                 sqlite3 xclip            # sqlite3 reads OpenCode's chats; xclip (or wl-clipboard) is the terminal client's copy
```

`make` then installs `caveira` and `cav` into `~/.local/bin`, the app as `~/.local/bin/caveira-desktop`, its launcher as `~/.local/share/applications/caveira.desktop` and its icon under `~/.local/share/icons`, so it shows in the app grid, and opens it. `git pull && make` keeps it current, quitting the running app first. With WebKitGTK 4.1 present the build links it (Ubuntu 24.04 has no 4.0), and the app turns off WebKit's DMA-BUF renderer, which leaves the window blank on NVIDIA drivers and under Xvfb. The window keeps the system's title bar and has no menu bar: Ctrl+N, Ctrl+O, Ctrl+, and Ctrl+\ do what the Mac menu's ⌘ shortcuts do, and Import is in Settings.
