# caveira cell editor

A small web editor for designing the TUI as terminal cell art. You lay out screens from lipgloss boxes, text, lines and freehand paint on a grid of terminal cells, and the editor saves each design next to the TUI in `tui/designs/`, where both people and the Go code can use it.

The canvas is a real xterm.js terminal: the design is written to it as escape sequences, so box drawing, block glyphs and colours look the way a terminal draws them. Boxes follow lipgloss v2 rules for borders, padding, wrapping and alignment, and the Go export rebuilds a design with lipgloss layers that render the same cells.

## Run

```sh
bun install
bun run dev        # http://localhost:4747
```

It opens the most recent design, or creates `scratch` on a fresh checkout. Set `PORT` to use another port, or `CELLS_DIR` to keep designs somewhere other than `../tui/designs`.

## What's on screen

- **Tools** (left, with their keys): select `V`, box `B`, text `T`, line `L`, paint `P`, erase `E`, fill `F`, pick `I`.
- **Layers** (left, under the tools): the element stack, top first. Drag rows to reorder, double-click to rename, and use the ○ / ◉ buttons to lock or hide.
- **Canvas** (middle): drag its bottom-right corner to change the size in cells.
- **Inspector** (right): properties of the selection. With nothing selected it shows the brush and the canvas size.

## Elements

- **Box**: a lipgloss block. Its size includes the border, matching `Width`/`Height` in lipgloss v2. It has any of the lipgloss border sets, per-side toggles, border and fill colours, padding, and wrapped text aligned on both axes. It can also carry a title drawn over the top border. Double-click a box, or press Enter, to edit its text.
- **Text**: one or more lines, placed at a cell. Click with the text tool and type; Esc finishes.
- **Line**: a horizontal or vertical run of light, heavy, double, dashed, dotted, ascii, block or custom characters.
- **Paint**: freehand cells. Strokes go into the selected paint layer, or a new one. Right-drag erases, ⇧-click draws a straight stroke from the last one, and the fill tool floods a region with the brush.

New boxes, lines and text take their colours from the topmost element of the same kind, so a run of panels stays consistent.

Colours are exactly what `lipgloss.Color` takes: the terminal default, an ANSI index `0`–`255`, or `#rrggbb`. The picker also offers the TUI's own palette (bone, slate, ember, brass) and the colours already in the design. The preview theme under *view* only changes how ANSI 0–15 and the default colours look here; in the real terminal those come from the user's theme.

## Keys

| | |
|---|---|
| `⌘Z` / `⇧⌘Z` | undo / redo |
| `⌘C` `⌘X` `⌘V` | copy, cut, paste elements (pasted at the pointer) |
| `⌘D` | duplicate |
| `⌥`-drag | drag a copy |
| arrows, `⇧`-arrows | nudge by 1 or 5 cells |
| `[` `]`, `{` `}` | send backward / forward, to back / front |
| `⌫` | delete |
| `G` | cell grid |
| `+` `-` | font size |
| `⌘S` | save now (it also autosaves) |

Pasting text that didn't come from the editor imports it as a paint layer. ANSI colours are kept, and unstyled spaces stay transparent, so `figlet` output or an existing `.ans` file drops straight in.

## Files

Every change autosaves three files per design in `tui/designs/`:

- `<name>.cells.json` is the source the editor reads and writes.
- `<name>.ans` is the design as ANSI. `cat` it to see it in a real terminal, with your font and theme.
- `<name>.txt` is plain text, for reading and diffing.

*export* copies or downloads the ANSI, the plain text, the JSON, or Go. The Go is a function returning the design as a string, built from one `lipgloss.Layer` per element composited onto a `lipgloss.NewCanvas` of the design's size. Border titles, which lipgloss has no style for, become their own layers.

## Layout

- `server.ts` serves the app and the `/api/designs` endpoints that read and write `tui/designs`.
- `src/lib/` is the rendering core, with no DOM in it. The server uses it too.
  - `model.ts` holds the document types.
  - `compose.ts` composites elements into a cell grid.
  - `text.ts` does widths and wrapping.
  - `ansi.ts` handles ANSI output, frame diffs and import.
  - `golang.ts` is the Go export.
  - `borders.ts` has the lipgloss border sets.
  - `colors.ts` has the palettes.
- `src/state.ts` is the store, undo history and autosave. `src/tools.ts` turns pointer gestures into edits.
- `src/ui/` holds the React components. `Terminal.tsx` hosts xterm.js and the input overlay.
- `test/` has the unit tests (`bun test`). `bun run build` typechecks and bundles.
