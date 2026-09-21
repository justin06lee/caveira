import { useEffect } from "react";
import { parseAnsi, toPlainText } from "../lib/ansi";
import { compose } from "../lib/compose";
import { elementBounds, newPaint, nextName, type Element } from "../lib/model";
import * as S from "../state";
import * as T from "../tools";
import { Inspector } from "./Inspector";
import { Sidebar, TOOLS } from "./Sidebar";
import { Stage } from "./Stage";
import { TopBar } from "./TopBar";

const MIME = "application/x-caveira-cells";

function unionBounds(els: Element[]) {
  const bs = els.map(elementBounds).filter((b) => b.w > 0);
  if (!bs.length) return null;
  const x = Math.min(...bs.map((b) => b.x));
  const y = Math.min(...bs.map((b) => b.y));
  return { x, y, w: Math.max(...bs.map((b) => b.x + b.w)) - x, h: Math.max(...bs.map((b) => b.y + b.h)) - y };
}

// Plain text of just these elements, cropped to where they are.
function selectionText(els: Element[]): string {
  const u = unionBounds(els);
  if (!u) return "";
  const moved = els.map((e) => ({ ...e, x: e.x - u.x, y: e.y - u.y }));
  return toPlainText(compose({ version: 1, name: "clip", cols: u.w, rows: u.h, elements: moved }));
}

function typingIn(t: EventTarget | null): boolean {
  return t instanceof HTMLElement && !!t.closest("input, textarea, select, [contenteditable]");
}

function onKey(e: KeyboardEvent) {
  const mod = e.metaKey || e.ctrlKey;
  const key = e.key.toLowerCase();
  if (mod && key === "s") {
    e.preventDefault();
    void S.flushSave().then(() => S.toast("saved"));
    return;
  }
  if (typingIn(e.target)) return;
  const s = S.get();
  if (mod) {
    if (key === "z") {
      e.preventDefault();
      if (e.shiftKey) S.redo();
      else S.undo();
    } else if (key === "y") {
      e.preventDefault();
      S.redo();
    } else if (key === "d") {
      e.preventDefault();
      S.duplicateSelection();
    } else if (key === "a") {
      e.preventDefault();
      S.set({ selected: s.doc.elements.filter((el) => !el.locked && !el.hidden).map((el) => el.id), tool: "select" });
    }
    return;
  }
  const step = e.shiftKey ? 5 : 1;
  switch (e.key) {
    case "Escape":
      S.set({ selected: [], tool: "select" });
      return;
    case "Backspace":
    case "Delete":
      e.preventDefault();
      S.removeElements(s.selected);
      return;
    case "ArrowLeft":
    case "ArrowRight":
    case "ArrowUp":
    case "ArrowDown": {
      if (!s.selected.length) return;
      e.preventDefault();
      const dx = e.key === "ArrowLeft" ? -step : e.key === "ArrowRight" ? step : 0;
      const dy = e.key === "ArrowUp" ? -step : e.key === "ArrowDown" ? step : 0;
      S.nudge(dx, dy);
      return;
    }
    case "[":
      S.restack("down");
      return;
    case "]":
      S.restack("up");
      return;
    case "{":
      S.restack("bottom");
      return;
    case "}":
      S.restack("top");
      return;
    case "Enter": {
      const only = S.selectedElements();
      if (only.length !== 1) return;
      e.preventDefault();
      if (only[0]!.type === "text") T.startEditing(only[0]!);
      else if (only[0]!.type === "box") window.dispatchEvent(new CustomEvent("cells:edit-box-text"));
      return;
    }
    case "g":
      S.set({ grid: !s.grid });
      return;
    case "=":
    case "+":
      S.set({ fontSize: Math.min(28, s.fontSize + 1) });
      return;
    case "-":
      S.set({ fontSize: Math.max(9, s.fontSize - 1) });
      return;
  }
  const tool = TOOLS.find((t) => t.key === key);
  if (tool) S.set({ tool: tool.id });
}

function onCopy(e: ClipboardEvent, cut: boolean) {
  if (typingIn(e.target) || typingIn(document.activeElement)) return;
  const els = S.selectedElements();
  if (!els.length || !e.clipboardData) return;
  e.preventDefault();
  e.clipboardData.setData(MIME, JSON.stringify(els));
  e.clipboardData.setData("text/plain", selectionText(els));
  if (cut) S.removeElements(els.map((el) => el.id));
  S.toast(`${cut ? "cut" : "copied"} ${els.length} element${els.length > 1 ? "s" : ""}`);
}

function onPaste(e: ClipboardEvent) {
  if (typingIn(e.target) || typingIn(document.activeElement) || !e.clipboardData) return;
  const s = S.get();
  const custom = e.clipboardData.getData(MIME);
  e.preventDefault();
  if (custom) {
    let els: Element[];
    try {
      els = JSON.parse(custom);
    } catch {
      return;
    }
    const u = unionBounds(els);
    const dx = s.hover && u ? s.hover.x - u.x : 2;
    const dy = s.hover && u ? s.hover.y - u.y : 1;
    const copies = S.cloneElements(els, dx, dy).map((c) => ({ ...c, name: c.name.replace(/ copy$/, "") }));
    S.edit((d) => ({ ...d, elements: [...d.elements, ...copies] }));
    S.set({ selected: copies.map((c) => c.id), tool: "select" });
    return;
  }
  const text = e.clipboardData.getData("text/plain");
  if (!text) return;
  const { cells, width, height } = parseAnsi(text);
  if (!Object.keys(cells).length) return;
  const el = newPaint(s.doc);
  el.name = nextName(s.doc, "Pasted");
  el.x = s.hover?.x ?? 0;
  el.y = s.hover?.y ?? 0;
  el.cells = cells;
  S.addElement(el);
  S.set({ selected: [el.id], tool: "select" });
  S.toast(`pasted ${width}×${height} as a paint layer`);
}

export function App() {
  const s = S.useEditor();

  useEffect(() => {
    void S.boot();
    const copy = (e: ClipboardEvent) => onCopy(e, false);
    const cut = (e: ClipboardEvent) => onCopy(e, true);
    const flush = () => void S.flushSave();
    window.addEventListener("keydown", onKey);
    document.addEventListener("copy", copy);
    document.addEventListener("cut", cut);
    document.addEventListener("paste", onPaste);
    window.addEventListener("pagehide", flush);
    return () => {
      window.removeEventListener("keydown", onKey);
      document.removeEventListener("copy", copy);
      document.removeEventListener("cut", cut);
      document.removeEventListener("paste", onPaste);
      window.removeEventListener("pagehide", flush);
    };
  }, []);

  const tool = TOOLS.find((t) => t.id === s.tool)!;
  return (
    <div className="app">
      <TopBar />
      <Sidebar />
      {s.loaded ? <Stage /> : <main className="stage" />}
      <Inspector />
      <footer className="statusbar">
        <span className="pos">{s.hover ? `${s.hover.x}, ${s.hover.y}` : "–"}</span>
        <span className="hint">{tool.hint}</span>
        <span className="spacer" />
        <span className="muted">
          {s.selected.length ? `${s.selected.length} selected · ` : ""}
          {s.doc.elements.length} elements
        </span>
        <span className="path">{s.dir ? `${s.dir.replace(/.*\/(tui\/designs)$/, "$1")}/${s.doc.name}.cells.json` : ""}</span>
      </footer>
      {s.toast && <div className="toast">{s.toast}</div>}
    </div>
  );
}
