import type { Color } from "./model";

// Preview themes. They only change how the design looks here: ANSI colours
// 0–15 and the default fg/bg belong to whoever's terminal runs the TUI.
export interface Theme {
  id: string;
  label: string;
  background: string;
  foreground: string;
  ansi: string[]; // 16 entries
}

export const THEMES: Theme[] = [
  {
    id: "caveira",
    label: "caveira",
    background: "#0a0a0c",
    foreground: "#d9d2c3",
    ansi: [
      "#16161a", "#d0463b", "#8fa35a", "#b8863f", "#5f7fa8", "#9a6fa0", "#5e9a98", "#c9c2b3",
      "#6f7b8f", "#e5594e", "#a8bd6e", "#d6a55a", "#7e9cc4", "#b78cbd", "#7db7b4", "#f0ebdf",
    ],
  },
  {
    id: "xterm",
    label: "xterm",
    background: "#000000",
    foreground: "#e5e5e5",
    ansi: [
      "#000000", "#cd0000", "#00cd00", "#cdcd00", "#0000ee", "#cd00cd", "#00cdcd", "#e5e5e5",
      "#7f7f7f", "#ff0000", "#00ff00", "#ffff00", "#5c5cff", "#ff00ff", "#00ffff", "#ffffff",
    ],
  },
  {
    id: "paper",
    label: "paper (light)",
    background: "#f8f7f2",
    foreground: "#1d1d21",
    ansi: [
      "#1d1d21", "#b3261e", "#3f7d20", "#8a6100", "#1f5fa8", "#8e3a8e", "#1f7a7a", "#d6d3c8",
      "#6f7b8f", "#d0463b", "#5a9a33", "#b8863f", "#3a7bd5", "#a855a8", "#2f9e9e", "#ffffff",
    ],
  },
];

// The TUI's own palette (tui/internal/ui/styles.go).
export const BRAND: { name: string; color: Color }[] = [
  { name: "bone", color: "#D9D2C3" },
  { name: "slate", color: "#6F7B8F" },
  { name: "ember", color: "#D0463B" },
  { name: "brass", color: "#B8863F" },
];

export const ANSI_NAMES = [
  "black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
  "bright black", "bright red", "bright green", "bright yellow", "bright blue", "bright magenta", "bright cyan", "bright white",
];

const CUBE = [0, 95, 135, 175, 215, 255];

const hex2 = (n: number) => n.toString(16).padStart(2, "0");

// Hex for palette entries 16–255, which are fixed across terminals.
export function xterm256(n: number): string {
  if (n >= 232) {
    const v = 8 + (n - 232) * 10;
    return `#${hex2(v)}${hex2(v)}${hex2(v)}`;
  }
  const i = n - 16;
  return `#${hex2(CUBE[Math.floor(i / 36)]!)}${hex2(CUBE[Math.floor(i / 6) % 6]!)}${hex2(CUBE[i % 6]!)}`;
}

export function isHex(c: Color): boolean {
  return /^#[0-9a-fA-F]{6}$/.test(c);
}

export function isIndex(c: Color): boolean {
  return /^\d{1,3}$/.test(c) && Number(c) <= 255;
}

// CSS colour for a design colour under a theme; null means "terminal default".
export function toCss(c: Color, theme: Theme): string | null {
  if (!c) return null;
  if (isHex(c)) return c;
  if (isIndex(c)) {
    const n = Number(c);
    return n < 16 ? theme.ansi[n]! : xterm256(n);
  }
  return null;
}

// Accepts what people type: "#abc", "abc123", "9", "ansi 9".
export function parseColor(input: string): Color | null {
  const s = input.trim().toLowerCase().replace(/^ansi\s*/, "");
  if (s === "" || s === "default" || s === "none") return "";
  if (/^\d{1,3}$/.test(s) && Number(s) <= 255) return String(Number(s));
  const h = s.startsWith("#") ? s.slice(1) : s;
  if (/^[0-9a-f]{6}$/.test(h)) return `#${h}`;
  if (/^[0-9a-f]{3}$/.test(h)) return `#${h[0]}${h[0]}${h[1]}${h[1]}${h[2]}${h[2]}`;
  return null;
}

export function describeColor(c: Color): string {
  if (!c) return "default";
  if (isIndex(c)) {
    const n = Number(c);
    return n < 16 ? `${n} · ${ANSI_NAMES[n]}` : `${n}`;
  }
  const brand = BRAND.find((b) => b.color.toLowerCase() === c.toLowerCase());
  return brand ? `${c} · ${brand.name}` : c;
}
