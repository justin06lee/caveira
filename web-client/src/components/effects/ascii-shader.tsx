"use client";

import * as React from "react";

import { cn } from "@/lib/utils";

export type ShaderPoint = {
  /** Column index, 0-based. */
  col: number;
  /** Row index, 0-based. */
  row: number;
  /**
   * Horizontal position, roughly -1..1 across the grid and aspect-corrected
   * using the measured character cell, so `x*x + y*y` describes a real circle
   * rather than the ellipse a naive column/row normalisation would give.
   */
  x: number;
  /** Vertical position, roughly -1..1 down the grid. */
  y: number;
  /** Seconds since mount, already scaled by `speed`. */
  t: number;
  cols: number;
  rows: number;
};

/** Returns luminance in 0..1. Values outside the range are clamped. */
export type ShaderFn = (point: ShaderPoint) => number;

export type AsciiShaderProps = {
  shader: ShaderFn;
  /** Fixed column count. Omit to fill the element's width. */
  cols?: number;
  /** Fixed row count. Omit to fill the element's height. */
  rows?: number;
  /** Luminance ramp, dark to light. */
  chars?: string;
  /** Frame cap. ASCII reads fine well below 60, and the saving is real. */
  fps?: number;
  /** Multiplies the time fed to the shader. */
  speed?: number;
  /** Freeze on the current frame. */
  paused?: boolean;
  /** Font size in px. Drives the auto-fit grid. */
  size?: number;
  /**
   * CSS containment to isolate the per-frame repaint. Faster, but containment
   * creates its own paint context.
   */
  isolate?: boolean;
  /** Accessible name. Without one the canvas is decorative and hidden. */
  label?: string;
  className?: string;
};

const DEFAULT_CHARS = " .:-=+*#%@";

/**
 * ASCII fragment shader — a character grid painted by a per-cell function of
 * position and time. Ported from the chrome registry (chrome.justin06lee.dev).
 *
 * The frame is built as one string and assigned to `textContent` — a single
 * text mutation per frame, no per-cell DOM. The loop is capped at `fps`, an
 * IntersectionObserver stops it while off-screen, and under
 * `prefers-reduced-motion` it paints one frame at t=0 and never loops.
 */
