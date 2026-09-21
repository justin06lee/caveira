import type { BorderKind, LineKind } from "./model";

// Mirrors lipgloss v2's borders.go so a box here is the box lipgloss draws.
export interface BorderChars {
  top: string;
  bottom: string;
  left: string;
  right: string;
  tl: string;
  tr: string;
  bl: string;
  br: string;
}

const b = (top: string, bottom: string, left: string, right: string, tl: string, tr: string, bl: string, br: string): BorderChars => ({
  top, bottom, left, right, tl, tr, bl, br,
});

export const BORDERS: Record<Exclude<BorderKind, "none">, BorderChars> = {
  normal: b("─", "─", "│", "│", "┌", "┐", "└", "┘"),
  rounded: b("─", "─", "│", "│", "╭", "╮", "╰", "╯"),
  thick: b("━", "━", "┃", "┃", "┏", "┓", "┗", "┛"),
  double: b("═", "═", "║", "║", "╔", "╗", "╚", "╝"),
  block: b("█", "█", "█", "█", "█", "█", "█", "█"),
  outerHalfBlock: b("▀", "▄", "▌", "▐", "▛", "▜", "▙", "▟"),
  innerHalfBlock: b("▄", "▀", "▐", "▌", "▗", "▖", "▝", "▘"),
  ascii: b("-", "-", "|", "|", "+", "+", "+", "+"),
  markdown: b("-", "-", "|", "|", "|", "|", "|", "|"),
  hidden: b(" ", " ", " ", " ", " ", " ", " ", " "),
};

export const BORDER_OPTIONS: { kind: BorderKind; label: string; go: string }[] = [
  { kind: "rounded", label: "rounded", go: "lipgloss.RoundedBorder()" },
  { kind: "normal", label: "normal", go: "lipgloss.NormalBorder()" },
  { kind: "thick", label: "thick", go: "lipgloss.ThickBorder()" },
  { kind: "double", label: "double", go: "lipgloss.DoubleBorder()" },
  { kind: "block", label: "block", go: "lipgloss.BlockBorder()" },
  { kind: "outerHalfBlock", label: "outer half", go: "lipgloss.OuterHalfBlockBorder()" },
  { kind: "innerHalfBlock", label: "inner half", go: "lipgloss.InnerHalfBlockBorder()" },
  { kind: "ascii", label: "ascii", go: "lipgloss.ASCIIBorder()" },
  { kind: "markdown", label: "markdown", go: "lipgloss.MarkdownBorder()" },
  { kind: "hidden", label: "hidden", go: "lipgloss.HiddenBorder()" },
  { kind: "none", label: "none", go: "" },
];

export const LINE_CHARS: Record<Exclude<LineKind, "custom">, { h: string; v: string; label: string }> = {
  light: { h: "─", v: "│", label: "light" },
  heavy: { h: "━", v: "┃", label: "heavy" },
  double: { h: "═", v: "║", label: "double" },
  dashed: { h: "╌", v: "╎", label: "dashed" },
  dotted: { h: "┈", v: "┊", label: "dotted" },
  ascii: { h: "-", v: "|", label: "ascii" },
  block: { h: "█", v: "█", label: "block" },
};

export function lineChar(kind: LineKind, dir: "h" | "v", custom: string): string {
  if (kind === "custom") return custom || "·";
  return LINE_CHARS[kind][dir];
}
