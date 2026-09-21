import { BORDERS, lineChar } from "./borders";
import { ATTR_KEYS, type Attrs, type BoxElement, type Color, type Doc, type Element, type Ink, type TextElement } from "./model";
import { alignOffset, graphemes, graphemeWidth, stringWidth, truncate, wrap } from "./text";

// A flat cols × rows cell buffer. A wide grapheme occupies its cell and the
// next one, whose ch is "" (a continuation).
export interface Grid {
  cols: number;
  rows: number;
  ch: string[];
  fg: Color[];
  bg: Color[];
  attr: Uint8Array;
  wide: Uint8Array;
  // Index into doc.elements of the element that drew each cell, or -1.
  owner: Int32Array;
}

export const ATTR_BIT: Record<(typeof ATTR_KEYS)[number], number> = {
  bold: 1,
  faint: 2,
  italic: 4,
  underline: 8,
  strike: 16,
  reverse: 32,
};

export function attrMask(a: Attrs): number {
  let m = 0;
  for (const k of ATTR_KEYS) if (a[k]) m |= ATTR_BIT[k];
  return m;
}

export function maskToAttrs(m: number): Attrs {
  const a: Attrs = {};
  for (const k of ATTR_KEYS) if (m & ATTR_BIT[k]) a[k] = true;
  return a;
}

export function newGrid(cols: number, rows: number): Grid {
  const n = cols * rows;
  return {
    cols,
    rows,
    ch: new Array<string>(n).fill(" "),
    fg: new Array<Color>(n).fill(""),
    bg: new Array<Color>(n).fill(""),
    attr: new Uint8Array(n),
    wide: new Uint8Array(n),
    owner: new Int32Array(n).fill(-1),
  };
}

export function cellInk(g: Grid, i: number): Ink {
  return { fg: g.fg[i]!, bg: g.bg[i]!, ...maskToAttrs(g.attr[i]!) };
}

// Breaks up any wide character that cell i is part of, so overwriting half of
// it never leaves the other half dangling.
function unwide(g: Grid, i: number) {
  if (g.ch[i] === "" && i > 0 && g.wide[i - 1]) {
    g.ch[i - 1] = " ";
    g.wide[i - 1] = 0;
  }
  if (g.wide[i]) {
    g.wide[i] = 0;
    if (g.ch[i + 1] === "") g.ch[i + 1] = " ";
  }
}

// Writes one grapheme; returns how many cells it took.
export function put(g: Grid, x: number, y: number, grapheme: string, fg: Color, bg: Color, attr: number, owner: number): number {
  if (y < 0 || y >= g.rows || x < 0 || x >= g.cols) return graphemeWidth(grapheme) === 2 ? 2 : 1;
  let w = graphemeWidth(grapheme);
  if (w === 0) {
    grapheme = " ";
    w = 1;
  }
  if (w === 2 && x + 1 >= g.cols) {
    grapheme = " ";
    w = 1;
  }
  const i = y * g.cols + x;
  unwide(g, i);
  g.ch[i] = grapheme;
  g.fg[i] = fg;
  g.bg[i] = bg;
  g.attr[i] = attr;
  g.owner[i] = owner;
  if (w === 2) {
    unwide(g, i + 1);
    g.wide[i] = 1;
    g.ch[i + 1] = "";
    g.fg[i + 1] = fg;
    g.bg[i + 1] = bg;
    g.attr[i + 1] = attr;
    g.owner[i + 1] = owner;
  }
  return w;
}

function putString(g: Grid, x: number, y: number, s: string, fg: Color, bg: Color, attr: number, owner: number): number {
  let cx = x;
  for (const gr of graphemes(s)) cx += put(g, cx, y, gr, fg, bg, attr, owner);
  return cx - x;
}

function fillRect(g: Grid, x: number, y: number, w: number, h: number, fg: Color, bg: Color, owner: number) {
  for (let yy = y; yy < y + h; yy++) for (let xx = x; xx < x + w; xx++) put(g, xx, yy, " ", fg, bg, 0, owner);
}

// ---- layouts shared by the compositor, the overlay and the Go export -------

export interface BoxLayout {
  hasTop: boolean;
  hasRight: boolean;
  hasBottom: boolean;
  hasLeft: boolean;
  // Content area, inside borders and padding, in canvas cells.
  cx: number;
  cy: number;
  cw: number;
  ch: number;
  lines: string[];
  overflow: boolean;
}

export function boxLayout(e: BoxElement): BoxLayout {
  const bordered = e.border !== "none";
  const [t, r, bo, l] = e.sides;
  const hasTop = bordered && t;
  const hasRight = bordered && r;
  const hasBottom = bordered && bo;
  const hasLeft = bordered && l;
  const [pt, pr, pb, pl] = e.padding;
  const cx = e.x + (hasLeft ? 1 : 0) + pl;
  const cy = e.y + (hasTop ? 1 : 0) + pt;
  const cw = e.w - (hasLeft ? 1 : 0) - (hasRight ? 1 : 0) - pl - pr;
  const ch = e.h - (hasTop ? 1 : 0) - (hasBottom ? 1 : 0) - pt - pb;
  const all = e.text && cw > 0 ? wrap(e.text, cw) : [];
  const lines = ch > 0 ? all.slice(0, ch) : [];
  return { hasTop, hasRight, hasBottom, hasLeft, cx, cy, cw, ch, lines, overflow: all.length > lines.length };
}

export interface TitleLayout {
  x: number;
  text: string;
}