export function AsciiShader({
  shader,
  cols: fixedCols,
  rows: fixedRows,
  chars = DEFAULT_CHARS,
  fps = 24,
  speed = 1,
  paused = false,
  size = 12,
  isolate = true,
  label,
  className,
}: AsciiShaderProps) {
  const hostRef = React.useRef<HTMLDivElement>(null);
  const preRef = React.useRef<HTMLPreElement>(null);
  const [grid, setGrid] = React.useState<{
    cols: number;
    rows: number;
    cell: number;
  } | null>(null);

  const shaderRef = React.useRef(shader);
  shaderRef.current = shader;
  const pausedRef = React.useRef(paused);
  pausedRef.current = paused;

  const lineHeight = 1.15;

  React.useEffect(() => {
    const host = hostRef.current;
    if (!host) return;

    const probe = document.createElement("pre");
    probe.style.cssText =
      "position:absolute;visibility:hidden;white-space:pre;margin:0;padding:0;";
    probe.style.font = `${size}px ${getComputedStyle(host).fontFamily}`;
    probe.style.lineHeight = String(lineHeight);
    probe.textContent = "0".repeat(50);
    host.appendChild(probe);
    const charWidth = probe.getBoundingClientRect().width / 50;
    const charHeight = size * lineHeight;
    host.removeChild(probe);

    if (!charWidth) return;

    const measure = () => {
      const rect = host.getBoundingClientRect();
      const cols = fixedCols ?? Math.max(1, Math.floor(rect.width / charWidth));
      const rows = fixedRows ?? Math.max(1, Math.floor(rect.height / charHeight));
      setGrid((prev) =>
        prev && prev.cols === cols && prev.rows === rows
          ? prev
          : { cols, rows, cell: charWidth / charHeight },
      );
    };

    measure();
    const observer = new ResizeObserver(measure);
    observer.observe(host);
    return () => observer.disconnect();
  }, [size, fixedCols, fixedRows]);

  React.useEffect(() => {
    const pre = preRef.current;
    const host = hostRef.current;
    if (!pre || !host || !grid) return;

    const { cols, rows, cell } = grid;
    const ramp = chars.length > 0 ? chars : DEFAULT_CHARS;
    const top = ramp.length - 1;

    const physWidth = cols * cell;
    const physHeight = rows;
    const half = Math.min(physWidth, physHeight) / 2;
    const spanX = physWidth / 2;
    const spanY = physHeight / 2;

    const paint = (t: number) => {
      const fn = shaderRef.current;
      const out: string[] = [];
      for (let row = 0; row < rows; row++) {
        const y = (row + 0.5 - spanY) / half;
        let line = "";
        for (let col = 0; col < cols; col++) {
          const x = ((col + 0.5) * cell - spanX) / half;
          const value = fn({ col, row, x, y, t, cols, rows });
          const clamped = value > 0 ? (value < 1 ? value : 1) : 0;
          line += ramp[Math.round(clamped * top)];
        }
        out.push(line);
      }
      pre.textContent = out.join("\n");
    };

    const reduced =
      typeof window !== "undefined" &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;

    if (reduced) {
      paint(0);
      return;
    }

    let raf = 0;
    let last = 0;
    let elapsed = 0;
    let visible = true;
    const interval = 1000 / Math.max(1, fps);

    const frame = (now: number) => {
      raf = requestAnimationFrame(frame);
      if (now - last < interval) return;
      const previous = last || now;
      last = now;
      if (pausedRef.current) return;
      elapsed += now - previous;
      paint((elapsed / 1000) * speed);
    };

    const observer = new IntersectionObserver(
      ([entry]) => {
        const nowVisible = entry?.isIntersecting ?? true;
        if (nowVisible === visible) return;
        visible = nowVisible;
        if (visible) {
          last = 0;
          raf = requestAnimationFrame(frame);
        } else {
          cancelAnimationFrame(raf);
        }
      },
      { threshold: 0 },
    );
    observer.observe(host);

    paint(0);
    raf = requestAnimationFrame(frame);

    return () => {
      cancelAnimationFrame(raf);
      observer.disconnect();
    };
  }, [grid, chars, fps, speed]);

  return (
    <div
      ref={hostRef}
      role={label ? "img" : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cn("relative overflow-hidden font-mono", className)}
    >
      <pre
        ref={preRef}
        className={cn("m-0 whitespace-pre", fixedRows == null && "absolute inset-0")}
        style={{
          fontSize: size,
          lineHeight,
          fontVariantLigatures: "none",
          fontFeatureSettings: '"liga" 0, "calt" 0',
          contain: isolate ? "layout paint style" : undefined,
        }}
      />
    </div>
  );
}

/** Interfering sine fields — the classic demoscene plasma. */
export const plasma: ShaderFn = ({ x, y, t }) =>
  (Math.sin(x * 3 + t) +
    Math.sin(y * 4 - t * 0.7) +
    Math.sin((x + y) * 2.5 + t * 1.3) +
    3) /
  6;

/** Concentric rings travelling outward from the centre. */
export const ripple: ShaderFn = ({ x, y, t }) => {
  const d = Math.sqrt(x * x + y * y);
  return (Math.sin(d * 10 - t * 3) + 1) / 2 / (1 + d);
};

/** A rotating spiral tunnel. */
export const tunnel: ShaderFn = ({ x, y, t }) => {
  const d = Math.sqrt(x * x + y * y) || 1e-6;
  const angle = Math.atan2(y, x);
  return (Math.sin(angle * 4 + 1 / d + t * 2) + 1) / 2;
};
