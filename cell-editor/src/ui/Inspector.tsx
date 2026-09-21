import { useEffect, useRef } from "react";
import { BORDER_OPTIONS, BORDERS, LINE_CHARS } from "../lib/borders";
import { toCss } from "../lib/colors";
import { boxLayout } from "../lib/compose";
import {
  MAX_COLS,
  MAX_ROWS,
  elementBounds,
  type Attrs,
  type BoxElement,
  type Element,
  type HAlign,
  type Ink,
  type LineElement,
  type LineKind,
  type PaintElement,
  type TextElement,
  type VAlign,
} from "../lib/model";
import { firstGrapheme } from "../lib/text";
import * as S from "../state";
import { CharPalette } from "./CharPalette";
import { AttrToggles, ColorField, NumberField, Row, Section, Segmented, useTheme } from "./controls";

const PAINT_TOOLS = new Set<S.Tool>(["pencil", "eraser", "fill", "picker"]);

export function Inspector() {
  const s = S.useEditor();
  const sel = S.selectedElements(s);
  const one = sel.length === 1 ? sel[0]! : null;
  const brush = PAINT_TOOLS.has(s.tool) || one?.type === "paint" || sel.length === 0;
  return (
    <aside className="inspector">
      {one && <ElementPanel key={one.id} el={one} />}
      {sel.length > 1 && <MultiPanel els={sel} />}
      {s.tool === "box" && <BoxToolPanel />}
      {s.tool === "line" && <LineToolPanel />}
      {brush && <BrushPanel />}
      {sel.length === 0 && <DocPanel />}
    </aside>
  );
}

// ---- helpers ---------------------------------------------------------------

function up<T extends Element>(el: T, field: string) {
  return (patch: Partial<T>) => S.updateElement<T>(el.id, patch, { key: `${el.id}:${field}` });
}

function InkFields(props: { ink: Ink; onChange(i: Ink): void; fgLabel?: string; bgLabel?: string; attrs?: boolean; fgDefault?: string; bgDefault?: string }) {
  const { ink, onChange } = props;
  return (
    <>
      <Row label={props.fgLabel ?? "fg"}>
        <ColorField value={ink.fg} onChange={(fg) => onChange({ ...ink, fg })} defaultLabel={props.fgDefault} />
      </Row>
      <Row label={props.bgLabel ?? "bg"}>
        <ColorField value={ink.bg} onChange={(bg) => onChange({ ...ink, bg })} defaultLabel={props.bgDefault} />
      </Row>
      {props.attrs !== false && (
        <Row label="style">
          <AttrToggles value={ink} onChange={(a: Attrs) => onChange({ ...ink, ...a, fg: ink.fg, bg: ink.bg })} />
        </Row>
      )}
    </>
  );
}

const H_OPTS: { value: HAlign; label: string; title: string }[] = [
  { value: "left", label: "⇤", title: "Left" },
  { value: "center", label: "↔", title: "Center" },
  { value: "right", label: "⇥", title: "Right" },
];
const V_OPTS: { value: VAlign; label: string; title: string }[] = [
  { value: "top", label: "⤒", title: "Top" },
  { value: "middle", label: "↕", title: "Middle" },
  { value: "bottom", label: "⤓", title: "Bottom" },
];

// ---- per-element panels ------------------------------------------------------

const TYPE_LABEL: Record<Element["type"], string> = { box: "box", text: "text", line: "line", paint: "paint" };

function ElementPanel({ el }: { el: Element }) {
  const set = up(el, "geom");
  return (
    <>
      <Section
        title={TYPE_LABEL[el.type]}
        right={
          <div className="head-actions">
            <button type="button" title={el.hidden ? "Show" : "Hide"} className={el.hidden ? "on" : ""} onClick={() => S.updateElement(el.id, { hidden: !el.hidden || undefined })}>
              {el.hidden ? "◌" : "◉"}
            </button>
            <button type="button" title={el.locked ? "Unlock" : "Lock"} className={el.locked ? "on" : ""} onClick={() => S.updateElement(el.id, { locked: !el.locked || undefined })}>
              {el.locked ? "⊘" : "○"}
            </button>
          </div>
        }
      >
        <input className="name-input" value={el.name} onChange={(e) => up(el, "name")({ name: e.target.value })} />
        <div className="grid2">
          <NumberField label="x" value={el.x} min={-MAX_COLS} max={MAX_COLS} onChange={(x) => set({ x })} />
          <NumberField label="y" value={el.y} min={-MAX_ROWS} max={MAX_ROWS} onChange={(y) => set({ y })} />
          {el.type === "box" && (
            <>
              <NumberField label="w" value={el.w} min={1} max={MAX_COLS} onChange={(w) => set({ w } as Partial<Element>)} />
              <NumberField label="h" value={el.h} min={1} max={MAX_ROWS} onChange={(h) => set({ h } as Partial<Element>)} />
            </>
          )}
          {el.type === "line" && <NumberField label="len" value={el.len} min={1} max={MAX_COLS} onChange={(len) => set({ len } as Partial<Element>)} />}
        </div>
      </Section>
      {el.type === "box" && <BoxPanel el={el} />}
      {el.type === "text" && <TextPanel el={el} />}
      {el.type === "line" && <LinePanel el={el} />}
      {el.type === "paint" && <PaintPanel el={el} />}
      <ArrangeSection />
    </>
  );
}

