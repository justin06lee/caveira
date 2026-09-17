<div align="center">

<img src="assets/caveira.svg" alt="caveira" width="640" />

# caveira

**A Claude Code–style coding agent for abliterated models.**<br>
*Your model, your machine, no refusals.*

</div>

---

caveira is an agentic coding assistant in the spirit of Claude Code, built to drive abliterated open-weight models, meaning models whose refusal behavior has been removed. It comes in two clients that share the same idea: you give the agent a task and it reads, edits, and runs code in your project.

## Layout

- `web-client/` is the browser client: Next.js (App Router), TypeScript, and Tailwind CSS, managed with bun. See [web-client/README.md](web-client/README.md).
- `tui/` is the terminal client: Go with Bubble Tea v2. See [tui/README.md](tui/README.md).

Both clients are fresh scaffolds so far. Neither talks to a model yet.

## Quick start

Terminal client, built and installed as `caveira` in `~/.local/bin`:

```sh
make
caveira
```

Web client, served at http://localhost:3000:

```sh
cd web-client
bun install
bun run dev
```
