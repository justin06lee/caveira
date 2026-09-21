// Pointer gestures on the canvas. The stage reports cell coordinates; each
// tool turns a down/move/up sequence into document edits, and the whole
// gesture becomes one undo step.
import { maskToAttrs } from "./lib/compose";
import {
  MAX_COLS,
  MAX_ROWS,
  elementBounds,
  newBox,
  newLine,
  newPaint,
  newText,
  type BoxElement,
  type Doc,
  type Element,
  type LineElement,
  type PaintElement,
  type TextElement,
} from "./lib/model";
import { graphemeWidth } from "./lib/text";
import * as S from "./state";

export interface Cell {
  x: number;
  y: number;
}

export interface Mods {
  button: number;
  shift: boolean;
  alt: boolean;
}

export type Handle = "n" | "s" | "e" | "w" | "ne" | "nw" | "se" | "sw" | "start" | "end" | "canvas";

type Gesture =
  | { kind: "move"; start: Cell; base: Doc; origins: Map<string, Cell> }
  | { kind: "marquee"; start: Cell; base: Doc; keep: string[] }
  | { kind: "resize"; start: Cell; base: Doc; id: string; handle: Handle; orig: { x: number; y: number; w: number; h: number } }
  | { kind: "canvas"; start: Cell; base: Doc; cols: number; rows: number }
  | { kind: "create-box"; anchor: Cell; base: Doc; id: string }
  | { kind: "create-line"; anchor: Cell; base: Doc; id: string }
  | { kind: "paint"; base: Doc; id: string; last: Cell; erase: boolean }
  | { kind: "erase"; base: Doc; last: Cell };

let g: Gesture | null = null;
let lastStamp: { id: string; cell: Cell } | null = null;

export function hitTest(c: Cell): Element | null {
  const s = S.get();
  const grid = S.gridOf(s.doc);
  if (c.x < 0 || c.y < 0 || c.x >= grid.cols || c.y >= grid.rows) return null;
  const o = grid.owner[c.y * grid.cols + c.x]!;
  const el = o >= 0 ? s.doc.elements[o] : undefined;
  return el && !el.locked ? el : null;
}

// New elements pick up the look of the topmost element of the same kind, so
// a run of boxes or labels stays consistent without restyling each one.
function lastOfType<T extends Element>(doc: Doc, type: T["type"]): T | undefined {
  for (let i = doc.elements.length - 1; i >= 0; i--) if (doc.elements[i]!.type === type) return doc.elements[i] as T;
  return undefined;
}

function beginBox(c: Cell): BoxElement {
  const s = S.get();
  const el = newBox(s.doc, c.x, c.y, 1, 1, s.boxBorder, "");
  const prev = lastOfType<BoxElement>(s.doc, "box");
  if (prev) {
    el.sides = [...prev.sides];
    el.borderInk = { ...prev.borderInk };
    el.ink = { ...prev.ink };
    el.padding = [...prev.padding];
    el.titleInk = { ...prev.titleInk };
    el.align = prev.align;
    el.valign = prev.valign;
  }
  return el;
}

export function lineCells(a: Cell, b: Cell): Cell[] {
  const out: Cell[] = [];
  let x0 = a.x;
  let y0 = a.y;
  const dx = Math.abs(b.x - x0);
  const dy = -Math.abs(b.y - y0);
  const sx = x0 < b.x ? 1 : -1;
  const sy = y0 < b.y ? 1 : -1;
  let err = dx + dy;
  for (;;) {
    out.push({ x: x0, y: y0 });
    if (x0 === b.x && y0 === b.y) return out;
    const e2 = 2 * err;
    if (e2 >= dy) {
      err += dy;
      x0 += sx;
    }
    if (e2 <= dx) {
      err += dx;
      y0 += sy;
    }
  }
}

// The paint layer strokes go into: the selected one, or a fresh layer on top.
function paintTarget(): string {
  const sel = S.selectedElements();
  const only = sel.length === 1 ? sel[0] : undefined;
  if (only?.type === "paint" && !only.locked && !only.hidden) return only.id;
  const el = newPaint(S.get().doc);
  S.addElement(el, "skip");
  S.set({ selected: [el.id] });
  return el.id;
}

function brushFootprint(c: Cell): Cell[] {
  const { brushSize: n, brush } = S.get();
  if (n <= 1) return [c];
  const step = Math.max(1, graphemeWidth(brush.ch));
  const off = Math.floor((n - 1) / 2);
  const out: Cell[] = [];
  for (let dy = 0; dy < n; dy++) for (let dx = 0; dx < n; dx += step) out.push({ x: c.x - off + dx, y: c.y - off + dy });
  return out;
}

