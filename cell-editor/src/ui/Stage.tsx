import { useEffect, useLayoutEffect, useRef, useState, type CSSProperties } from "react";
import { textLayout } from "../lib/compose";
import { elementBounds, type Element, type TextElement } from "../lib/model";
import { stringWidth } from "../lib/text";
import * as S from "../state";
import * as T from "../tools";
import { useTheme } from "./controls";
import { TerminalView, type CellSize } from "./Terminal";

const rect = (x: number, y: number, w: number, h: number, c: CellSize): CSSProperties => ({
  left: x * c.w,
  top: y * c.h,
  width: w * c.w,
  height: h * c.h,
});

const BOX_HANDLES = ["nw", "n", "ne", "e", "se", "s", "sw", "w"] as const;

function handlePos(h: string, b: { x: number; y: number; w: number; h: number }, c: CellSize): CSSProperties {
  const l = b.x * c.w;
  const r = (b.x + b.w) * c.w;
  const t = b.y * c.h;
  const bo = (b.y + b.h) * c.h;
  const x = h.includes("w") ? l : h.includes("e") ? r : (l + r) / 2;
  const y = h.includes("n") ? t : h.includes("s") ? bo : (t + bo) / 2;
  return { left: x, top: y };
}

export function Stage() {
  const s = S.useEditor();
  const theme = useTheme();
  const grid = S.gridOf(s.doc);
  const sel = S.selectedElements(s);
  const editing = s.doc.elements.find((e) => e.id === s.editingId);
  const hovered = s.tool === "select" && s.hover && !s.dragging ? T.hitTest(s.hover) : null;

  return (
    <main className="stage" data-tool={s.tool}>
      <div className="stage-inner">
        <div className="term-label">
          <span>{s.doc.name}</span>
          <span className="muted">
            {s.doc.cols}×{s.doc.rows}
          </span>
        </div>
        <TerminalView
          grid={grid}
          theme={theme}
          fontSize={s.fontSize}
          fontFamily={s.fontFamily}
          onDown={T.pointerDown}
          onHandle={T.handleDown}
          onMove={T.pointerMove}
          onUp={T.pointerUp}
          onLeave={T.pointerLeave}
          onDouble={T.doubleClick}
        >
          {(c) => (
            <>
              {s.grid && <div className="grid-lines" style={{ backgroundSize: `${c.w}px ${c.h}px` }} />}
              {hovered && !sel.includes(hovered) && <Outline el={hovered} c={c} className="hover-outline" />}
              {s.tool !== "select" && s.hover && <HoverCell s={s} c={c} />}
              {sel.map((e) => (
                <Outline key={e.id} el={e} c={c} className={`sel-outline ${e.locked ? "locked" : ""}`} />
              ))}
              {sel.length === 1 && s.tool === "select" && !sel[0]!.locked && <Handles el={sel[0]!} c={c} />}
              {s.marquee && (
                <div
                  className="marquee"
                  style={rect(
                    Math.min(s.marquee.x0, s.marquee.x1),
                    Math.min(s.marquee.y0, s.marquee.y1),
                    Math.abs(s.marquee.x1 - s.marquee.x0) + 1,
                    Math.abs(s.marquee.y1 - s.marquee.y0) + 1,
                    c,
                  )}
                />
              )}
              {editing?.type === "text" && <TextEditor key={editing.id} el={editing} c={c} />}
              <div className="canvas-handle" data-handle="canvas" title="Drag to resize the canvas" />
            </>
          )}
        </TerminalView>
      </div>
    </main>
  );
}

function Outline({ el, c, className }: { el: Element; c: CellSize; className: string }) {
  const b = elementBounds(el);
  if (b.w <= 0) return null;
  return <div className={className} style={rect(b.x, b.y, b.w, b.h, c)} />;
}

function HoverCell({ s, c }: { s: S.State; c: CellSize }) {
  const h = s.hover!;
  if (s.tool === "pencil" || s.tool === "eraser") {
    const n = s.brushSize;
    const off = Math.floor((n - 1) / 2);
    return <div className={`brush-outline ${s.tool}`} style={rect(h.x - off, h.y - off, n, n, c)} />;
  }
  return <div className="cell-cursor" style={rect(h.x, h.y, 1, 1, c)} />;
}

function Handles({ el, c }: { el: Element; c: CellSize }) {
  const b = elementBounds(el);
  if (el.type === "box") {
    return (
      <>
        {BOX_HANDLES.map((h) => (
          <div key={h} className={`handle h-${h}`} data-handle={h} style={handlePos(h, b, c)} />
        ))}
      </>
    );
  }
  if (el.type === "line") {
    const [a, z] = el.dir === "h" ? ["w", "e"] : ["n", "s"];
    return (
      <>
        <div className={`handle h-${a}`} data-handle="start" style={handlePos(a!, b, c)} />
        <div className={`handle h-${z}`} data-handle="end" style={handlePos(z!, b, c)} />
      </>
    );
  }
  return null;
}

// Typing straight onto the canvas: a hidden textarea owns the text and the
// selection, the terminal shows the result, and a drawn caret marks the spot.
function TextEditor({ el, c }: { el: TextElement; c: CellSize }) {
  const ta = useRef<HTMLTextAreaElement>(null);
  const [caret, setCaret] = useState(el.text.length);

  useLayoutEffect(() => {
    const t = ta.current!;
    t.focus();
    t.setSelectionRange(t.value.length, t.value.length);
    setCaret(t.value.length);
  }, []);

  useEffect(() => {
    const onSel = () => ta.current && document.activeElement === ta.current && setCaret(ta.current.selectionEnd);
    document.addEventListener("selectionchange", onSel);
    return () => document.removeEventListener("selectionchange", onSel);
  }, []);

  const before = el.text.slice(0, caret);
  const row = before.split("\n").length - 1;
  const L = textLayout(el);
  const line = L.lines[row];
  const col = (line?.offset ?? 0) + stringWidth(before.slice(before.lastIndexOf("\n") + 1));

  return (
    <>
      <textarea
        ref={ta}
        className="ghost-input"
        style={rect(el.x, el.y, Math.max(1, L.width), Math.max(1, L.lines.length), c)}
        value={el.text}
        spellCheck={false}
        onChange={(e) => {
          S.updateElement<TextElement>(el.id, { text: e.target.value }, "skip");
          setCaret(e.target.selectionEnd);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape" || (e.key === "Enter" && (e.metaKey || e.ctrlKey))) {
            e.preventDefault();
            T.finishEditing();
          }
        }}
        onBlur={() => S.get().editingId === el.id && T.finishEditing()}
      />
      <div className="caret" style={{ left: (el.x + col) * c.w, top: (el.y + row) * c.h, height: c.h }} />
    </>
  );
}
