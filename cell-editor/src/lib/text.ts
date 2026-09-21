// Terminal text measurement: graphemes, cell widths and word wrapping, close
// enough to what lipgloss (via x/ansi) does that designs line up in Go.

const segmenter = new Intl.Segmenter(undefined, { granularity: "grapheme" });

// East Asian Wide/Fullwidth and emoji-presentation ranges, Unicode 11-ish,
// which is what xterm's unicode11 addon uses.
const WIDE: [number, number][] = [
  [0x1100, 0x115f], [0x231a, 0x231b], [0x2329, 0x232a], [0x23e9, 0x23ec], [0x23f0, 0x23f0],
  [0x23f3, 0x23f3], [0x25fd, 0x25fe], [0x2614, 0x2615], [0x2648, 0x2653], [0x267f, 0x267f],
  [0x2693, 0x2693], [0x26a1, 0x26a1], [0x26aa, 0x26ab], [0x26bd, 0x26be], [0x26c4, 0x26c5],
  [0x26ce, 0x26ce], [0x26d4, 0x26d4], [0x26ea, 0x26ea], [0x26f2, 0x26f3], [0x26f5, 0x26f5],
  [0x26fa, 0x26fa], [0x26fd, 0x26fd], [0x2705, 0x2705], [0x270a, 0x270b], [0x2728, 0x2728],
  [0x274c, 0x274c], [0x274e, 0x274e], [0x2753, 0x2755], [0x2757, 0x2757], [0x2795, 0x2797],
  [0x27b0, 0x27b0], [0x27bf, 0x27bf], [0x2b1b, 0x2b1c], [0x2b50, 0x2b50], [0x2b55, 0x2b55],
  [0x2e80, 0x303e], [0x3041, 0x33ff], [0x3400, 0x4dbf], [0x4e00, 0x9fff], [0xa000, 0xa4cf],
  [0xa960, 0xa97f], [0xac00, 0xd7a3], [0xf900, 0xfaff], [0xfe10, 0xfe19], [0xfe30, 0xfe6f],
  [0xff00, 0xff60], [0xffe0, 0xffe6], [0x16fe0, 0x16fe4], [0x17000, 0x18aff], [0x1b000, 0x1b2ff],
  [0x1f004, 0x1f004], [0x1f0cf, 0x1f0cf], [0x1f18e, 0x1f18e], [0x1f191, 0x1f19a], [0x1f200, 0x1f251],
  [0x1f300, 0x1f64f], [0x1f680, 0x1f6ff], [0x1f7e0, 0x1f7eb], [0x1f90c, 0x1f9ff], [0x1fa70, 0x1faff],
  [0x20000, 0x2fffd], [0x30000, 0x3fffd],
];

function isWide(cp: number): boolean {
  if (cp < 0x1100) return false;
  let lo = 0;
  let hi = WIDE.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    const [a, b] = WIDE[mid]!;
    if (cp < a) hi = mid - 1;
    else if (cp > b) lo = mid + 1;
    else return true;
  }
  return false;
}

// Width of one grapheme cluster: 0, 1 or 2 cells.
export function graphemeWidth(g: string): number {
  const cp = g.codePointAt(0) ?? 0;
  if (cp < 0x20 || (cp >= 0x7f && cp < 0xa0)) return 0;
  if (cp < 0x300) return 1;
  if ((cp >= 0x300 && cp <= 0x36f) || (cp >= 0x200b && cp <= 0x200f) || (cp >= 0xfe00 && cp <= 0xfe0f)) return 0;
  if (isWide(cp)) return 2;
  if (g.includes("️")) return 2; // emoji presentation selector
  return 1;
}

const ASCII = /^[\x20-\x7e]*$/;

export function graphemes(s: string): string[] {
  if (ASCII.test(s)) return s.split("");
  return Array.from(segmenter.segment(s), (x) => x.segment);
}

export function stringWidth(s: string): number {
  if (ASCII.test(s)) return s.length;
  let w = 0;
  for (const g of graphemes(s)) w += graphemeWidth(g);
  return w;
}

// The first grapheme of s, or "" if there is none.
export function firstGrapheme(s: string): string {
  return graphemes(s)[0] ?? "";
}

// Cuts s to at most `width` cells.
export function truncate(s: string, width: number, tail = ""): string {
  if (stringWidth(s) <= width) return s;
  const tw = stringWidth(tail);
  let out = "";
  let w = 0;
  for (const g of graphemes(s)) {
    const gw = graphemeWidth(g);
    if (w + gw > width - tw) break;
    out += g;
    w += gw;
  }
  return out + tail;
}

// Greedy word wrap at `limit` cells. Words longer than the limit are broken,
// and runs of spaces inside a line are kept, like lipgloss.Wrap.
export function wrap(text: string, limit: number): string[] {
  const out: string[] = [];
  if (limit <= 0) return out;
  for (const para of text.split("\n")) {
    const words = para.split(" ");
    let line = "";
    let lineW = 0;
    let started = false;
    const push = () => {
      out.push(line);
      line = "";
      lineW = 0;
      started = false;
    };
    for (const word of words) {
      const ww = stringWidth(word);
      if (!started) {
        started = true;
        placeWord(word, ww);
        continue;
      }
      if (lineW + 1 + ww <= limit) {
        line += " " + word;
        lineW += 1 + ww;
      } else {
        push();
        if (word === "") continue; // don't carry a break's spaces onto the next line
        started = true;
        placeWord(word, ww);
      }
    }
    push();

    function placeWord(word: string, ww: number) {
      if (ww <= limit) {
        line = word;
        lineW = ww;
        return;
      }
      for (const g of graphemes(word)) {
        const gw = graphemeWidth(g);
        if (lineW + gw > limit) {
          out.push(line);
          line = "";
          lineW = 0;
        }
        line += g;
        lineW += gw;
      }
    }
  }
  return out;
}

// Pads a line to `width` cells for the given alignment; remainder goes right,
// matching lipgloss.
export function alignOffset(lineWidth: number, width: number, align: "left" | "center" | "right"): number {
  const short = Math.max(0, width - lineWidth);
  if (align === "right") return short;
  if (align === "center") return Math.floor(short / 2);
  return 0;
}