function stamp(id: string, cells: Cell[], erase: boolean) {
  const { brush } = S.get();
  S.updateElement<PaintElement>(
    id,
    (e) => {
      const next = { ...e.cells };
      for (const c of cells) {
        const key = `${c.x - e.x},${c.y - e.y}`;
        if (erase) delete next[key];
        else next[key] = { ...brush };
      }
      return { cells: next };
    },
    "skip",
  );
}

// Erases whatever paint is visible under each cell, from any unlocked layer.
function eraseAt(cells: Cell[]) {
  const s = S.get();
  const grid = S.gridOf(s.doc);
  const byId = new Map<string, Cell[]>();
  for (const c of cells) {
    if (c.x < 0 || c.y < 0 || c.x >= grid.cols || c.y >= grid.rows) continue;
    let i = c.y * grid.cols + c.x;
    if (grid.ch[i] === "" && c.x > 0) i--; // the right half of a wide glyph
    const el = s.doc.elements[grid.owner[i]!];
    if (el?.type !== "paint" || el.locked) continue;
    const cx = i % grid.cols;
    const list = byId.get(el.id) ?? [];
    list.push({ x: cx, y: c.y });
    byId.set(el.id, list);
  }
  for (const [id, list] of byId) stamp(id, list, true);
}

function flood(seed: Cell) {
  const s = S.get();
  const grid = S.gridOf(s.doc);
  const { cols, rows } = grid;
  if (seed.x < 0 || seed.y < 0 || seed.x >= cols || seed.y >= rows) return;
  const key = (i: number) => `${grid.ch[i]}\0${grid.fg[i]}\0${grid.bg[i]}\0${grid.attr[i]}`;
  const target = key(seed.y * cols + seed.x);
  const seen = new Uint8Array(cols * rows);
  const stack = [seed.y * cols + seed.x];
  const cells: Cell[] = [];
  while (stack.length) {
    const i = stack.pop()!;
    if (seen[i] || key(i) !== target) continue;
    seen[i] = 1;
    const x = i % cols;
    const y = (i - x) / cols;
    cells.push({ x, y });
    if (x > 0) stack.push(i - 1);
    if (x < cols - 1) stack.push(i + 1);
    if (y > 0) stack.push(i - cols);
    if (y < rows - 1) stack.push(i + cols);
  }
  const base = s.doc;
  stamp(paintTarget(), cells, false);
  S.commitGesture(base);
}

function pick(c: Cell) {
  const grid = S.gridOf(S.get().doc);
  if (c.x < 0 || c.y < 0 || c.x >= grid.cols || c.y >= grid.rows) return;
  let i = c.y * grid.cols + c.x;
  if (grid.ch[i] === "" && c.x > 0) i--;
  S.pickChar(grid.ch[i] || " ");
  S.set((s) => ({ brush: { ch: s.brush.ch, fg: grid.fg[i]!, bg: grid.bg[i]!, ...maskToAttrs(grid.attr[i]!) }, tool: "pencil" }));
}

// A text editing session is one undo step, from placing the text (or
// opening it) to leaving it. Text left empty is removed.
let textSession: { id: string; base: Doc; created: boolean } | null = null;

export function startEditing(el: TextElement, base = S.get().doc, created = false) {
  textSession = { id: el.id, base, created };
  S.set({ selected: [el.id], editingId: el.id });
}

export function finishEditing() {
  const s = S.get();
  const session = textSession;
  textSession = null;
  const el = s.doc.elements.find((e) => e.id === s.editingId);
  S.set({ editingId: null });
  if (!session) return;
  if (el?.type === "text" && !el.text) {
    S.edit((d) => ({ ...d, elements: d.elements.filter((e) => e.id !== el.id) }), "skip");
    S.set({ selected: [] });
    if (session.created) return;
  }
  S.commitGesture(session.base);
}

