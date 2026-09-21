import { ATTR_BIT, type Grid } from "./compose";
import { isHex, isIndex } from "./colors";
import type { Color, PaintCell } from "./model";
import { graphemes, graphemeWidth } from "./text";

const ESC = "\x1b";

function colorParams(c: Color, base: number, brightBase: number, ext: number): string | null {
  if (!c) return null;
  if (isIndex(c)) {
    const n = Number(c);
    if (n < 8) return String(base + n);
    if (n < 16) return String(brightBase + n - 8);
    return `${ext};5;${n}`;
  }
  if (isHex(c)) {
    const r = parseInt(c.slice(1, 3), 16);
    const g = parseInt(c.slice(3, 5), 16);
    const b = parseInt(c.slice(5, 7), 16);
    return `${ext};2;${r};${g};${b}`;
  }
  return null;
}

// A full SGR that resets and then sets exactly this style.
export function sgr(fg: Color, bg: Color, attr: number): string {
  const p = ["0"];
  if (attr & ATTR_BIT.bold) p.push("1");
  if (attr & ATTR_BIT.faint) p.push("2");
  if (attr & ATTR_BIT.italic) p.push("3");
  if (attr & ATTR_BIT.underline) p.push("4");
  if (attr & ATTR_BIT.reverse) p.push("7");
  if (attr & ATTR_BIT.strike) p.push("9");
  const f = colorParams(fg, 30, 90, 38);
  if (f) p.push(f);
  const b = colorParams(bg, 40, 100, 48);
  if (b) p.push(b);
  return `${ESC}[${p.join(";")}m`;
}

const VISIBLE_BLANK = ATTR_BIT.underline | ATTR_BIT.strike | ATTR_BIT.reverse;

function isBlank(g: Grid, i: number): boolean {
  return g.ch[i] === " " && g.bg[i] === "" && !(g.attr[i]! & VISIBLE_BLANK);
}

function lastInk(g: Grid, y: number): number {
  for (let x = g.cols - 1; x >= 0; x--) if (!isBlank(g, y * g.cols + x)) return x;
  return -1;
}

// The design as an ANSI string you can `cat` in a real terminal.
export function toAnsi(g: Grid): string {
  const rows: string[] = [];
  for (let y = 0; y < g.rows; y++) {
    const end = lastInk(g, y);
    let s = "";
    let cur = "";
    for (let x = 0; x <= end; x++) {
      const i = y * g.cols + x;
      if (g.ch[i] === "") continue;
      const plain = !g.fg[i] && !g.bg[i] && !g.attr[i];
      const want = plain ? "" : sgr(g.fg[i]!, g.bg[i]!, g.attr[i]!);
      if (want !== cur) {
        s += want || `${ESC}[0m`;
        cur = want;
      }
      s += g.ch[i];
    }
    if (cur) s += `${ESC}[0m`;
    rows.push(s);
  }
  while (rows.length && rows[rows.length - 1] === "") rows.pop();
  return rows.join("\n") + "\n";
}

export function toPlainText(g: Grid): string {
  const rows: string[] = [];
  for (let y = 0; y < g.rows; y++) {
    let s = "";
    for (let x = 0; x < g.cols; x++) s += g.ch[y * g.cols + x];
    rows.push(s.trimEnd());
  }
  while (rows.length && rows[rows.length - 1] === "") rows.pop();
  return rows.join("\n") + "\n";
}

function same(a: Grid, b: Grid, i: number): boolean {
  return a.ch[i] === b.ch[i] && a.fg[i] === b.fg[i] && a.bg[i] === b.bg[i] && a.attr[i] === b.attr[i];
}

// Escape sequences that turn a terminal showing `prev` into one showing
// `next`, touching only the cells that changed. With no prev it repaints.
export function frameDiff(prev: Grid | null, next: Grid): string {
  const full = !prev || prev.cols !== next.cols || prev.rows !== next.rows;
  let out = `${ESC}[0m`;
  if (full) out += `${ESC}[2J`;
  let cur = "";
  for (let y = 0; y < next.rows; y++) {
    let x = 0;
    while (x < next.cols) {
      const i = y * next.cols + x;
      if (!full && same(prev!, next, i)) {
        x++;
        continue;
      }
      // Start of a changed run; back up onto the lead of a wide character.
      let sx = x;
      if (next.ch[i] === "" && sx > 0) sx--;
      out += `${ESC}[${y + 1};${sx + 1}H`;
      let cx = sx;
      while (cx < next.cols) {
        const j = y * next.cols + cx;
        if (!full && cx > x && same(prev!, next, j) && next.ch[j] !== "") break;
        if (next.ch[j] === "") {
          cx++;
          continue;
        }
        const want = sgr(next.fg[j]!, next.bg[j]!, next.attr[j]!);
        if (want !== cur) {
          out += want;
          cur = want;
        }
        out += next.ch[j];
        if (next.wide[j]) {
          cx += 2;
          // Re-sync in case the terminal measures this glyph differently.
          out += `${ESC}[${y + 1};${cx + 1}H`;
        } else cx++;
      }
      x = Math.max(cx, x + 1);
    }
  }
  return out + `${ESC}[0m`;
}

