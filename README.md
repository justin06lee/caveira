<div align="center">

<img src="assets/caveira.svg" alt="caveira" width="640" />

# caveira

**A Claude Code–style coding agent for abliterated models.**<br>
*Your model, your machine, no refusals.*

</div>

---

caveira is an agentic coding assistant in the spirit of Claude Code, built to drive abliterated open-weight models, meaning models whose refusal behavior has been removed. You give it a task in your project; it reads and searches the code, edits files, runs commands, checks its own work, and tells you what it did. It does not second-guess what you asked for. The only pushback it gives is technical.

The terminal client is the first of three planned clients. A desktop app and a website for the cloud side (accounts, hosted sessions) come later.

## Quick start

```sh
make                                   # builds and installs `caveira` into ~/.local/bin
echo 'ABLITERATION_API_KEY=ak_…' > .env.local   # or export it, or put it in ~/.caveira/config.json
cd your/project && caveira
```

caveira talks to [abliteration.ai](https://abliteration.ai) out of the box, through its OpenAI-compatible endpoint. Any other endpoint that speaks the same protocol works too; a local Ollama is one flag away:

```sh
caveira --base-url http://localhost:11434/v1 --model llama3.2:latest
```

Type what you want. `esc` interrupts, `/help` lists the commands, `caveira -c` continues the last session for the directory, and `caveira -p "…"` runs a task without the interface and prints the result. The full reference, including configuration, is in [tui/README.md](tui/README.md).

## How it works

Every turn, caveira sends the conversation and its tool definitions to the model and streams the reply. When the model calls a tool, caveira runs it, appends the result, and asks again, until the model answers in words. The tools are the same seven that make Claude Code useful: read a file with line numbers, write one, replace an exact string (with a diff back), run a shell command, find files by glob, search contents by regex, list a directory.

The system prompt is caveira's own, written for abliterated models: it tells the model to do the task as asked without moralizing or checking in, and then spends most of its words on what makes an agent good at code. Understand before changing, keep edits minimal and in the project's style, read before editing, verify with the build and the tests, fix root causes rather than symptoms, report faithfully, be brief. Instructions from `CAVEIRA.md`, `AGENTS.md`, or `CLAUDE.md` in the project are appended to it.

Conversations are saved after every turn under `~/.caveira/sessions/`, and when one approaches the model's context window the agent summarizes it into a handoff note and carries on.

## Layout

- `tui/` is the terminal client, in Go with Bubble Tea v2. See [tui/README.md](tui/README.md).
- `cell-editor/` is a web editor for designing TUI screens as terminal cell art: lipgloss boxes, text, lines and paint, saved to `tui/designs/` as JSON, ANSI and plain text with a lipgloss Go export. Run it with `cd cell-editor && bun run dev`. See [cell-editor/README.md](cell-editor/README.md).
- `assets/` holds the banner and the app icon: a softly rounded, pure-black skull eye socket on an off-white (`#F8F7F2`) squircle.

`make` builds both the client and the editor and installs `caveira`; `make update` swaps in a fresh build; `make test` runs the Go tests. Set `BINDIR` to install somewhere other than `~/.local/bin`.
