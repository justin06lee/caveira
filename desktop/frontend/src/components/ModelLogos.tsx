// The logos of who makes a model, as the model picker shows them:
// abliteration.ai's alone, or abliteration.ai's × Z.ai's for the GLM
// models it serves. Both are their makers' own marks, on their own tiles.

import type { ComponentType } from "react";

function Abliteration() {
  return (
    <svg className="brand-logo" viewBox="0 0 512 512" aria-label="abliteration.ai">
      <rect width="512" height="512" rx="104" fill="#000" />
      <g transform="translate(70 89)" fill="#fff">
        <path d="M51.48 0H0V333.122H51.48V0Z" />
        <path d="M371.358 0H316.415L186.498 314.964L179.337 333.122H233.111L363.402 19.3752L371.358 0Z" />
        <path d="M189.4 0H137.78L90.9328 325.822L89.8096 333.122H141.243L186.779 17.2224L189.4 0Z" />
      </g>
    </svg>
  );
}

function Zai() {
  return (
    <svg className="brand-logo" viewBox="1.49 1.49 27.02 27.02" aria-label="Z.ai">
      <path
        fill="#2d2d2d"
        d="M24.51,28.51H5.49c-2.21,0-4-1.79-4-4V5.49c0-2.21,1.79-4,4-4h19.03c2.21,0,4,1.79,4,4v19.03C28.51,26.72,26.72,28.51,24.51,28.51z"
      />
      <g fill="#fff">
        <path d="M15.47,7.1l-1.3,1.85c-0.2,0.29-0.54,0.47-0.9,0.47h-7.1V7.09C6.16,7.1,15.47,7.1,15.47,7.1z" />
        <polygon points="24.3,7.1 13.14,22.91 5.7,22.91 16.86,7.1" />
        <path d="M14.53,22.91l1.31-1.86c0.2-0.29,0.54-0.47,0.9-0.47h7.09v2.33H14.53z" />
      </g>
    </svg>
  );
}

const marks: Record<string, ComponentType> = {
  abliteration: Abliteration,
  zai: Zai,
};

export function ModelLogos({ logos }: { logos?: string[] }) {
  const known = (logos ?? []).filter((l) => marks[l]);
  if (!known.length) return null;
  return (
    <span className="model-logos">
      {known.map((l, i) => {
        const M = marks[l];
        return (
          <span key={l} className="model-logos-one">
            {i > 0 && <span className="model-logos-x">×</span>}
            <M />
          </span>
        );
      })}
    </span>
  );
}
