// The logos of who makes a model, as the model picker shows them:
// abliteration.ai's × the open model's maker, Qwen's or Z.ai's (GLM).
// Each is its maker's mark without its tile, in the text colour: black in
// light mode, white in dark.

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

function Qwen({ height }: { height: number }) {
  return (
    <svg className="brand-logo" width={height * 1.1} height={height * 1.1} viewBox="1 1 22 22" aria-label="Qwen">
      <path
        fillRule="evenodd"
        d="M12.604 1.34c.393.69.784 1.382 1.174 2.075a.18.18 0 00.157.091h5.552c.174 0 .322.11.446.327l1.454 2.57c.19.337.24.478.024.837-.26.43-.513.864-.76 1.3l-.367.658c-.106.196-.223.28-.04.512l2.652 4.637c.172.301.111.494-.043.77-.437.785-.882 1.564-1.335 2.34-.159.272-.352.375-.68.37-.777-.016-1.552-.01-2.327.016a.099.099 0 00-.081.05 575.097 575.097 0 01-2.705 4.74c-.169.293-.38.363-.725.364-.997.003-2.002.004-3.017.002a.537.537 0 01-.465-.271l-1.335-2.323a.09.09 0 00-.083-.049H4.982c-.285.03-.553-.001-.805-.092l-1.603-2.77a.543.543 0 01-.002-.54l1.207-2.12a.198.198 0 000-.197 550.951 550.951 0 01-1.875-3.272l-.79-1.395c-.16-.31-.173-.496.095-.965.465-.813.927-1.625 1.387-2.436.132-.234.304-.334.584-.335a338.3 338.3 0 012.589-.001.124.124 0 00.107-.063l2.806-4.895a.488.488 0 01.422-.246c.524-.001 1.053 0 1.583-.006L11.704 1c.341-.003.724.032.9.34zm-3.432.403a.06.06 0 00-.052.03L6.254 6.788a.157.157 0 01-.135.078H3.253c-.056 0-.07.025-.041.074l5.81 10.156c.025.042.013.062-.034.063l-2.795.015a.218.218 0 00-.2.116l-1.32 2.31c-.044.078-.021.118.068.118l5.716.008c.046 0 .08.02.104.061l1.403 2.454c.046.081.092.082.139 0l5.006-8.76.783-1.382a.055.055 0 01.096 0l1.424 2.53a.122.122 0 00.107.062l2.763-.02a.04.04 0 00.035-.02.041.041 0 000-.04l-2.9-5.086a.108.108 0 010-.113l.293-.507 1.12-1.977c.024-.041.012-.062-.035-.062H9.2c-.059 0-.073-.026-.043-.077l1.434-2.505a.107.107 0 000-.114L9.225 1.774a.06.06 0 00-.053-.031zm6.29 8.02c.046 0 .058.02.034.06l-.832 1.465-2.613 4.585a.056.056 0 01-.05.029.058.058 0 01-.05-.029L8.498 9.841c-.02-.034-.01-.052.028-.054l.216-.012 6.722-.012z"
      />
    </svg>
  );
}

const marks: Record<string, ComponentType<{ height: number }>> = { abliteration: Abliteration, zai: Zai, qwen: Qwen };

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