// ---- import --------------------------------------------------------------

const TOKEN = /\x1b\[([0-9;:?]*)([@-~])|\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b[()][0-9A-Za-z]|\x1b.|\r?\n|\r|\t|[^\x1b\r\n\t]+/g;

// Parses ANSI art or plain text into paint cells. Unstyled spaces are left
// empty so pasted art keeps its holes.
export function parseAnsi(text: string): { cells: Record<string, PaintCell>; width: number; height: number } {
  const cells: Record<string, PaintCell> = {};
  let x = 0;
  let y = 0;
  let width = 0;
  let fg: Color = "";
  let bg: Color = "";
  let at = { bold: false, faint: false, italic: false, underline: false, strike: false, reverse: false };

  const applySgr = (params: string) => {
    const p = params === "" ? [0] : params.split(/[;:]/).map((n) => Number(n) || 0);
    for (let k = 0; k < p.length; k++) {
      const n = p[k]!;
      if (n === 0) {
        fg = "";
        bg = "";
        at = { bold: false, faint: false, italic: false, underline: false, strike: false, reverse: false };
      } else if (n === 1) at.bold = true;
      else if (n === 2) at.faint = true;
      else if (n === 3) at.italic = true;
      else if (n === 4) at.underline = true;
      else if (n === 7) at.reverse = true;
      else if (n === 9) at.strike = true;
      else if (n === 22) at.bold = at.faint = false;
      else if (n === 23) at.italic = false;
      else if (n === 24) at.underline = false;
      else if (n === 27) at.reverse = false;
      else if (n === 29) at.strike = false;
      else if (n >= 30 && n <= 37) fg = String(n - 30);
      else if (n >= 90 && n <= 97) fg = String(n - 90 + 8);
      else if (n === 39) fg = "";
      else if (n >= 40 && n <= 47) bg = String(n - 40);
      else if (n >= 100 && n <= 107) bg = String(n - 100 + 8);
      else if (n === 49) bg = "";
      else if (n === 38 || n === 48) {
        let c: Color = "";
        if (p[k + 1] === 5) {
          c = String(p[k + 2] ?? 0);
          k += 2;
        } else if (p[k + 1] === 2) {
          const h = (v: number | undefined) => Math.max(0, Math.min(255, v ?? 0)).toString(16).padStart(2, "0");
          c = `#${h(p[k + 2])}${h(p[k + 3])}${h(p[k + 4])}`;
          k += 4;
        }
        if (n === 38) fg = c;
        else bg = c;
      }
    }
  };

  for (const m of text.matchAll(TOKEN)) {
    const tok = m[0];
    if (m[2] !== undefined) {
      if (m[2] === "m") applySgr(m[1]!);
      else if (m[2] === "C") x += Number(m[1]) || 1;
      continue;
    }
    if (tok[0] === "\x1b") continue;
    if (tok === "\n" || tok === "\r\n") {
      y++;
      x = 0;
      continue;
    }
    if (tok === "\r") {
      x = 0;
      continue;
    }
    if (tok === "\t") {
      x = (Math.floor(x / 8) + 1) * 8;
      continue;
    }
    for (const g of graphemes(tok)) {
      const w = graphemeWidth(g);
      if (w === 0) continue;
      const styled = bg !== "" || at.underline || at.reverse || at.strike;
      if (g !== " " || styled) {
        const cell: PaintCell = { ch: g, fg, bg };
        for (const [k, v] of Object.entries(at)) if (v) (cell as unknown as Record<string, boolean>)[k] = true;
        cells[`${x},${y}`] = cell;
        width = Math.max(width, x + w);
      }
      x += w;
    }
  }
  return { cells, width, height: y + 1 };
}
