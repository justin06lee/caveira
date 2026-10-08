// SkullLoader is the terminal client's 11 by 11 pixel skull
// (tui/internal/art/mascot.go) drawn as a matrix of dots, dissolving in
// and out the way the terminal's splash does: each dot comes and goes at
// its own moment, in the same scattered order, and flares as it lands.

const SKULL = [
  "..#######..",
  ".##aaaab##.",
  "##aaaaabc##",
  "#aaaaabbcd#",
  "#aaaabbbcd#",
  "###aab##bd#",
  "###a#b##bd#",
  "#aaaabbbcd#",
  "##aaaab####",
  ".#a#a#b###.",
  ".########..",
];

// hash is mascot.go's: the order the dots arrive in.
function hash(x: number, y: number): number {
  let h = (Math.imul(x, 374761393) + Math.imul(y, 668265263)) >>> 0;
  h = Math.imul(h ^ (h >>> 13), 1274126177) >>> 0;
  h = (h ^ (h >>> 16)) >>> 0;
  return (h & 0xffff) / 0xffff;
}

// The outline and the face's four shades, lit from the left.
const SHADE: Record<string, string> = { "#": "o", a: "f1", b: "f2", c: "f3", d: "f4" };

// SPREAD is how long, in seconds, the dots take to all arrive.
const SPREAD = 0.9;

const DOTS = SKULL.flatMap((row, y) =>
  [...row].flatMap((c, x) =>
    c === "." ? [] : [{ x, y, shade: SHADE[c], delay: hash(x * 7 + 3, y * 13 + 5) * SPREAD }],
  ),
);

export function SkullLoader({ size = 32 }: { size?: number }) {
  return (
    <svg className="skull" width={size} height={size} viewBox="0 0 11 11" aria-hidden="true">
      {DOTS.map((d) => (
        <circle
          key={`${d.x},${d.y}`}
          className={d.shade}
          cx={d.x + 0.5}
          cy={d.y + 0.5}
          r={0.38}
          style={{ animationDelay: `${d.delay.toFixed(3)}s` }}
        />
      ))}
    </svg>
  );
}
