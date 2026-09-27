import type { Theme } from "./types";

// applyTheme resolves "system" to light or dark and keeps following the
// system while it is chosen. The choice is also kept in localStorage so
// index.html can paint the first frame in it.
const media = matchMedia("(prefers-color-scheme: dark)");
let current: Theme = "system";

function paint() {
  const dark = current === "dark" || (current === "system" && media.matches);
  document.documentElement.dataset.theme = dark ? "dark" : "light";
}

media.addEventListener("change", paint);

export function applyTheme(t: Theme) {
  current = t;
  localStorage.setItem("caveira.theme", t);
  paint();
}
