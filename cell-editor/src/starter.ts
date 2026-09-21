import { newBox, newDoc, newLine, newPaint, newText, type Doc, type PaintCell } from "./lib/model";

// The design a fresh checkout opens with: a few of each element, in the TUI's
// palette, so there is something to poke at.
export function starterDoc(): Doc {
  const doc = newDoc("scratch", 100, 30);
  const bone = "#D9D2C3";
  const slate = "#6F7B8F";
  const ember = "#D0463B";
  const brass = "#B8863F";

  const welcome = newBox(doc, 3, 2, 50, 11, "rounded", slate);
  welcome.name = "Welcome";
  welcome.title = "caveira · cells";
  welcome.titleInk = { fg: ember, bg: "", bold: true };
  welcome.padding = [1, 2, 1, 2];
  welcome.ink = { fg: bone, bg: "" };
  welcome.text =
    "Terminal cell art for the TUI.\n\nBoxes are lipgloss blocks: drag to move, pull a handle to resize, double-click to edit. P paints, T types, L draws lines.";
  doc.elements.push(welcome);

  const card = newBox(doc, 57, 2, 30, 7, "thick", ember);
  card.name = "Card";
  card.text = "selected card";
  card.align = "center";
  card.valign = "middle";
  card.ink = { fg: bone, bg: "", bold: true };
  doc.elements.push(card);

  const rule = newLine(doc, 3, 14, "dashed", slate);
  rule.name = "Rule";
  rule.len = 84;
  doc.elements.push(rule);

  const ramp = newPaint(doc);
  ramp.name = "Ramp";
  const steps: [string, string][] = [["░", ember], ["▒", ember], ["▓", ember], ["█", ember], ["█", brass], ["▓", brass], ["▒", brass], ["░", brass]];
  steps.forEach(([ch, fg], i) => {
    for (let k = 0; k < 3; k++) ramp.cells[`${3 + i * 3 + k},16`] = { ch, fg, bg: "" } satisfies PaintCell;
  });
  doc.elements.push(ramp);

  const hint = newText(doc, 3, 18, { fg: slate, bg: "" });
  hint.name = "Hint";
  hint.text = "autosaved to tui/designs  ·  `cat tui/designs/scratch.ans` to see it in a real terminal";
  doc.elements.push(hint);

  return doc;
}
