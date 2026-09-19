"use client";

import { useEffect, useState } from "react";

import { AsciiShader, plasma, ripple, tunnel, type ShaderFn } from "./ascii-shader";

const CYCLE: ShaderFn[] = [plasma, tunnel, ripple];
const BY_NAME: Record<string, ShaderFn> = { plasma, tunnel, ripple };

export type ShaderVariant = "cycle" | "plasma" | "tunnel" | "ripple";

/**
 * The living background behind a hero: the chrome ascii-shader, drifting
 * slowly through its presets so the wall of characters never sits still long
 * enough to read as a static image.
 *
 * Takes the preset by *name* rather than by function, so a server component
 * can render it — a shader function can't cross the server/client boundary.
 */
export function ShaderField({
  variant = "cycle",
  className = "h-full w-full text-ember/45",
  size = 12,
  speed = 0.75,
  fps = 20,
}: {
  variant?: ShaderVariant;
  className?: string;
  size?: number;
  speed?: number;
  fps?: number;
}) {
  const [index, setIndex] = useState(0);

  useEffect(() => {
    if (variant !== "cycle") return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const id = setInterval(() => setIndex((i) => (i + 1) % CYCLE.length), 14000);
    return () => clearInterval(id);
  }, [variant]);

  const shader = variant === "cycle" ? CYCLE[index] : BY_NAME[variant] ?? plasma;

  return (
    <AsciiShader
      shader={shader}
      size={size}
      speed={speed}
      fps={fps}
      label="caveira ascii field"
      className={`transition-opacity duration-1000 ${className}`}
    />
  );
}

/** The landing page's hero background: the full drifting cycle. */
export function HeroShader() {
  return <ShaderField variant="cycle" />;
}
