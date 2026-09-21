import { useEffect, useLayoutEffect, useMemo, useRef, useState, type ReactNode } from "react";
import { ANSI_NAMES, BRAND, describeColor, parseColor, THEMES, toCss, xterm256, type Theme } from "../lib/colors";
import { ATTR_KEYS, type Attrs, type AttrKey, type Color, type Doc } from "../lib/model";
import { useEditor } from "../state";

export function Section(props: { title: string; right?: ReactNode; children: ReactNode }) {
  return (
    <section className="section">
      <header className="section-head">
        <span>{props.title}</span>
        {props.right}
      </header>
      <div className="section-body">{props.children}</div>
    </section>
  );
}

export function Row(props: { label: string; children: ReactNode }) {
  return (
    <div className="row">
      <span className="row-label">{props.label}</span>
      <div className="row-value">{props.children}</div>
    </div>
  );
}

// A number input that commits on every valid keystroke and clamps.
export function NumberField(props: { value: number; min?: number; max?: number; label?: string; onChange(v: number): void; width?: number }) {
  const [text, setText] = useState(String(props.value));
  const focused = useRef(false);
  useEffect(() => {
    if (!focused.current) setText(String(props.value));
  }, [props.value]);
  const clamp = (n: number) => Math.min(props.max ?? Infinity, Math.max(props.min ?? -Infinity, n));
  return (
    <label className="num" style={props.width ? { width: props.width } : undefined}>
      {props.label && <span className="num-label">{props.label}</span>}
      <input
        type="text"
        inputMode="numeric"
        value={text}
        onFocus={(e) => {
          focused.current = true;
          e.currentTarget.select();
        }}
        onBlur={() => {
          focused.current = false;
          setText(String(props.value));
        }}
        onChange={(e) => {
          setText(e.target.value);
          const n = Number(e.target.value);
          if (e.target.value.trim() !== "" && Number.isFinite(n)) props.onChange(clamp(Math.round(n)));
        }}
        onKeyDown={(e) => {
          if (e.key === "ArrowUp" || e.key === "ArrowDown") {
            e.preventDefault();
            const n = clamp(props.value + (e.key === "ArrowUp" ? 1 : -1) * (e.shiftKey ? 10 : 1));
            setText(String(n));
            props.onChange(n);
          }
          if (e.key === "Enter" || e.key === "Escape") e.currentTarget.blur();
        }}
      />
    </label>
  );
}