function BorderPicker(props: { value: BoxElement["border"]; onChange(k: BoxElement["border"]): void }) {
  return (
    <div className="border-grid">
      {BORDER_OPTIONS.map((o) => {
        const b = o.kind === "none" ? null : BORDERS[o.kind];
        return (
          <button key={o.kind} type="button" title={o.label} className={props.value === o.kind ? "on" : ""} onClick={() => props.onChange(o.kind)}>
            <span className="border-glyph">{b ? `${b.tl}${b.top}${b.tr}\n${b.bl}${b.bottom}${b.br}` : "  \n  "}</span>
            <span className="border-label">{o.label}</span>
          </button>
        );
      })}
    </div>
  );
}

function BoxPanel({ el }: { el: BoxElement }) {
  const text = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    const focus = () => {
      text.current?.focus();
      text.current?.select();
    };
    window.addEventListener("cells:edit-box-text", focus);
    return () => window.removeEventListener("cells:edit-box-text", focus);
  }, []);
  const L = boxLayout(el);
  const side = (i: number, label: string) => (
    <button
      type="button"
      title={["Top", "Right", "Bottom", "Left"][i]}
      className={el.sides[i] ? "on" : ""}
      onClick={() => {
        const sides = [...el.sides] as BoxElement["sides"];
        sides[i] = !sides[i];
        S.updateElement<BoxElement>(el.id, { sides });
      }}
    >
      {label}
    </button>
  );
  return (
    <>
      <Section title="content">
        <textarea
          ref={text}
          className="text-area"
          rows={4}
          placeholder="text inside the box"
          value={el.text}
          onChange={(e) => up(el, "text")({ text: e.target.value })}
        />
        {L.overflow && <div className="warn">text overflows the box by more lines than fit; it is clipped</div>}
        <Row label="align">
          <Segmented value={el.align} options={H_OPTS} onChange={(align) => S.updateElement<BoxElement>(el.id, { align })} />
          <Segmented value={el.valign} options={V_OPTS} onChange={(valign) => S.updateElement<BoxElement>(el.id, { valign })} />
        </Row>
        <InkFields ink={el.ink} onChange={(ink) => up(el, "ink")({ ink })} fgLabel="text" bgLabel="fill" />
        <Row label="padding">
          <div className="pad4">
            {(["t", "r", "b", "l"] as const).map((k, i) => (
              <NumberField
                key={k}
                label={k}
                value={el.padding[i]!}
                min={0}
                max={40}
                onChange={(v) => {
                  const padding = [...el.padding] as BoxElement["padding"];
                  padding[i] = v;
                  up(el, "padding")({ padding });
                }}
              />
            ))}
          </div>
        </Row>
      </Section>
      <Section title="border">
        <BorderPicker value={el.border} onChange={(border) => S.updateElement<BoxElement>(el.id, { border })} />
        {el.border !== "none" && (
          <>
            <Row label="sides">
              <div className="seg">
                {side(0, "top")}
                {side(1, "right")}
                {side(2, "bottom")}
                {side(3, "left")}
              </div>
            </Row>
            <InkFields ink={el.borderInk} onChange={(borderInk) => up(el, "borderInk")({ borderInk })} attrs={false} />
          </>
        )}
      </Section>
      {el.border !== "none" && (
        <Section title="title">
          <input className="text-input" placeholder="drawn over the top border" value={el.title} onChange={(e) => up(el, "title")({ title: e.target.value })} />
          <Row label="align">
            <Segmented value={el.titleAlign} options={H_OPTS} onChange={(titleAlign) => S.updateElement<BoxElement>(el.id, { titleAlign })} />
          </Row>
          <InkFields ink={el.titleInk} onChange={(titleInk) => up(el, "titleInk")({ titleInk })} fgDefault="border" bgDefault="border" />
        </Section>
      )}
    </>
  );
}

