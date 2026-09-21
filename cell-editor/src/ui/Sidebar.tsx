import { useState, type ReactNode } from "react";
import { elementBounds, type Element } from "../lib/model";
import * as S from "../state";

export const TOOLS: { id: S.Tool; key: string; glyph: string; label: string; hint: string }[] = [
  { id: "select", key: "v", glyph: "↖", label: "select", hint: "click or drag to select · drag to move · ⌥-drag copies · double-click edits text" },
  { id: "box", key: "b", glyph: "╭╮", label: "box", hint: "drag out a lipgloss box · click drops a 30×8" },
  { id: "text", key: "t", glyph: "T", label: "text", hint: "click to place text and type · esc to finish" },
  { id: "line", key: "l", glyph: "─", label: "line", hint: "drag a horizontal or vertical line" },
  { id: "pencil", key: "p", glyph: "✎", label: "paint", hint: "paint the brush · right-drag erases · ⇧-click draws a straight stroke" },
  { id: "eraser", key: "e", glyph: "⌫", label: "erase", hint: "erase painted cells" },
  { id: "fill", key: "f", glyph: "◍", label: "fill", hint: "flood-fill a region with the brush" },
  { id: "picker", key: "i", glyph: "◎", label: "pick", hint: "pick a cell's character and colours into the brush" },
];

export function Sidebar() {
  const s = S.useEditor();
  return (
    <nav className="sidebar">
      <div className="tools">
        {TOOLS.map((t) => (
          <button
            key={t.id}
            type="button"
            className={`tool ${s.tool === t.id ? "on" : ""}`}
            title={`${t.label} (${t.key.toUpperCase()})`}
            onClick={() => S.set({ tool: t.id })}
          >
            <span className="tool-glyph">{t.glyph}</span>
            <span className="tool-label">{t.label}</span>
            <span className="tool-key">{t.key}</span>
          </button>
        ))}
      </div>
      <Layers />
    </nav>
  );
}

const GLYPH: Record<Element["type"], string> = { box: "▢", text: "T", line: "─", paint: "▚" };

function Layers() {
  const { doc, selected } = S.useEditor();
  const [drag, setDrag] = useState<{ id: string; over: number } | null>(null);
  const [renaming, setRenaming] = useState<string | null>(null);
  // Top of the list is the top of the stack.
  const rows = doc.elements.map((e, i) => ({ e, i })).reverse();

  const select = (id: string, shift: boolean) => {
    if (shift) S.set((s) => ({ selected: s.selected.includes(id) ? s.selected.filter((x) => x !== id) : [...s.selected, id] }));
    else S.set({ selected: [id], tool: "select" });
  };

  return (
    <div className="layers">
      <div className="layers-head">
        <span>layers</span>
        <span className="muted">{doc.elements.length}</span>
      </div>
      <div className="layers-list" onDragOver={(e) => e.preventDefault()}>
        {rows.length === 0 && <div className="muted empty">nothing yet: pick a tool and draw</div>}
        {rows.map(({ e, i }) => {
          const b = elementBounds(e);
          const item: ReactNode = (
            <div
              key={e.id}
              className={`layer ${selected.includes(e.id) ? "on" : ""} ${e.hidden ? "hidden" : ""} ${drag?.over === i ? "drop" : ""}`}
              draggable={renaming !== e.id}
              onDragStart={(ev) => {
                ev.dataTransfer.effectAllowed = "move";
                setDrag({ id: e.id, over: i });
              }}
              onDragOver={(ev) => {
                ev.preventDefault();
                if (drag && drag.over !== i) setDrag({ ...drag, over: i });
              }}
              onDrop={(ev) => {
                ev.preventDefault();
                if (drag) S.moveElementTo(drag.id, i);
                setDrag(null);
              }}
              onDragEnd={() => setDrag(null)}
              onClick={(ev) => select(e.id, ev.shiftKey)}
              onDoubleClick={() => setRenaming(e.id)}
            >
              <span className="layer-glyph">{GLYPH[e.type]}</span>
              {renaming === e.id ? (
                <input
                  autoFocus
                  className="layer-rename"
                  defaultValue={e.name}
                  onClick={(ev) => ev.stopPropagation()}
                  onBlur={(ev) => {
                    S.updateElement(e.id, { name: ev.target.value || e.name });
                    setRenaming(null);
                  }}
                  onKeyDown={(ev) => {
                    if (ev.key === "Enter") ev.currentTarget.blur();
                    if (ev.key === "Escape") setRenaming(null);
                  }}
                />
              ) : (
                <span className="layer-name">{e.name}</span>
              )}
              <span className="layer-dim">{b.w > 0 ? `${b.w}×${b.h}` : "empty"}</span>
              <button
                type="button"
                className={`layer-btn ${e.locked ? "on" : ""}`}
                title={e.locked ? "Unlock" : "Lock"}
                onClick={(ev) => {
                  ev.stopPropagation();
                  S.updateElement(e.id, { locked: !e.locked || undefined });
                }}
              >
                {e.locked ? "⊘" : "○"}
              </button>
              <button
                type="button"
                className={`layer-btn ${e.hidden ? "on" : ""}`}
                title={e.hidden ? "Show" : "Hide"}
                onClick={(ev) => {
                  ev.stopPropagation();
                  S.updateElement(e.id, { hidden: !e.hidden || undefined });
                }}
              >
                {e.hidden ? "◌" : "◉"}
              </button>
            </div>
          );
          return item;
        })}
      </div>
    </div>
  );
}