export function titleLayout(e: BoxElement, L = boxLayout(e)): TitleLayout | null {
  if (!e.title || !L.hasTop) return null;
  const innerX = e.x + (L.hasLeft ? 1 : 0);
  const innerW = e.w - (L.hasLeft ? 1 : 0) - (L.hasRight ? 1 : 0);
  const room = innerW - 2; // keep one border cell showing on each side
  if (room < 3) return null;
  const text = truncate(` ${e.title} `, room, "… ");
  const tw = stringWidth(text);
  let x = innerX + 1;
  if (e.titleAlign === "center") x = innerX + Math.floor((innerW - tw) / 2);
  else if (e.titleAlign === "right") x = innerX + innerW - 1 - tw;
  return { x, text };
}

export function titleInk(e: BoxElement): Ink {
  return { ...e.titleInk, fg: e.titleInk.fg || e.borderInk.fg, bg: e.titleInk.bg || e.borderInk.bg };
}

export interface TextLayout {
  width: number;
  lines: { text: string; offset: number; width: number }[];
}

export function textLayout(e: TextElement): TextLayout {
  const raw = e.text.split("\n");
  const widths = raw.map(stringWidth);
  const width = Math.max(0, ...widths);
  return {
    width,
    lines: raw.map((text, i) => ({ text, width: widths[i]!, offset: alignOffset(widths[i]!, width, e.align) })),
  };
}

// ---- compositing ---------------------------------------------------------

function drawBox(g: Grid, e: BoxElement, owner: number) {
  if (e.w <= 0 || e.h <= 0) return;
  const L = boxLayout(e);
  const content = attrMask(e.ink);
  fillRect(g, e.x, e.y, e.w, e.h, "", e.ink.bg, owner);

  if (e.border !== "none") {
    const B = BORDERS[e.border];
    const { fg, bg } = e.borderInk;
    const x0 = e.x;
    const x1 = e.x + e.w - 1;
    const y0 = e.y;
    const y1 = e.y + e.h - 1;
    const rowStart = x0 + (L.hasLeft ? 1 : 0);
    const rowEnd = x1 - (L.hasRight ? 1 : 0);
    const colStart = y0 + (L.hasTop ? 1 : 0);
    const colEnd = y1 - (L.hasBottom ? 1 : 0);
    if (L.hasTop) for (let x = rowStart; x <= rowEnd; x++) put(g, x, y0, B.top, fg, bg, 0, owner);
    if (L.hasBottom) for (let x = rowStart; x <= rowEnd; x++) put(g, x, y1, B.bottom, fg, bg, 0, owner);
    if (L.hasLeft) for (let y = colStart; y <= colEnd; y++) put(g, x0, y, B.left, fg, bg, 0, owner);
    if (L.hasRight) for (let y = colStart; y <= colEnd; y++) put(g, x1, y, B.right, fg, bg, 0, owner);
    if (L.hasTop && L.hasLeft) put(g, x0, y0, B.tl, fg, bg, 0, owner);
    if (L.hasTop && L.hasRight) put(g, x1, y0, B.tr, fg, bg, 0, owner);
    if (L.hasBottom && L.hasLeft) put(g, x0, y1, B.bl, fg, bg, 0, owner);
    if (L.hasBottom && L.hasRight) put(g, x1, y1, B.br, fg, bg, 0, owner);
  }

  if (L.lines.length) {
    let top = 0;
    const spare = L.ch - L.lines.length;
    if (e.valign === "bottom") top = spare;
    else if (e.valign === "middle") top = Math.floor(spare / 2);
    L.lines.forEach((line, i) => {
      const off = alignOffset(stringWidth(line), L.cw, e.align);
      putString(g, L.cx + off, L.cy + top + i, line, e.ink.fg, e.ink.bg, content, owner);
    });
  }

  const T = titleLayout(e, L);
  if (T) {
    const ink = titleInk(e);
    putString(g, T.x, e.y, T.text, ink.fg, ink.bg, attrMask(ink), owner);
  }
}

function drawText(g: Grid, e: TextElement, owner: number) {
  if (!e.text) return;
  const T = textLayout(e);
  const mask = attrMask(e.ink);
  T.lines.forEach((line, i) => {
    const y = e.y + i;
    // Alignment padding is whitespace: it carries the background, not the attributes.
    for (let x = 0; x < T.width; x++) put(g, e.x + x, y, " ", e.ink.fg, e.ink.bg, 0, owner);
    putString(g, e.x + line.offset, y, line.text, e.ink.fg, e.ink.bg, mask, owner);
  });
}

export function drawElement(g: Grid, e: Element, owner: number) {
  switch (e.type) {
    case "box":
      drawBox(g, e, owner);
      break;
    case "text":
      drawText(g, e, owner);
      break;
    case "line": {
      const ch = lineChar(e.kind, e.dir, e.char);
      const mask = attrMask(e.ink);
      for (let i = 0; i < e.len; i++) {
        if (e.dir === "h") i += put(g, e.x + i, e.y, ch, e.ink.fg, e.ink.bg, mask, owner) - 1;
        else put(g, e.x, e.y + i, ch, e.ink.fg, e.ink.bg, mask, owner);
      }
      break;
    }
    case "paint":
      for (const k in e.cells) {
        const c = e.cells[k]!;
        const comma = k.indexOf(",");
        put(g, e.x + Number(k.slice(0, comma)), e.y + Number(k.slice(comma + 1)), c.ch, c.fg, c.bg, attrMask(c), owner);
      }
      break;
  }
}

export function compose(doc: Doc): Grid {
  const g = newGrid(doc.cols, doc.rows);
  doc.elements.forEach((e, i) => {
    if (!e.hidden) drawElement(g, e, i);
  });
  return g;
}