export function Segmented<T extends string>(props: { value: T; options: { value: T; label: ReactNode; title?: string }[]; onChange(v: T): void }) {
  return (
    <div className="seg">
      {props.options.map((o) => (
        <button
          key={o.value}
          type="button"
          title={o.title}
          className={o.value === props.value ? "on" : ""}
          onClick={() => props.onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

const ATTR_LABEL: Record<AttrKey, { label: ReactNode; title: string }> = {
  bold: { label: <b>B</b>, title: "Bold" },
  faint: { label: <span style={{ opacity: 0.5 }}>F</span>, title: "Faint" },
  italic: { label: <i>I</i>, title: "Italic" },
  underline: { label: <u>U</u>, title: "Underline" },
  strike: { label: <s>S</s>, title: "Strikethrough" },
  reverse: { label: <span className="rev">R</span>, title: "Reverse" },
};

export function AttrToggles(props: { value: Attrs; onChange(a: Attrs): void }) {
  return (
    <div className="seg attrs">
      {ATTR_KEYS.map((k) => (
        <button
          key={k}
          type="button"
          title={ATTR_LABEL[k].title}
          className={props.value[k] ? "on" : ""}
          onClick={() => props.onChange({ ...props.value, [k]: !props.value[k] || undefined })}
        >
          {ATTR_LABEL[k].label}
        </button>
      ))}
    </div>
  );
}

export function useTheme(): Theme {
  const { themeId } = useEditor();
  return THEMES.find((t) => t.id === themeId) ?? THEMES[0]!;
}

function docColors(doc: Doc): Color[] {
  const seen = new Set<string>();
  const add = (c: Color | undefined) => c && seen.add(c);
  for (const e of doc.elements) {
    if (e.type === "box") {
      add(e.borderInk.fg);
      add(e.borderInk.bg);
      add(e.ink.fg);
      add(e.ink.bg);
      add(e.titleInk.fg);
    } else if (e.type === "paint") {
      for (const k in e.cells) {
        add(e.cells[k]!.fg);
        add(e.cells[k]!.bg);
      }
    } else {
      add(e.ink.fg);
      add(e.ink.bg);
    }
  }
  return [...seen].slice(0, 32);
}

export function Swatch(props: { color: Color; theme: Theme; fallback?: string }) {
  const css = toCss(props.color, props.theme);
  return <span className={`swatch ${css ? "" : "default"}`} style={{ background: css ?? props.fallback ?? undefined }} />;
}

// Colour picker for anything lipgloss.Color accepts: the terminal default, the
// ANSI palette, or true colour.
export function ColorField(props: { value: Color; onChange(c: Color): void; defaultLabel?: string; fallback?: string }) {
  const theme = useTheme();
  const { doc } = useEditor();
  const [open, setOpen] = useState(false);
  const btn = useRef<HTMLButtonElement>(null);
  const pop = useRef<HTMLDivElement>(null);
  const [pos, setPos] = useState({ left: 0, top: 0 });
  const [hex, setHex] = useState(props.value);
  const used = useMemo(() => (open ? docColors(doc) : []), [open, doc]);

  useEffect(() => setHex(props.value), [props.value]);

  useLayoutEffect(() => {
    if (!open || !btn.current || !pop.current) return;
    const r = btn.current.getBoundingClientRect();
    const ph = pop.current.offsetHeight;
    const pw = pop.current.offsetWidth;
    const top = r.bottom + 4 + ph > window.innerHeight ? Math.max(8, r.top - 4 - ph) : r.bottom + 4;
    setPos({ left: Math.max(8, Math.min(r.left, window.innerWidth - pw - 8)), top });
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => {
      if (!pop.current?.contains(e.target as Node) && !btn.current?.contains(e.target as Node)) setOpen(false);
    };
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("pointerdown", close, true);
    window.addEventListener("keydown", esc, true);
    return () => {
      window.removeEventListener("pointerdown", close, true);
      window.removeEventListener("keydown", esc, true);
    };
  }, [open]);

  const choose = (c: Color) => props.onChange(c);
  const chip = (c: Color, title: string) => (
    <button
      key={title + c}
      type="button"
      className={`chip ${props.value === c ? "on" : ""}`}
      title={title}
      style={{ background: toCss(c, theme) ?? undefined }}
      onClick={() => choose(c)}
    />
  );

  return (
    <>
      <button ref={btn} type="button" className="color-btn" onClick={() => setOpen((o) => !o)}>
        <Swatch color={props.value} theme={theme} fallback={props.fallback} />
        <span className="color-name">{props.value ? describeColor(props.value) : (props.defaultLabel ?? "default")}</span>
      </button>
      {open && (
        <div ref={pop} className="popover color-pop" style={{ left: pos.left, top: pos.top }}>
          <div className="pop-row">
            <button type="button" className={`chip-default ${props.value === "" ? "on" : ""}`} onClick={() => choose("")}>
              {props.defaultLabel ?? "default"}
            </button>
            <div className="pop-hex">
              <input
                value={hex}
                placeholder="#rrggbb or 0–255"
                onChange={(e) => {
                  setHex(e.target.value);
                  const c = parseColor(e.target.value);
                  if (c !== null && e.target.value.trim() !== "") choose(c);
                }}
              />
              <input
                type="color"
                value={toCss(props.value, theme) ?? theme.foreground}
                onChange={(e) => choose(e.target.value.toUpperCase())}
              />
            </div>
          </div>
          <div className="pop-label">caveira</div>
          <div className="chips">{BRAND.map((b) => chip(b.color, b.name))}</div>
          <div className="pop-label">ansi 16 · follows the terminal's theme</div>
          <div className="chips ansi">{ANSI_NAMES.map((n, i) => chip(String(i), `${i} · ${n}`))}</div>
          <div className="pop-label">256</div>
          <div className="cube">
            {Array.from({ length: 216 }, (_, i) => {
              // Lay the cube out as six 6×6 slices side by side.
              const r = Math.floor(i / 36);
              const gg = Math.floor(i / 6) % 6;
              const b = i % 6;
              const n = 16 + r * 36 + gg * 6 + b;
              return (
                <button
                  key={n}
                  type="button"
                  title={String(n)}
                  className={props.value === String(n) ? "on" : ""}
                  style={{ background: xterm256(n), gridColumn: (r % 3) * 6 + b + 1, gridRow: Math.floor(r / 3) * 6 + gg + 1 }}
                  onClick={() => choose(String(n))}
                />
              );
            })}
          </div>
          <div className="grays">
            {Array.from({ length: 24 }, (_, i) => (
              <button
                key={i}
                type="button"
                title={String(232 + i)}
                className={props.value === String(232 + i) ? "on" : ""}
                style={{ background: xterm256(232 + i) }}
                onClick={() => choose(String(232 + i))}
              />
            ))}
          </div>
          {used.length > 0 && (
            <>
              <div className="pop-label">in this design</div>
              <div className="chips">{used.map((c) => chip(c, describeColor(c)))}</div>
            </>
          )}
        </div>
      )}
    </>
  );
}

export function Menu(props: { label: ReactNode; className?: string; children(close: () => void): ReactNode; align?: "left" | "right" }) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!open) return;
    const close = (e: PointerEvent) => !ref.current?.contains(e.target as Node) && setOpen(false);
    const esc = (e: KeyboardEvent) => e.key === "Escape" && setOpen(false);
    window.addEventListener("pointerdown", close, true);
    window.addEventListener("keydown", esc, true);
    return () => {
      window.removeEventListener("pointerdown", close, true);
      window.removeEventListener("keydown", esc, true);
    };
  }, [open]);
  return (
    <div className="menu" ref={ref}>
      <button type="button" className={`${props.className ?? "btn"} ${open ? "on" : ""}`} onClick={() => setOpen((o) => !o)}>
        {props.label}
      </button>
      {open && <div className={`popover menu-pop ${props.align === "right" ? "right" : ""}`}>{props.children(() => setOpen(false))}</div>}
    </div>
  );
}
