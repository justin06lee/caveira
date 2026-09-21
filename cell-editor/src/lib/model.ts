import { stringWidth } from "./text";

// The design document. Everything the editor draws is a list of elements
// composited bottom to top onto a cols × rows grid of terminal cells, the same
// way lipgloss composites layers onto a canvas.

// A colour is exactly what lipgloss.Color() accepts: "" for the terminal's
// default, "0"–"255" for the ANSI palette, or "#rrggbb" for true colour.
export type Color = string;

export interface Attrs {
  bold?: boolean;
  faint?: boolean;
  italic?: boolean;
  underline?: boolean;
  strike?: boolean;
  reverse?: boolean;
}

export interface Ink extends Attrs {
  fg: Color;
  bg: Color;
}

export const ATTR_KEYS = ["bold", "faint", "italic", "underline", "strike", "reverse"] as const;
export type AttrKey = (typeof ATTR_KEYS)[number];

export type BorderKind =
  | "normal"
  | "rounded"
  | "thick"
  | "double"
  | "block"
  | "outerHalfBlock"
  | "innerHalfBlock"
  | "ascii"
  | "markdown"
  | "hidden"
  | "none";

export type HAlign = "left" | "center" | "right";
export type VAlign = "top" | "middle" | "bottom";

// [top, right, bottom, left], the order lipgloss takes them in.
export type Sides = [boolean, boolean, boolean, boolean];
export type Padding = [number, number, number, number];

interface Base {
  id: string;
  name: string;
  x: number;
  y: number;
  hidden?: boolean;
  locked?: boolean;
}

// A lipgloss block: border, background, padding and aligned, wrapped text.
// w and h are the outer size, borders included, which is what Width() and
// Height() mean in lipgloss v2.
export interface BoxElement extends Base {
  type: "box";
  w: number;
  h: number;
  border: BorderKind;
  sides: Sides;
  borderInk: Ink;
  // Content ink: fg and attributes style the text, bg fills padding and content.
  ink: Ink;
  padding: Padding;
  text: string;
  align: HAlign;
  valign: VAlign;
  // Drawn over the top border. Empty title colours inherit the border's.
  title: string;
  titleAlign: HAlign;
  titleInk: Ink;
}

export interface TextElement extends Base {
  type: "text";
  text: string;
  ink: Ink;
  align: HAlign;
}

export type LineKind = "light" | "heavy" | "double" | "dashed" | "dotted" | "ascii" | "block" | "custom";

export interface LineElement extends Base {
  type: "line";
  dir: "h" | "v";
  len: number;
  kind: LineKind;
  char: string; // used when kind is "custom"
  ink: Ink;
}

export interface PaintCell extends Ink {
  ch: string;
}

// Freehand cells. Keys are "cx,cy" relative to the element's x and y.
export interface PaintElement extends Base {
  type: "paint";
  cells: Record<string, PaintCell>;
}

export type Element = BoxElement | TextElement | LineElement | PaintElement;
export type ElementType = Element["type"];

export interface Doc {
  version: 1;
  name: string;
  cols: number;
  rows: number;
  elements: Element[];
}

export const MAX_COLS = 400;
export const MAX_ROWS = 150;

export function uid(): string {
  return Math.random().toString(36).slice(2, 10);
}

export const blankInk = (): Ink => ({ fg: "", bg: "" });

export function newDoc(name: string, cols = 100, rows = 30): Doc {
  return { version: 1, name, cols, rows, elements: [] };
}

export function nextName(doc: Doc, base: string): string {
  const taken = new Set(doc.elements.map((e) => e.name));
  for (let i = 1; ; i++) {
    const n = `${base} ${i}`;
    if (!taken.has(n)) return n;
  }
}

export function newBox(doc: Doc, x: number, y: number, w: number, h: number, border: BorderKind, borderFg: Color): BoxElement {
  return {
    id: uid(),
    type: "box",
    name: nextName(doc, "Box"),
    x,
    y,
    w,
    h,
    border,
    sides: [true, true, true, true],
    borderInk: { fg: borderFg, bg: "" },
    ink: blankInk(),
    padding: [0, 1, 0, 1],
    text: "",
    align: "left",
    valign: "top",
    title: "",
    titleAlign: "left",
    titleInk: blankInk(),
  };
}

export function newText(doc: Doc, x: number, y: number, ink: Ink): TextElement {
  return { id: uid(), type: "text", name: nextName(doc, "Text"), x, y, text: "", ink: { ...ink }, align: "left" };
}

export function newLine(doc: Doc, x: number, y: number, kind: LineKind, fg: Color): LineElement {
  return { id: uid(), type: "line", name: nextName(doc, "Line"), x, y, dir: "h", len: 1, kind, char: "·", ink: { fg, bg: "" } };
}

export function newPaint(doc: Doc): PaintElement {
  return { id: uid(), type: "paint", name: nextName(doc, "Paint"), x: 0, y: 0, cells: {} };
}

// Normalises a design read from disk so older or hand-edited files still load.
export function sanitizeDoc(raw: unknown, fallbackName: string): Doc {
  const d = (raw ?? {}) as Partial<Doc>;
  const cols = clampInt(d.cols, 1, MAX_COLS, 100);
  const rows = clampInt(d.rows, 1, MAX_ROWS, 30);
  const elements = Array.isArray(d.elements) ? (d.elements as Element[]).filter((e) => e && typeof e === "object" && "type" in e) : [];
  for (const e of elements) {
    if (!e.id) e.id = uid();
    if (!e.name) e.name = e.type;
  }
  return { version: 1, name: typeof d.name === "string" && d.name ? d.name : fallbackName, cols, rows, elements };
}

function clampInt(v: unknown, lo: number, hi: number, dflt: number): number {
  const n = typeof v === "number" && Number.isFinite(v) ? Math.round(v) : dflt;
  return Math.min(hi, Math.max(lo, n));
}

export function inkEquals(a: Ink, b: Ink): boolean {
  if (a.fg !== b.fg || a.bg !== b.bg) return false;
  for (const k of ATTR_KEYS) if (!!a[k] !== !!b[k]) return false;
  return true;
}

export function inkKey(i: Ink): string {
  let s = `${i.fg}|${i.bg}|`;
  for (const k of ATTR_KEYS) s += i[k] ? "1" : "0";
  return s;
}

export function elementBounds(e: Element): { x: number; y: number; w: number; h: number } {
  switch (e.type) {
    case "box":
      return { x: e.x, y: e.y, w: e.w, h: e.h };
    case "line":
      return e.dir === "h" ? { x: e.x, y: e.y, w: e.len, h: 1 } : { x: e.x, y: e.y, w: 1, h: e.len };
    case "text": {
      const lines = e.text.split("\n");
      let w = 0;
      for (const l of lines) w = Math.max(w, stringWidth(l));
      return { x: e.x, y: e.y, w: Math.max(w, 1), h: lines.length };
    }
    case "paint": {
      let x0 = Infinity, y0 = Infinity, x1 = -Infinity, y1 = -Infinity;
      for (const k in e.cells) {
        const [cx, cy] = k.split(",").map(Number);
        x0 = Math.min(x0, cx!);
        y0 = Math.min(y0, cy!);
        x1 = Math.max(x1, cx!);
        y1 = Math.max(y1, cy!);
      }
      if (x0 === Infinity) return { x: e.x, y: e.y, w: 0, h: 0 };
      return { x: e.x + x0, y: e.y + y0, w: x1 - x0 + 1, h: y1 - y0 + 1 };
    }
  }
}
