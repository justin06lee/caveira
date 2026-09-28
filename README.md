<div align="center">

<img src="assets/caveira-icon.svg" alt="caveira" width="160" />

# caveira

**A Claude Code–style coding agent for abliterated models.**<br>
*Your model, your machine, no refusals.*

</div>

---

caveira is an agentic coding assistant in the spirit of Claude Code, built to drive abliterated open-weight models, meaning models whose refusal behavior has been removed. You give it a task in your project; it reads and searches the code, edits files, runs commands, checks its own work, and tells you what it did. It does not second-guess what you asked for. The only pushback it gives is technical.

It comes as a terminal client and a desktop app, which share one agent and one set of sessions. A website for the cloud side (accounts, hosted sessions) comes later.

## Quick start

```sh
make                                   # builds and installs `caveira` (and `cav`) into ~/.local/bin
                                       # and caveira.app into /Applications, then opens the app
echo 'ABLITERATION_API_KEY=ak_…' > .env.local   # or export it, or put it in ~/.caveira/config.json
cd your/project && caveira
```

In the app, open a folder and type; the first launch asks for the key there, or offers a model on this machine instead.

caveira talks to [abliteration.ai](https://abliteration.ai) out of the box, through its OpenAI-compatible endpoint. Any other endpoint that speaks the same protocol works too; a local Ollama is one flag away:

```sh
caveira --base-url http://localhost:11434/v1 --model llama3.2:latest
```

Run `caveira`, or `cav` for short. It opens on the pixel skull, dissolving in while everything loads, with the prompt centred under it; the first message slides the prompt to the bottom and the conversation takes over. Type what you want. `esc` interrupts, `/help` lists the commands, `caveira -c` continues the last session for the directory, `caveira -p "…"` runs a task without the interface and prints the result, and `caveira --dev` runs against a model on your machine (Ollama) instead of the API, for working on caveira itself. The full reference, including configuration, is in [tui/README.md](tui/README.md).

The desktop app is the same agent in a quiet window: projects and their chats in a sidebar, the conversation with its tool calls as single lines that open to show output and diffs, approvals as cards in the transcript, and a composer that switches model and effort. It reads the same settings and sessions as the terminal client, so a chat started in one continues in the other. On first launch it offers to bring over your chats and projects from Claude Code, Codex, and OpenCode, and projects open from a type-to-filter folder picker with tab completion. See [desktop/README.md](desktop/README.md).

## How it works

Every turn, caveira sends the conversation and its tool definitions to the model and streams the reply. When the model calls a tool, caveira runs it, appends the result, and asks again, until the model answers in words. The tools are the same seven that make Claude Code useful: read a file with line numbers, write one, replace an exact string (with a diff back), run a shell command, find files by glob, search contents by regex, list a directory.

The system prompt is caveira's own, written for abliterated models: it tells the model to do the task as asked without moralizing or checking in, and then spends most of its words on what makes an agent good at code. Understand before changing, keep edits minimal and in the project's style, read before editing, verify with the build and the tests, fix root causes rather than symptoms, report faithfully, be brief. Instructions from `CAVEIRA.md`, `AGENTS.md`, or `CLAUDE.md` in the project are appended to it.

Conversations are saved after every turn under `~/.caveira/sessions/`, and when one approaches the model's context window the agent summarizes it into a handoff note and carries on.

## Layout

- `core/` is the agent itself, shared by both clients: the OpenAI-compatible client (`llm`), the tools, the system prompt, the turn loop and sessions (`agent`), settings (`config`), local models through Ollama (`local`), and reading other agents' chats into caveira sessions (`importer`).
- `tui/` is the terminal client, in Go with Bubble Tea v2. See [tui/README.md](tui/README.md).
- `desktop/` is the desktop app, in Go with Wails v2 and a React and TypeScript frontend built with bun. See [desktop/README.md](desktop/README.md).
- `assets/` holds the app icon: a softly rounded, pure-black skull eye socket on an off-white (`#F8F7F2`) squircle.

`make` builds both clients, installs `caveira` with a `cav` link beside it and `caveira.app`, and opens the app; `make update` quits the app, swaps in fresh builds, and opens it again; `make test` runs the Go tests of all three modules. Set `BINDIR` to install the terminal client somewhere other than `~/.local/bin`, and `APPDIR` for the app.
