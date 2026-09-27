# caveira tui

The terminal client: the coding agent itself, in Go with [Bubble Tea](https://github.com/charmbracelet/bubbletea) v2 and Lip Gloss v2.

## What it does

You describe a task; caveira reads and searches the project, edits files, runs commands, and checks its own work, streaming everything to the screen as it happens. It talks to any OpenAI-compatible chat completions endpoint: abliteration.ai by default, or a local Ollama / vLLM / llama.cpp server for testing.

```sh
caveira                          # interactive session in the current directory
caveira "add a --json flag"      # same, with the first message already sent
caveira -p "explain main.go"     # one-shot: run the task, print the reply, exit
caveira --dev                    # same interface, on a local model instead of the API
caveira -c                       # continue the latest session for this directory
```

`cav` is the same program under a shorter name; `make` installs both.

On launch the pixel skull dissolves in and the name types itself in under it while settings, git state, and the previous session load on another goroutine; any key skips the intro. The prompt then appears centred under them, with the model and directory beneath it, in the same place the intro left room for, so nothing moves. The skull is drawn as large as the terminal's height allows, and left out on very small ones. The first prompt slides the box to the bottom as the skull dissolves out, and the transcript opens with a card of the session's model and directory.

The skull is drawn in half-block cells, two pixels to a cell. Terminals with 256 colours or true colour get the coloured skull, bone on a black outline; 16-colour terminals, `NO_COLOR`, and dumb terminals get the one-colour skull, drawn in the terminal's own foreground. caveira asks the terminal for its background colour and switches to a light palette on light backgrounds.

In the transcript, your messages are chat bubbles on the right, with corners cut to quarter cells (as round as a terminal cell gets) and a small tail; each tool call is a line (`Read main.go`, `Bash $ go test ./...`) with its result under it, edits show a numbered diff on tinted rows, and each turn closes with how long it took, how many tools it ran, the tokens it used, and what it cost. While the model works, the line above the input says what it is up to, for how long, and roughly how many tokens have streamed: a running tool says what it runs (`Running go test`, `Editing main.go`), and while the model thinks or writes it is the underground at work, a new line each step (`Casing the joint…`, `Cutting the wires…`, `Throwing pocket sand…`). Messages you send while it works wait at the bottom of the transcript as dashed bubbles and go out together when the turn ends; `↑` takes the newest back to edit. Typing `/` opens a command palette under the input: arrows choose, `tab` completes, `enter` runs. `/model` opens a picker in the input's place with the models the endpoint serves and the reasoning effort under them: `↑`/`↓` choose the model, `←`/`→` the effort, `enter` switches. Under `--confirm`, approvals are a card with three choices, picked with the arrows and `enter` or with `1`–`3` (`y`, `a`, `n` also work). The footer shows the keys that matter at the moment on the left, and the model, effort, tokens used, a context-window meter, and the session's spend on the right.

In a session, `enter` sends and `shift+enter` (or `alt+enter`, `ctrl+j`, or a trailing `\`) adds a line; past eight lines the text scrolls inside the box. `esc` interrupts the model mid-turn. `ctrl+t` shows the model's reasoning, `ctrl+o` expands tool output, `pgup`/`pgdn` and the mouse wheel scroll (`ctrl+end` jumps back to the bottom), `up`/`down` recall earlier prompts. Code blocks in replies have a `copy` button in their header: click it to put the code on the clipboard (through the terminal over SSH), or type `/copy` for the last one. `!<command>` runs a shell command yourself, with the output also going to the model. `/help` lists the slash commands: `/model`, `/effort`, `/compact`, `/copy`, `/cost`, `/clear`, `/session`, `/quit`.

Sessions are saved as JSON under `~/.caveira/sessions/` after every turn, so `caveira -c` picks up where you left off and `caveira --sessions` lists what is there. When a conversation approaches the model's context window the agent summarizes it into a handoff note and continues from that.

## Configuration

caveira needs an API key unless the endpoint is on localhost. It looks, in order of increasing priority, at `~/.caveira/config.json`, then `.env` and `.env.local` files between the repository root and the working directory, then the environment, then flags:

| What | Environment | `config.json` | Flag |
|---|---|---|---|
| API key | `ABLITERATION_API_KEY`, `ABLIT_KEY`, or `CAVEIRA_API_KEY` | `api_key` | `--api-key` |
| Endpoint | `CAVEIRA_BASE_URL` | `base_url` | `--base-url` |
| Model | `CAVEIRA_MODEL` | `model` | `-m`, `--model` |
| Reasoning effort | `CAVEIRA_REASONING_EFFORT` | `reasoning_effort` | `--effort` |
| Ask before commands and edits | `CAVEIRA_CONFIRM=1` | `confirm` | `--confirm` |

The default endpoint is `https://api.abliteration.ai/v1` and the default model is `abliterated-model` (256K context, multimodal). `abliterated-model-large-v2` is the stronger, text-only 1M-context model for harder work; switch in the `/model` picker, with `/model abliterated-model-large-v2`, or by setting `CAVEIRA_MODEL`. Reasoning effort is one of `none minimal low medium high xhigh max`; unset leaves it to the model's default. `/effort <level>` sets it directly. Switches made in a session last for that session.

Against a local model, for working on caveira without spending on the API:

```sh
caveira --dev                        # Ollama on localhost:11434, a small installed model
caveira --dev -m llama3.2:latest     # a specific one
```

`--dev` points caveira at `http://localhost:11434/v1` (or `CAVEIRA_DEV_BASE_URL`), leaves the API key and reasoning effort out, and uses the model from `-m`, `CAVEIRA_DEV_MODEL`, or the first installed of `qwen3:4b`, `qwen3:1.7b`, `qwen2.5:3b`, `qwen2.5:7b`, `llama3.1:8b`, `llama3.2`, `llama3.2:1b`. It says so up front if nothing is listening or the model is not pulled, and the screen carries a DEV badge wherever the model is named.

On Ollama, the model runs as a `caveira/` copy of itself (`caveira/llama3.2:latest`, say), which Ollama makes from the same weights, so it costs no disk. Two things about Ollama make that necessary. It runs models with a 4K context unless its own settings say otherwise, and cuts longer conversations from the front, system prompt first, while caveira's prompt and tools take about 3K; the endpoint caveira talks to ignores a context size in the request. And Llama 3's chat template tells the model to answer every message with a tool call, so llama3.2 ran a command when told hello. The copy gets a 16K window (or the server's, if bigger), temperature 0.3, and for Llama 3 models a template that offers the tools but leaves calling one to the model. If the copy cannot be made, the model runs as it is and the session card warns when its window is that small. The `/model` picker lists what Ollama has installed, offers the models that can call tools as their copies, and marks the ones that cannot. When a model writes a call out as text instead of making it, caveira repairs it and runs it as the call. Small models still make mistakes; Qwen's are fewer than Llama's.

Any other local server still works with `--base-url` and `--model`.

Project instructions are read from `CAVEIRA.md`, `AGENTS.md`, or `CLAUDE.md` (the first found in each directory from the repository root down to the working directory) and appended to the system prompt. `caveira --show-system-prompt` prints the prompt it would use here.

By default caveira does not ask before running commands or changing files: it is meant to be left alone with a task. `--confirm` turns on a prompt for every command and file change, with `a` to stop asking for that tool for the rest of the session.

## Tools

The model gets seven tools, described to it in `core/tools`: `read_file` (numbered lines, paged), `write_file`, `edit_file` (exact-string replacement with a diff back), `bash` (non-interactive, timed out, output bounded), `glob`, `grep` (RE2, skips binaries and build output), and `list_dir`. Write and execute tools are the ones `--confirm` gates.

Answering in words is the default, the way Claude Code behaves: the system prompt (`core/prompt`) keeps tools for work in the project and questions about it, with short examples of both, and each tool's description says what it is not for. `bash` refuses a command that only prints fixed text (`echo hello`), since its output reaches no one; small models reach for it to talk. A reply that is nothing but a tool call written out as text is repaired and run as the call.

## Run from source

```sh
go run .                 # or: go build -o bin/caveira . && ./bin/caveira
go test ./...            # the interface; the agent's tests are in ../core
```

From the repo root, `make` builds and installs `caveira`, with a `cav` link beside it, into `~/.local/bin` (`BINDIR=/usr/local/bin make` to change that), along with the desktop app, and `make update` replaces the installed builds with fresh ones. `make install-tui` does the terminal client alone.

## Layout

- `main.go` — flags, configuration, and the one-shot `-p` mode; `dev.go` — `--dev`, on top of `core/local`.
- `internal/ui` — the Bubble Tea screens: the home screen with its intro (`home.go`), and the session with its transcript (`items.go`), status line, input, footer, approval card (`view.go`), and command palette (`commands.go`); `theme.go` holds the palette for dark, light, and low-colour terminals.
- `internal/art` — the 11×11 pixel skull mascot (`mascot.go`), rendered as half-block cells in colour or in one colour; `go run ./internal/art/preview` prints it.

The agent itself is not in here: it is the `core` module at the repository root, shared with the desktop app, which this module pulls in with a `replace` directive.

- `core/llm` — streaming client for OpenAI-compatible chat completions: SSE parsing, tool-call assembly, retries.
- `core/tools` — the tool set and its registry.
- `core/prompt` — the system prompt: stance, working method, tool guidance, environment, project instructions.
- `core/agent` — the loop: stream a reply, run the tools it asks for, feed results back; approvals, compaction, sessions.
- `core/config` — settings resolution and what is known about each model.
- `core/local` — running on a model on this machine: picking an installed Ollama model and making its `caveira/` copy.
- `designs/` — TUI screens drawn as terminal cell art.
