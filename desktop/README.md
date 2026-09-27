# caveira desktop

The desktop client: the same agent as the terminal one, in a window. Go and [Wails](https://wails.io) v2 on the back, React and TypeScript on the front, built with bun.

## What it does

Open a folder and talk to caveira about it. It reads, searches, edits, and runs commands in that folder the way the terminal client does, because it is the same agent: the loop, tools, prompt, and settings all come from the shared `core` module at the repository root.

The window is a sidebar and a conversation. The sidebar holds the project (switch between recent folders, open another with ⌘O, show it in the Finder) and its chats, newest first, with a dot on any that is still working; ⌘N starts a new one, and chats keep running when you switch away from them. In the conversation, your messages sit on the right and caveira's replies are plain markdown, code in panels with a copy button. Each tool call is one quiet line (`Read main.go · 11 lines`, `Edit main.go +1 −1`, `Run go test ./...`) that shimmers while it runs and opens to show its output or diff; a failed one turns red. A model's thinking folds into a "Thought" line. Each turn closes with how long it took, how many tools it ran, and what it cost. Under the composer: the model and reasoning effort (click to switch; the choice becomes the default for new chats), how full the context window is, and what the chat has spent. `enter` sends, `shift+enter` adds a line, `esc` stops.

With "Ask before commands and edits" on, a command or file change waits in the transcript as a card showing the command, or the lines an edit takes out and puts in, with Deny, Always allow, and Allow.

caveira runs on [abliteration.ai](https://abliteration.ai) or on a model on this machine. The first launch asks for an API key, or offers the local route: Ollama at `localhost:11434`, where models run as `caveira/` copies with room for the prompt, exactly as the terminal client's `--dev` does (see `core/local`). Settings (⌘,) switch between the two and hold the key, endpoint, default model, effort, the ask-first switch, and light, dark, or system appearance.

Chats are the same session files the terminal uses, under `~/.caveira/sessions/`, so a chat started in one opens in the other. The key, endpoint, model, effort, and ask-first setting are `~/.caveira/config.json`, shared with the terminal; `.env` files and the environment still win over it, and the settings screen says when they do. What only the app keeps (recent projects, appearance, the local model setting) is `~/.caveira/desktop.json`.

An app opened from the Dock does not get your shell's environment, so on launch it asks your login shell for it once, the way editors do: the agent's commands find `go`, `bun`, and whatever else your `PATH` has, and a key exported in your shell profile is seen.

## Build and run

From the repository root, `make` builds both clients, installs `caveira.app` into `/Applications` (and the terminal client into `~/.local/bin`), and opens the app. `make update` quits the running app first and opens the new one after. Each local build is signed anew, which macOS treats as a different app for folder access (Documents, Desktop, external drives), so `make` also clears the old build's grants and the new one asks again.

To build just the app: `make desktop`, which leaves it in `desktop/build/bin/caveira.app`. Wails is run with `go run`, so there is nothing to install besides Go, bun, and the Xcode command line tools.

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

Setting `HOME` keeps the harness's chats and settings out of your real ones.

## Layout

- `main.go` — the window, its menu, and its options.
- `app.go` — what the window can ask for: projects, settings, model lists.
- `chat.go` — chats: building an agent from the settings, running turns, approvals, and turning agent events into the transcript and the events the window hears.
- `transcript.go` — rebuilding a saved chat's transcript from its messages.
- `prefs.go` — `~/.caveira/desktop.json`.
- `shellenv.go` — the login shell's environment for a Dock launch.
- `harness_test.go` — the browser harness.
- `frontend/src/lib` — the bridge to Go, the store, theme, and formatting.
- `frontend/src/components` — the sidebar, chat pane, transcript, tool rows, composer, settings, and first-run screens.
- `frontend/src/styles.css` — the whole look: the icon's off-white and black, and the steps between them.
