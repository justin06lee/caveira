// The logos of who makes a model, as the model picker shows them:
// abliteration.ai's alone, or abliteration.ai's × Z.ai's for the GLM
// models it serves. Each is its maker's mark without its tile, in the
// text colour: black in light mode, white in dark.

import type { ComponentType } from "react";

// Marks are drawn this tall; each is as wide as its shape.
const HEIGHT = 12;

function Abliteration({ height }: { height: number }) {
  return (
    <svg
      className="brand-logo"
      width={(height * 371.358) / 333.122}
      height={height}
      viewBox="0 0 371.358 333.122"
      aria-label="abliteration.ai"
    >
      <path d="M51.48 0H0V333.122H51.48V0Z" />
      <path d="M371.358 0H316.415L186.498 314.964L179.337 333.122H233.111L363.402 19.3752L371.358 0Z" />
      <path d="M189.4 0H137.78L90.9328 325.822L89.8096 333.122H141.243L186.779 17.2224L189.4 0Z" />
    </svg>
  );
}

function Zai({ height }: { height: number }) {
  return (
    <svg
      className="brand-logo"
      width={(height * 18.6) / 15.82}
      height={height}
      viewBox="5.7 7.09 18.6 15.82"
      aria-label="Z.ai"
    >
      <path d="M15.47,7.1l-1.3,1.85c-0.2,0.29-0.54,0.47-0.9,0.47h-7.1V7.09C6.16,7.1,15.47,7.1,15.47,7.1z" />
      <polygon points="24.3,7.1 13.14,22.91 5.7,22.91 16.86,7.1" />
      <path d="M14.53,22.91l1.31-1.86c0.2-0.29,0.54-0.47,0.9-0.47h7.09v2.33H14.53z" />
    </svg>
  );
}

const marks: Record<string, ComponentType<{ height: number }>> = { abliteration: Abliteration, zai: Zai };

export function ModelLogos({ logos, height = HEIGHT }: { logos?: string[]; height?: number }) {
  const known = (logos ?? []).filter((l) => marks[l]);
  if (!known.length) return null;
  return (
    <span className="model-logos">
      {known.map((l, i) => {
        const M = marks[l];
        return (
          <span key={l} className="model-logos-one">
            {i > 0 && <span className="model-logos-x">×</span>}
            <M height={height} />
          </span>
        );
      })}
    </span>
  );
}

// Mark is one maker's logo alone, for the plans.
export function BrandMark({ id, height = HEIGHT }: { id: string; height?: number }) {
  const M = marks[id];
  return M ? <M height={height} /> : null;
}
