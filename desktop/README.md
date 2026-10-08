# caveira desktop

The desktop client: the same agent as the terminal one, in a window. Go and [Wails](https://wails.io) v2 on the back, React and TypeScript on the front, built with bun.

## What it does

Open a project and talk to caveira about it. It reads, searches, edits, and runs commands in that folder the way the terminal client does, because it is the same agent: the loop, tools, prompt, and settings all come from the shared `core` module at the repository root.

The first launch takes two steps, both skippable and both in Settings afterwards. First it offers to bring over the chats other coding agents left on this computer, from Claude Code, Codex, and OpenCode, grouped by app and project; see [Importing](#importing). Then it asks where your projects live, with the folder most of the known projects share already typed in.

Projects are opened with the picker (⌘O, and the whole window when no project is open): a box to type in, and the workspace folder's contents under it, one line each, the way `ls -a` would list them. Typing narrows the list to what starts with the last part of the path, and a slash goes into a folder. Tab completes as far as the matches agree and, when they agree no further, steps through them the way a shell's menu completion does. `..` and enter go up a level for as long as the picker is open; `~/` and `/` start from home or the root. Hidden files show with the eye in the box, or ⌘⇧. as in the Finder (Ctrl+H on Linux, as in GTK), or when what is typed starts with a dot. Enter opens the highlighted folder, or a folder typed with a slash after it; a name that is not there is offered as a new folder. Browse… is the Finder's own dialog.

The window is a sidebar and a conversation. The sidebar lists your projects as folders, most recently used first; click one to open it and its chats are inside, newest first, so a chat in any project is one click away. A folder shows six chats until you ask for the rest, remembers whether it was open, and puts a dot on itself while a chat inside it is working. Hover a folder for + (a new chat there) and × (take it off the sidebar; the folder and its chats stay on disk), or right-click it for those and Show in Finder (Open in Files on Linux). ⌘N starts a new chat in the project you are in, ⌘O opens another project, and chats keep running when you switch away from them. In the conversation, your messages sit on the right and caveira's replies are plain markdown, code in panels with a copy button. Each tool call is one quiet line (`Read main.go · 11 lines`, `Edit main.go +1 −1`, `Run go test ./...`) that shimmers while it runs and opens to show its output or diff; a failed one turns red. A model's thinking folds into a "Thought" line. While caveira works, a line under the conversation says what it is up to the way the terminal's status line does, with one of the same rebel verbs (`Plotting…`, `Casing the joint…`) and how long the turn has run. An empty chat asks what the job is, in a line that goes with the time of day. Each turn closes with how long it took, how many tools it ran, and what it cost. Under the composer: the model and reasoning effort (click to switch; the choice becomes the default for new chats). The menu lists the models caveira offers, abliterated-model and the GLM ones abliteration.ai serves (GLM-5.2 is `abliterated-model-large`, GLM-5.3 `abliterated-model-large-v2`), each with its makers' logos; More models… lists the ones on this machine, and picking one moves the chat there. Then how full the context window is, and what the chat has spent. `enter` sends, `shift+enter` adds a line, `esc` stops.

With "Ask before commands and edits" on, a command or file change waits in the transcript as a card showing the command, or the lines an edit takes out and puts in, with Deny, Always allow, and Allow.

A chat runs only on a plan: Free (abliterated-model, a small usage limit), or Lightweight, Middleweight, Heavyweight, and Champion at $20, $50, $100, and $200 a month, which add the GLM models and more usage. Without one, a message sent stays on screen with the plans under it, and goes once a plan is picked; the same holds for a GLM model on Free. Settings shows the plan and changes it. There is no caveira cloud to pay yet, so the plan picked is kept in `desktop.json` and nothing is charged; `plans.go` is where that check lives.

caveira runs on [abliteration.ai](https://abliteration.ai) or on a model on this machine. The first launch asks for an API key, or offers the local route: Ollama at `localhost:11434`, where models run as `caveira/` copies with room for the prompt, exactly as the terminal client's `--dev` does (see `core/local`). A model too big for the machine's memory at that window steps down to a smaller window, then to a smaller model, with a line in the chat saying so. Settings (⌘,), a page in place of the conversation, switch between the two and hold the plan, the key, endpoint, default model, effort, the ask-first switch, and light, dark, or system appearance.

Chats are the same session files the terminal uses, under `~/.caveira/sessions/`, so a chat started in one opens in the other. The key, endpoint, model, effort, and ask-first setting are `~/.caveira/config.json`, shared with the terminal; `.env` files and the environment still win over it, and the settings screen says when they do. What only the app keeps (recent projects, the workspace, appearance, the local model setting, the plan) is `~/.caveira/desktop.json`.

An app opened from the Dock (or GNOME's app grid) does not get your shell's environment, so on launch it asks your login shell for it once, the way editors do: the agent's commands find `go`, `bun`, and whatever else your `PATH` has, and a key exported in your shell profile is seen.

## Importing

File → Import from Other Agents…, Settings, or the first launch. Each app's chats are read into caveira's own session format by `core/importer`: tool calls caveira also has become its own (a Claude Code `Edit` is an `edit_file`, with the diff Claude Code kept beside it), the text agents slip into user messages (environment blocks, reminders, slash-command records) is left out, a compacted chat starts from its summary as caveira's own do, and every tool call is answered so the history is one an endpoint accepts when the chat is carried on. Tool results are cut to 24 KB each. A tool caveira does not have keeps its name and input and shows as a line of its own.

Where each app keeps its chats: Claude Code in `~/.claude/projects` (or `$CLAUDE_CONFIG_DIR`), Codex in `~/.codex/sessions` (or `$CODEX_HOME`), OpenCode in `~/.local/share/opencode/opencode.db` (or `$XDG_DATA_HOME`), read with the `sqlite3` that ships with macOS (on Linux, install the `sqlite3` package). Chats that ran in temporary folders or inside app bundles, subagents' threads, and Codex's own copies of Claude Code chats are left out. The originals are only read.

An imported chat keeps its id in the other app (`claude-<id>`, `codex-<id>`, `opencode-<id>`), so importing again brings over only what is new; a chat already imported is not refreshed from the original. Chats with nothing said in them are remembered in `desktop.json` and not offered again. Their folders join the recent projects, up to forty.

## Build and run

From the repository root, `make` builds both clients, installs `caveira.app` into `/Applications` (and the terminal client into `~/.local/bin`), and opens the app. `make update` quits the running app first and opens the new one after. Each local build is signed anew, which macOS treats as a different app for folder access (Documents, Desktop, external drives), so `make` also clears the old build's grants and the new one asks again.

To build just the app: `make desktop`, which leaves it in `desktop/build/bin/caveira.app` (on Linux, the binary `desktop/build/bin/caveira`). Wails is run with `go run`, so there is nothing to install besides Go, bun, and the Xcode command line tools. With the full Xcode installed, the build also compiles the app's Icon Composer icon; without it the app keeps the classic icon, which macOS 26 shades with glass in the Dock.

On Linux, `make` installs the binary as `~/.local/bin/caveira-desktop` with a launcher and icon under `~/.local/share`, so it is in the app grid; it needs GTK 3 and WebKitGTK 4.1 headers (see the [Linux section](../README.md#linux) of the main README). The window keeps the system title bar and has no menu bar; the page takes Ctrl+N, Ctrl+O, Ctrl+, and Ctrl+\ itself.

## Working on it

```sh
wails dev                               # live-reloading window (go install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0)
go test ./...                           # the backend: turns, approvals, transcripts, settings
cd frontend && bun run build            # typecheck and bundle the frontend
```

WebKit does not paint a window that is behind others, so to look at the UI without bringing the app forward there is a harness: it serves the built frontend with a stand-in for the Wails bridge, backed by the real Go side, for any browser.

```sh
cd frontend && bun run build && cd ..
HOME=/tmp/caveira-home CAVEIRA_HARNESS=localhost:34999 go test -run TestHarness -timeout 0 .
```

Setting `HOME` keeps the harness's chats and settings out of your real ones. To try the import on real chats, link `~/.claude`, `~/.codex`, and `~/.local/share/opencode` into that home; the importer only reads them. The frontend is embedded when the test builds, so restart the harness after `bun run build`.

## Layout

- `main.go` — the window, its menu (macOS only), and its options for macOS and Linux.
- `app.go` — what the window can ask for: projects, settings, model lists.
- `files.go` — the folder picker's listings, new folders, the workspace, and the end of the first run.
- `imports.go` — finding and importing other agents' chats (the reading itself is `core/importer`).
- `chat.go` — chats: building an agent from the settings, running turns, approvals, and turning agent events into the transcript and the events the window hears.
- `plans.go` — the plans, which models each runs, and holding a message until a plan runs it; also the models offered on abliteration.ai.
- `transcript.go` — rebuilding a saved chat's transcript from its messages.
- `prefs.go` — `~/.caveira/desktop.json`.
- `shellenv.go` — the login shell's environment for a Dock launch.
- `harness_test.go` — the browser harness.
- `frontend/src/lib` — the bridge to Go, the store, theme, formatting, and the rebel lines (`rebel.ts`).
- `frontend/src/components` — the sidebar, chat pane, transcript, tool rows, composer, settings, the plans (`Plans`), the model makers' logos (`ModelLogos`), the first run (`Onboarding`, `ImportPanel`), and the folder picker (`PathPicker`, shown by `Welcome` and `OpenProject`).
- `frontend/src/styles.css` — the whole look: the icon's off-white and black, and the steps between them.
- `build/appicon.png` — the classic icon Wails packages, for macOS before 26.
- `build/linux/caveira.desktop` — the Linux launcher `make` installs, with the binary's path filled in.
- `build/darwin/caveira.icon` — the Icon Composer icon for macOS 26, with glass, highlight, and shadow off so the Dock shows it flat: the eye from `assets/caveira-icon.svg` on a solid `#F8F7F2` fill.