export function pointerDown(c: Cell, m: Mods) {
  const s = S.get();
  if (s.editingId) finishEditing();
  const doc = S.get().doc;

  if (m.button === 2 && (s.tool === "pencil" || s.tool === "eraser")) {
    g = { kind: "erase", base: doc, last: c };
    eraseAt(brushFootprint(c));
    S.set({ dragging: true });
    return;
  }

  switch (s.tool) {
    case "select": {
      const hit = hitTest(c);
      if (!hit) {
        const keep = m.shift ? s.selected : [];
        g = { kind: "marquee", start: c, base: doc, keep };
        S.set({ selected: keep, marquee: { x0: c.x, y0: c.y, x1: c.x, y1: c.y } });
        break;
      }
      let sel = s.selected;
      if (m.shift) {
        sel = sel.includes(hit.id) ? sel.filter((id) => id !== hit.id) : [...sel, hit.id];
        S.set({ selected: sel });
        if (!sel.includes(hit.id)) break;
      } else if (!sel.includes(hit.id)) {
        sel = [hit.id];
        S.set({ selected: sel });
      }
      if (m.alt) {
        // Alt-drag leaves the originals and drags copies.
        const copies = S.cloneElements(S.selectedElements(), 0, 0);
        S.edit((d) => ({ ...d, elements: [...d.elements, ...copies] }), "skip");
        sel = copies.map((e) => e.id);
        S.set({ selected: sel });
      }
      const origins = new Map<string, Cell>();
      for (const e of S.get().doc.elements) if (sel.includes(e.id) && !e.locked) origins.set(e.id, { x: e.x, y: e.y });
      g = { kind: "move", start: c, base: doc, origins };
      break;
    }
    case "box": {
      const el = beginBox(c);
      S.addElement(el, "skip");
      S.set({ selected: [el.id] });
      g = { kind: "create-box", anchor: c, base: doc, id: el.id };
      break;
    }
    case "line": {
      const el = newLine(doc, c.x, c.y, s.lineKind, "");
      const prev = lastOfType<LineElement>(doc, "line");
      if (prev) el.ink = { ...prev.ink };
      S.addElement(el, "skip");
      S.set({ selected: [el.id] });
      g = { kind: "create-line", anchor: c, base: doc, id: el.id };
      break;
    }
    case "text": {
      const hit = hitTest(c);
      if (hit?.type === "text") {
        startEditing(hit);
      } else {
        const prev = lastOfType<TextElement>(doc, "text");
        const el = newText(doc, c.x, c.y, prev ? prev.ink : { fg: "", bg: "" });
        S.addElement(el, "skip");
        startEditing(el, doc, true);
      }
      S.set({ tool: "select" });
      break;
    }
    case "pencil": {
      const id = paintTarget();
      // Shift-click draws a straight stroke from the last stamp.
      const from = m.shift && lastStamp?.id === id ? lastStamp.cell : c;
      stamp(id, lineCells(from, c).flatMap(brushFootprint), false);
      lastStamp = { id, cell: c };
      g = { kind: "paint", base: doc, id, last: c, erase: false };
      break;
    }
    case "eraser":
      eraseAt(brushFootprint(c));
      g = { kind: "erase", base: doc, last: c };
      break;
    case "fill":
      flood(c);
      break;
    case "picker":
      pick(c);
      break;
  }
  if (g) S.set({ dragging: true });
}

export function handleDown(handle: Handle, c: Cell) {
  const s = S.get();
  if (handle === "canvas") {
    g = { kind: "canvas", start: c, base: s.doc, cols: s.doc.cols, rows: s.doc.rows };
  } else {
    const el = S.selectedElements()[0];
    if (!el || el.locked || (el.type !== "box" && el.type !== "line")) return;
    g = { kind: "resize", start: c, base: s.doc, id: el.id, handle, orig: elementBounds(el) };
  }
  S.set({ dragging: true });
}

