import { useState } from "react";
import { graphemes } from "../lib/text";
import { pickChar, useEditor } from "../state";

const chars = (s: string) => graphemes(s.replace(/\s+/g, ""));
const range = (a: number, b: number) => Array.from({ length: b - a }, (_, i) => a + i);

const GROUPS: { id: string; label: string; chars: string[]; note?: string }[] = [
  {
    id: "box",
    label: "box",
    chars: chars(`
      ─│┌┐└┘├┤┬┴┼ ╭╮╰╯╱╲╳
      ━┃┏┓┗┛┣┫┳┻╋ ═║╔╗╚╝╠╣╦╩╬
      ┍┑┕┙┝┥┯┷┿ ┎┒┖┚┠┨┰┸╂ ╒╕╘╛╞╡╤╧╪ ╓╖╙╜╟╢╥╨╫
      ╌╎┄┆┈┊╍╏┅┇┉┋ ╴╵╶╷╸╹╺╻╼╽╾╿`),
  },
  {
    id: "block",
    label: "blocks",
    chars: chars(`
      █▉▊▋▌▍▎▏▐ ▀▔▁▂▃▄▅▆▇ ▕
      ▖▗▘▝▚▞▙▛▜▟ ░▒▓
      🬀🬁🬂🬃🬄🬅🬆🬇🬈🬉🬊🬋🬌🬍🬎🬏🬐🬑🬒🬓🬔🬕🬖🬗🬘🬙🬚🬛🬜🬝🬞🬟🬠🬡🬢🬣🬤🬥🬦🬧🬨🬩🬪🬫🬬🬭🬮🬯🬰🬱🬲🬳🬴🬵🬶🬷🬸🬹🬺🬻`),
    note: "the sextants (🬀…) need a recent font",
  },
  {
    id: "shape",
    label: "shapes",
    chars: chars(`
      ■□▪▫▬▭▮▯▰▱ ●○◌◍◎◉◦•∙· ◐◑◒◓◔◕◖◗
      ◆◇◈◊ ▲△▴▵▶▷▸▹►▻▼▽▾▿◀◁◂◃◄◅
      ◢◣◤◥ ◧◨◩◪◫ ◰◱◲◳ ◴◵◶◷ ⬒⬓⬔⬕`),
  },
  {
    id: "arrow",
    label: "arrows",
    chars: chars(`←↑→↓↔↕↖↗↘↙ ⇐⇑⇒⇓⇔⇕ ⟵⟶⟷ ↩↪↰↱↲↳↴↵ ➜➔➙➛➝➞➟➠➤ ⇠⇡⇢⇣ ⤴⤵ ⭠⭡⭢⭣`),
  },
  {
    id: "symbol",
    label: "symbols",
    chars: chars(`
      …‥⋯⋮ ✓✔✗✘✕✖ ★☆✦✧✶✱✲✳✴✷✸✹ ※⁂
      ⌘⌥⇧⌃⎋⏎⌫⌦⇥ ⏻⏼⏽ ⚠⚡ ♠♣♥♦♪♫ ☰☱☲☳☴☵☶☷
      ¶§†‡°±×÷≈≠≤≥∞∑∏√∂∆λπΩµ ¤¢£¥€ «»‹›“”‘’`),
  },
  {
    id: "spinner",
    label: "braille",
    chars: Array.from({ length: 256 }, (_, i) => String.fromCodePoint(0x2800 + i)).slice(1),
  },
  {
    id: "powerline",
    label: "powerline",
    chars: [...range(0xe0a0, 0xe0a3), ...range(0xe0b0, 0xe0c8), 0xe0cc, 0xe0cd, 0xe0ce, 0xe0cf, 0xe0d0, 0xe0d1, 0xe0d2, 0xe0d4].map((c) => String.fromCodePoint(c)),
    note: "needs a Nerd Font in the terminal and here",
  },
];

export function CharPalette() {
  const { brush, recentChars } = useEditor();
  const [group, setGroup] = useState("box");
  const g = GROUPS.find((x) => x.id === group)!;
  return (
    <div className="palette">
      <div className="palette-recent">
        {recentChars.map((c) => (
          <button key={c} type="button" className={c === brush.ch ? "on" : ""} onClick={() => pickChar(c)}>
            {c}
          </button>
        ))}
      </div>
      <div className="palette-tabs">
        {GROUPS.map((x) => (
          <button key={x.id} type="button" className={x.id === group ? "on" : ""} onClick={() => setGroup(x.id)}>
            {x.label}
          </button>
        ))}
      </div>
      <div className="palette-grid">
        {g.chars.map((c, i) => (
          <button key={c + i} type="button" title={`U+${c.codePointAt(0)!.toString(16).toUpperCase().padStart(4, "0")}`} className={c === brush.ch ? "on" : ""} onClick={() => pickChar(c)}>
            {c}
          </button>
        ))}
      </div>
      {g.note && <div className="palette-note">{g.note}</div>}
    </div>
  );
}