function TextPanel({ el }: { el: TextElement }) {
  return (
    <Section title="text">
      <textarea className="text-area" rows={3} value={el.text} onChange={(e) => up(el, "text")({ text: e.target.value })} />
      <Row label="align">
        <Segmented value={el.align} options={H_OPTS} onChange={(align) => S.updateElement<TextElement>(el.id, { align })} />
      </Row>
      <InkFields ink={el.ink} onChange={(ink) => up(el, "ink")({ ink })} />
    </Section>
  );
}

const LINE_KINDS = [...Object.entries(LINE_CHARS).map(([k, v]) => ({ value: k as LineKind, label: v.h, title: v.label })), { value: "custom" as LineKind, label: "…", title: "custom character" }];

function LinePanel({ el }: { el: LineElement }) {
  return (
    <Section title="line">
      <Row label="kind">
        <Segmented value={el.kind} options={LINE_KINDS} onChange={(kind) => S.updateElement<LineElement>(el.id, { kind })} />
      </Row>
      {el.kind === "custom" && (
        <Row label="char">
          <input className="char-input" value={el.char} onChange={(e) => up(el, "char")({ char: firstGrapheme(e.target.value.slice(-2)) || el.char })} />
        </Row>
      )}
      <Row label="direction">
        <Segmented
          value={el.dir}
          options={[
            { value: "h", label: "horizontal" },
            { value: "v", label: "vertical" },
          ]}
          onChange={(dir) => S.updateElement<LineElement>(el.id, { dir })}
        />
      </Row>
      <InkFields ink={el.ink} onChange={(ink) => up(el, "ink")({ ink })} />
    </Section>
  );
}

function PaintPanel({ el }: { el: PaintElement }) {
  const { brush } = S.useEditor();
  const count = Object.keys(el.cells).length;
  const b = elementBounds(el);
  return (
    <Section title="paint layer">
      <div className="muted">
        {count} cells{count ? ` · ${b.w}×${b.h}` : ""} · strokes go into this layer while it's selected
      </div>
      <div className="btn-row">
        <button
          type="button"
          className="btn"
          disabled={!count}
          onClick={() =>
            S.updateElement<PaintElement>(el.id, (e) => {
              const cells: PaintElement["cells"] = {};
              for (const k in e.cells) cells[k] = { ...e.cells[k]!, fg: brush.fg, bg: brush.bg };
              return { cells };
            })
          }
        >
          recolor with brush
        </button>
        <button type="button" className="btn" disabled={!count} onClick={() => S.updateElement<PaintElement>(el.id, { cells: {} })}>
          clear
        </button>
      </div>
    </Section>
  );
}

function ArrangeSection() {
  return (
    <Section title="arrange">
      <div className="btn-row">
        <button type="button" className="btn" title="Bring to front (⇧])" onClick={() => S.restack("top")}>⤒ front</button>
        <button type="button" className="btn" title="Forward (])" onClick={() => S.restack("up")}>↑</button>
        <button type="button" className="btn" title="Backward ([)" onClick={() => S.restack("down")}>↓</button>
        <button type="button" className="btn" title="Send to back (⇧[)" onClick={() => S.restack("bottom")}>⤓ back</button>
      </div>
      <div className="btn-row">
        <button type="button" className="btn" title="Duplicate (⌘D)" onClick={S.duplicateSelection}>duplicate</button>
        <button type="button" className="btn danger" title="Delete (⌫)" onClick={() => S.removeElements(S.get().selected)}>delete</button>
      </div>
    </Section>
  );
}