export function pointerMove(c: Cell) {
  const s = S.get();
  if (!s.hover || s.hover.x !== c.x || s.hover.y !== c.y) S.set({ hover: c });
  if (!g) return;
  switch (g.kind) {
    case "move": {
      const dx = c.x - g.start.x;
      const dy = c.y - g.start.y;
      const o = g.origins;
      S.edit((d) => ({ ...d, elements: d.elements.map((e) => (o.has(e.id) ? { ...e, x: o.get(e.id)!.x + dx, y: o.get(e.id)!.y + dy } : e)) }), "skip");
      break;
    }
    case "marquee": {
      const r = { x0: g.start.x, y0: g.start.y, x1: c.x, y1: c.y };
      const minX = Math.min(r.x0, r.x1);
      const maxX = Math.max(r.x0, r.x1);
      const minY = Math.min(r.y0, r.y1);
      const maxY = Math.max(r.y0, r.y1);
      const hits = s.doc.elements
        .filter((e) => {
          if (e.hidden || e.locked) return false;
          const b = elementBounds(e);
          return b.w > 0 && b.x <= maxX && b.x + b.w - 1 >= minX && b.y <= maxY && b.y + b.h - 1 >= minY;
        })
        .map((e) => e.id);
      S.set({ marquee: r, selected: [...new Set([...g.keep, ...hits])] });
      break;
    }
    case "resize": {
      const dx = c.x - g.start.x;
      const dy = c.y - g.start.y;
      const { x, y, w, h } = g.orig;
      const el = s.doc.elements.find((e) => e.id === (g as { id: string }).id);
      if (el?.type === "line") {
        const horiz = el.dir === "h";
        const d = horiz ? dx : dy;
        const len = horiz ? w : h;
        if (g.handle === "start") {
          const shift = Math.min(d, len - 1);
          S.updateElement<LineElement>(el.id, horiz ? { x: x + shift, len: len - shift } : { y: y + shift, len: len - shift }, "skip");
        } else S.updateElement<LineElement>(el.id, { len: Math.max(1, len + d) }, "skip");
        break;
      }
      let x0 = x;
      let y0 = y;
      let x1 = x + w - 1;
      let y1 = y + h - 1;
      if (g.handle.includes("w")) x0 = Math.min(x0 + dx, x1);
      if (g.handle.includes("e")) x1 = Math.max(x1 + dx, x0);
      if (g.handle.includes("n")) y0 = Math.min(y0 + dy, y1);
      if (g.handle.includes("s")) y1 = Math.max(y1 + dy, y0);
      S.updateElement<BoxElement>(g.id, { x: x0, y: y0, w: x1 - x0 + 1, h: y1 - y0 + 1 }, "skip");
      break;
    }
    case "canvas": {
      const cols = Math.min(MAX_COLS, Math.max(10, g.cols + c.x - g.start.x));
      const rows = Math.min(MAX_ROWS, Math.max(4, g.rows + c.y - g.start.y));
      if (cols !== s.doc.cols || rows !== s.doc.rows) S.edit((d) => ({ ...d, cols, rows }), "skip");
      break;
    }
    case "create-box": {
      const a = g.anchor;
      S.updateElement<BoxElement>(g.id, { x: Math.min(a.x, c.x), y: Math.min(a.y, c.y), w: Math.abs(c.x - a.x) + 1, h: Math.abs(c.y - a.y) + 1 }, "skip");
      break;
    }
    case "create-line": {
      const a = g.anchor;
      const dx = c.x - a.x;
      const dy = c.y - a.y;
      const horiz = Math.abs(dx) >= Math.abs(dy);
      S.updateElement<LineElement>(
        g.id,
        horiz
          ? { dir: "h", x: Math.min(a.x, c.x), y: a.y, len: Math.abs(dx) + 1 }
          : { dir: "v", x: a.x, y: Math.min(a.y, c.y), len: Math.abs(dy) + 1 },
        "skip",
      );
      break;
    }
    case "paint":
      if (c.x === g.last.x && c.y === g.last.y) break;
      stamp(g.id, lineCells(g.last, c).flatMap(brushFootprint), false);
      g.last = c;
      lastStamp = { id: g.id, cell: c };
      break;
    case "erase":
      if (c.x === g.last.x && c.y === g.last.y) break;
      eraseAt(lineCells(g.last, c).flatMap(brushFootprint));
      g.last = c;
      break;
  }
}

export function pointerUp() {
  if (!g) return;
  const done = g;
  g = null;
  if (done.kind === "create-box") {
    const el = S.get().doc.elements.find((e) => e.id === done.id) as BoxElement | undefined;
    // A click without a drag drops a default-sized box.
    if (el && el.w < 3 && el.h < 3) S.updateElement<BoxElement>(el.id, { w: 30, h: 8 }, "skip");
    S.set({ tool: "select" });
  }
  if (done.kind === "create-line") {
    const el = S.get().doc.elements.find((e) => e.id === done.id) as LineElement | undefined;
    if (el && el.len < 2) S.updateElement<LineElement>(el.id, { len: 20 }, "skip");
    S.set({ tool: "select" });
  }
  S.commitGesture(done.base);
  S.set({ dragging: false, marquee: null });
}

export function doubleClick(c: Cell) {
  if (S.get().tool !== "select") return;
  const hit = hitTest(c);
  if (hit?.type === "text") startEditing(hit);
  else if (hit?.type === "box") {
    S.set({ selected: [hit.id] });
    window.dispatchEvent(new CustomEvent("cells:edit-box-text"));
  }
}

export function pointerLeave() {
  if (!g) S.set({ hover: null });
}
