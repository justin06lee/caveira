"use client";

import { useEffect, useState } from "react";

import { AsciiShader, plasma, ripple, tunnel, type ShaderFn } from "./ascii-shader";

const CYCLE: ShaderFn[] = [plasma, tunnel, ripple];

/**
 * The living background behind the hero: the chrome ascii-shader, drifting
 * slowly through its three presets so the wall of characters never sits still
 * long enough to read as a static image. Kept dim and ember-tinted; the scrim
 * and headline live above it in the page.
 */
export function HeroShader() {
  const [index, setIndex] = useState(0);

  useEffect(() => {
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    const id = setInterval(() => setIndex((i) => (i + 1) % CYCLE.length), 14000);
    return () => clearInterval(id);
  }, []);

  return (
    <AsciiShader
      shader={CYCLE[index]}
      size={12}
      speed={0.75}
      fps={20}
      label="caveira ascii field"
      className="h-full w-full text-ember/45 transition-opacity duration-1000"
    />
  );
}
