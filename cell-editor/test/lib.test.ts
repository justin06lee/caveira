import { describe, expect, test } from "bun:test";
import { frameDiff, parseAnsi, toAnsi, toPlainText } from "../src/lib/ansi";
import { compose } from "../src/lib/compose";
import { toGo } from "../src/lib/golang";
import { newBox, newDoc, newLine, newPaint, newText } from "../src/lib/model";
import { stringWidth, wrap } from "../src/lib/text";
import { starterDoc } from "../src/starter";

const text = (d: ReturnType<typeof newDoc>) => toPlainText(compose(d));

describe("text", () => {
  test("widths", () => {
    expect(stringWidth("abc")).toBe(3);
    expect(stringWidth("─│╭")).toBe(3);
    expect(stringWidth("日本")).toBe(4);
    expect(stringWidth("🔥")).toBe(2);
  });

  test("wrap keeps words whole and breaks long ones", () => {
    expect(wrap("the quick brown fox", 9)).toEqual(["the quick", "brown fox"]);
    expect(wrap("abcdefghij", 4)).toEqual(["abcd", "efgh", "ij"]);
    expect(wrap("a\n\nb", 5)).toEqual(["a", "", "b"]);
  });
});

describe("compose", () => {
  test("a rounded box with a title and centred text", () => {
    const d = newDoc("t", 12, 5);
    const b = newBox(d, 0, 0, 12, 5, "rounded", "");
    b.title = "hi";
    b.text = "yo";
    b.align = "center";
    b.valign = "middle";
    d.elements.push(b);
    expect(text(d)).toBe(["╭─ hi ─────╮", "│          │", "│    yo    │", "│          │", "╰──────────╯", ""].join("\n"));
  });

  test("partial sides drop their corners like lipgloss", () => {
    const d = newDoc("t", 6, 3);
    const b = newBox(d, 0, 0, 6, 3, "normal", "");
    b.sides = [true, false, true, false];
    b.padding = [0, 0, 0, 0];
    d.elements.push(b);
    expect(text(d)).toBe(["──────", "", "──────", ""].join("\n"));
  });

  test("later elements draw over earlier ones; wide glyphs take two cells", () => {
    const d = newDoc("t", 8, 1);
    const l = newLine(d, 0, 0, "heavy", "");
    l.len = 8;
    d.elements.push(l);
    const t = newText(d, 2, 0, { fg: "", bg: "" });
    t.text = "日x";
    d.elements.push(t);
    expect(text(d)).toBe("━━日x━━━\n");
  });
});

describe("ansi", () => {
  test("round-trips through parseAnsi", () => {
    const d = newDoc("t", 10, 2);
    const p = newPaint(d);
    p.cells = { "0,0": { ch: "█", fg: "#D0463B", bg: "" }, "1,0": { ch: "▓", fg: "9", bg: "236", bold: true }, "3,1": { ch: "x", fg: "", bg: "" } };
    d.elements.push(p);
    const parsed = parseAnsi(toAnsi(compose(d)));
    expect(parsed.cells["0,0"]).toEqual({ ch: "█", fg: "#d0463b", bg: "" });
    expect(parsed.cells["1,0"]).toEqual({ ch: "▓", fg: "9", bg: "236", bold: true });
    expect(parsed.cells["3,1"]).toEqual({ ch: "x", fg: "", bg: "" });
  });

  test("frame diffs touch only changed cells", () => {
    const d = newDoc("t", 10, 3);
    const a = compose(d);
    const t = newText(d, 4, 1, { fg: "1", bg: "" });
    t.text = "hi";
    d.elements.push(t);
    const out = frameDiff(a, compose(d));
    expect(out).toContain("\x1b[2;5H");
    expect(out).toContain("hi");
    expect(out).not.toContain("\x1b[1;1H");
  });
});

test("the Go export names the design and composites every element", () => {
  const go = toGo(starterDoc());
  expect(go).toContain("func Scratch() string");
  expect(go).toContain("lipgloss.RoundedBorder()");
  expect(go).toContain("lipgloss.NewCanvas(100, 30)");
  expect(go.match(/lipgloss\.NewLayer\(/g)!.length).toBeGreaterThanOrEqual(6);
});