function MultiPanel({ els }: { els: Element[] }) {
  const align = (mode: "l" | "c" | "r" | "t" | "m" | "b") => {
    const bs = els.map((e) => ({ e, b: elementBounds(e) }));
    const minX = Math.min(...bs.map((x) => x.b.x));
    const maxX = Math.max(...bs.map((x) => x.b.x + x.b.w));
    const minY = Math.min(...bs.map((x) => x.b.y));
    const maxY = Math.max(...bs.map((x) => x.b.y + x.b.h));
    const moves = new Map<string, { dx: number; dy: number }>();
    for (const { e, b } of bs) {
      let dx = 0;
      let dy = 0;
      if (mode === "l") dx = minX - b.x;
      if (mode === "r") dx = maxX - (b.x + b.w);
      if (mode === "c") dx = Math.floor((minX + maxX - b.w) / 2) - b.x;
      if (mode === "t") dy = minY - b.y;
      if (mode === "b") dy = maxY - (b.y + b.h);
      if (mode === "m") dy = Math.floor((minY + maxY - b.h) / 2) - b.y;
      moves.set(e.id, { dx, dy });
    }
    S.edit((d) => ({
      ...d,
      elements: d.elements.map((e) => {
        const m = moves.get(e.id);
        return m && !e.locked ? { ...e, x: e.x + m.dx, y: e.y + m.dy } : e;
      }),
    }));
  };
  return (
    <>
      <Section title={`${els.length} selected`}>
        <Row label="align">
          <div className="seg">
            <button type="button" title="Left edges" onClick={() => align("l")}>⇤</button>
            <button type="button" title="Centers" onClick={() => align("c")}>↔</button>
            <button type="button" title="Right edges" onClick={() => align("r")}>⇥</button>
            <button type="button" title="Top edges" onClick={() => align("t")}>⤒</button>
            <button type="button" title="Middles" onClick={() => align("m")}>↕</button>
            <button type="button" title="Bottom edges" onClick={() => align("b")}>⤓</button>
          </div>
        </Row>
      </Section>
      <ArrangeSection />
    </>
  );
}

// ---- tool panels -------------------------------------------------------------

function BoxToolPanel() {
  const { boxBorder } = S.useEditor();
  return (
    <Section title="new boxes">
      <BorderPicker value={boxBorder} onChange={(boxBorder) => S.set({ boxBorder })} />
      <div className="muted">drag on the canvas, or click for a 30×8 box</div>
    </Section>
  );
}

function LineToolPanel() {
  const { lineKind } = S.useEditor();
  return (
    <Section title="new lines">
      <Segmented value={lineKind} options={LINE_KINDS} onChange={(lineKind) => S.set({ lineKind })} />
      <div className="muted">drag horizontally or vertically</div>
    </Section>
  );
}

function BrushPanel() {
  const { brush, brushSize } = S.useEditor();
  const theme = useTheme();
  const setInk = (ink: Ink) => S.set({ brush: { ...ink, ch: brush.ch } });
  return (
    <Section title="brush">
      <div className="brush-head">
        <div
          className="brush-preview"
          style={{
            color: toCss(brush.fg, theme) ?? theme.foreground,
            background: toCss(brush.bg, theme) ?? theme.background,
            fontWeight: brush.bold ? 700 : 400,
            fontStyle: brush.italic ? "italic" : undefined,
          }}
        >
          {brush.ch}
        </div>
        <div className="brush-meta">
          <input
            className="char-input"
            aria-label="Brush character"
            value={brush.ch}
            onChange={(e) => {
              const g = firstGrapheme([...e.target.value].slice(-1).join("")) || firstGrapheme(e.target.value);
              if (g) S.pickChar(g);
            }}
          />
          <div className="muted">U+{brush.ch.codePointAt(0)?.toString(16).toUpperCase().padStart(4, "0")}</div>
          <Row label="size">
            <Segmented
              value={String(brushSize)}
              options={["1", "2", "3", "5"].map((v) => ({ value: v, label: v }))}
              onChange={(v) => S.set({ brushSize: Number(v) })}
            />
          </Row>
        </div>
      </div>
      <InkFields ink={brush} onChange={setInk} />
      <CharPalette />
    </Section>
  );
}

const SIZES: [number, number][] = [
  [80, 24],
  [100, 30],
  [120, 40],
  [160, 48],
];

function DocPanel() {
  const { doc } = S.useEditor();
  const setSize = (cols: number, rows: number) => S.edit((d) => ({ ...d, cols, rows }), { key: "doc:size" });
  return (
    <Section title="canvas">
      <div className="grid2">
        <NumberField label="cols" value={doc.cols} min={10} max={MAX_COLS} onChange={(c) => setSize(c, doc.rows)} />
        <NumberField label="rows" value={doc.rows} min={4} max={MAX_ROWS} onChange={(r) => setSize(doc.cols, r)} />
      </div>
      <div className="btn-row">
        {SIZES.map(([c, r]) => (
          <button key={`${c}x${r}`} type="button" className={`btn ${doc.cols === c && doc.rows === r ? "on" : ""}`} onClick={() => setSize(c, r)}>
            {c}×{r}
          </button>
        ))}
      </div>
      <div className="muted">
        {doc.elements.length} elements · drag the canvas corner to resize · paste ANSI or text to import it as a paint layer
      </div>
    </Section>
  );
}
