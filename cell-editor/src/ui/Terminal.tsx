import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebglAddon } from "@xterm/addon-webgl";
import { Terminal } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { frameDiff } from "../lib/ansi";
import type { Theme } from "../lib/colors";
import type { Grid } from "../lib/compose";
import type { Cell, Handle, Mods } from "../tools";

export interface CellSize {
  w: number;
  h: number;
}

interface Props {
  grid: Grid;
  theme: Theme;
  fontSize: number;
  fontFamily: string;
  onDown(c: Cell, m: Mods): void;
  onHandle(h: Handle, c: Cell): void;
  onMove(c: Cell): void;
  onUp(): void;
  onLeave(): void;
  onDouble(c: Cell): void;
  children(size: CellSize): ReactNode;
}

function xtermTheme(t: Theme) {
  const [black, red, green, yellow, blue, magenta, cyan, white, bBlack, bRed, bGreen, bYellow, bBlue, bMagenta, bCyan, bWhite] = t.ansi;
  return {
    background: t.background,
    foreground: t.foreground,
    cursor: t.background,
    cursorAccent: t.background,
    selectionBackground: "transparent",
    black, red, green, yellow, blue, magenta, cyan, white,
    brightBlack: bBlack, brightRed: bRed, brightGreen: bGreen, brightYellow: bYellow,
    brightBlue: bBlue, brightMagenta: bMagenta, brightCyan: bCyan, brightWhite: bWhite,
  };
}

// A real xterm.js terminal used purely as a display: the design is written to
// it as escape sequences, so glyphs, box drawing and colours render the way a
// terminal renders them. A transparent overlay on top takes all the input.
export function TerminalView(props: Props) {
  const { grid, theme, fontSize, fontFamily } = props;
  const host = useRef<HTMLDivElement>(null);
  const overlay = useRef<HTMLDivElement>(null);
  const term = useRef<Terminal | null>(null);
  const shown = useRef<Grid | null>(null);
  const [size, setSize] = useState<CellSize>({ w: 0, h: 0 });
  const cb = useRef(props);
  cb.current = props;

  // A layout effect, so the terminal exists before the frame effect below
  // writes the first frame into it.
  useLayoutEffect(() => {
    const t = new Terminal({
      cols: grid.cols,
      rows: grid.rows,
      fontSize,
      fontFamily,
      lineHeight: 1,
      letterSpacing: 0,
      scrollback: 0,
      disableStdin: true,
      cursorBlink: false,
      cursorStyle: "bar",
      cursorInactiveStyle: "none",
      customGlyphs: true,
      drawBoldTextInBrightColors: false,
      minimumContrastRatio: 1,
      allowProposedApi: true,
      theme: xtermTheme(theme),
    });
    t.open(host.current!);
    const uni = new Unicode11Addon();
    t.loadAddon(uni);
    t.unicode.activeVersion = "11";
    try {
      const gl = new WebglAddon();
      gl.onContextLoss(() => gl.dispose());
      t.loadAddon(gl);
    } catch {
      // The DOM renderer is fine, it just draws box glyphs from the font.
    }
    // No cursor, no autowrap: writing the last column must never scroll.
    t.write("\x1b[?25l\x1b[?7l");
    term.current = t;
    const measure = () => {
      const screen = host.current?.querySelector(".xterm-screen");
      if (!screen || !t.cols) return;
      const r = screen.getBoundingClientRect();
      const next = { w: r.width / t.cols, h: r.height / t.rows };
      setSize((s) => (Math.abs(s.w - next.w) < 0.01 && Math.abs(s.h - next.h) < 0.01 ? s : next));
    };
    const sub = t.onRender(measure);
    const ro = new ResizeObserver(measure);
    ro.observe(host.current!);
    measure();
    return () => {
      sub.dispose();
      ro.disconnect();
      t.dispose();
      term.current = null;
      shown.current = null;
    };
    // The terminal is created once; later prop changes are applied below.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useLayoutEffect(() => {
    const t = term.current;
    if (!t) return;
    if (t.cols !== grid.cols || t.rows !== grid.rows) {
      t.resize(grid.cols, grid.rows);
      shown.current = null;
    }
    t.write(frameDiff(shown.current, grid));
    shown.current = grid;
  }, [grid]);

  useEffect(() => {
    const t = term.current;
    if (!t) return;
    t.options.theme = xtermTheme(theme);
  }, [theme]);

  useEffect(() => {
    const t = term.current;
    if (!t) return;
    t.options.fontSize = fontSize;
    t.options.fontFamily = fontFamily;
  }, [fontSize, fontFamily]);

  const cellAt = (e: { clientX: number; clientY: number }): Cell => {
    const r = overlay.current!.getBoundingClientRect();
    return { x: Math.floor((e.clientX - r.left) / size.w), y: Math.floor((e.clientY - r.top) / size.h) };
  };

  const ready = size.w > 0;

  return (
    <div className="term-host" style={{ background: theme.background }}>
      <div ref={host} className="term-xterm" />
      <div
        ref={overlay}
        className="term-overlay"
        style={{ width: grid.cols * size.w, height: grid.rows * size.h }}
        // Keep focus where the tools put it (the text editor's hidden textarea),
        // and keep the browser from selecting page text on drags.
        onMouseDown={(e) => e.preventDefault()}
        onPointerDown={(e) => {
          if (!ready) return;
          if (e.button !== 0 && e.button !== 2) return;
          const active = document.activeElement;
          if (active instanceof HTMLElement && !e.currentTarget.contains(active)) active.blur();
          try {
            e.currentTarget.setPointerCapture(e.pointerId);
          } catch {
            // Not an active pointer (synthetic events); dragging still works inside the canvas.
          }
          const handle = (e.target as HTMLElement).dataset.handle as Handle | undefined;
          if (handle) cb.current.onHandle(handle, cellAt(e));
          else cb.current.onDown(cellAt(e), { button: e.button, shift: e.shiftKey, alt: e.altKey });
        }}
        onPointerMove={(e) => ready && cb.current.onMove(cellAt(e))}
        onPointerUp={(e) => {
          if (e.currentTarget.hasPointerCapture(e.pointerId)) e.currentTarget.releasePointerCapture(e.pointerId);
          cb.current.onUp();
        }}
        onPointerCancel={() => cb.current.onUp()}
        onPointerLeave={() => cb.current.onLeave()}
        onDoubleClick={(e) => ready && cb.current.onDouble(cellAt(e))}
        onContextMenu={(e) => e.preventDefault()}
      >
        {ready && props.children(size)}
      </div>
    </div>
  );
}
