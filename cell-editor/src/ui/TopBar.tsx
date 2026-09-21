import { useState } from "react";
import { toAnsi, toPlainText } from "../lib/ansi";
import { THEMES } from "../lib/colors";
import { goIdent, toGo } from "../lib/golang";
import * as S from "../state";
import { Menu } from "./controls";

async function copy(text: string, what: string) {
  try {
    await navigator.clipboard.writeText(text);
    S.toast(`copied ${what}`);
  } catch {
    S.toast("clipboard blocked by the browser");
  }
}

function download(text: string, filename: string) {
  const url = URL.createObjectURL(new Blob([text], { type: "text/plain" }));
  const a = document.createElement("a");
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

const SAVE_LABEL: Record<S.SaveState, string> = {
  idle: "",
  dirty: "unsaved",
  saving: "saving…",
  saved: "saved",
  error: "save failed",
};

export function TopBar() {
  const s = S.useEditor();
  const { doc } = s;
  const [draft, setDraft] = useState<string | null>(null);
  const grid = () => S.gridOf(S.get().doc);

  return (
    <header className="topbar">
      <div className="brand">
        <span className="brand-mark">▚</span>
        <span className="brand-name">caveira</span>
        <span className="brand-sep">/</span>
        <span className="brand-app">cells</span>
      </div>

      <Menu label={<><span className="doc-name">{doc.name}</span><span className="caret-down">▾</span></>} className="btn doc-btn">
        {(close) => (
          <div className="doc-menu">
            <div className="pop-label">designs · {s.dir.replace(/.*\/(tui\/designs)$/, "$1") || "…"}</div>
            <div className="doc-list">
              {s.designs.map((d) => (
                <div key={d.name} className={`doc-item ${d.name === doc.name ? "on" : ""}`}>
                  <button
                    type="button"
                    className="doc-open"
                    onClick={() => {
                      close();
                      if (d.name !== doc.name) void S.openDesign(d.name);
                    }}
                  >
                    {d.name}
                  </button>
                  <button
                    type="button"
                    className="doc-del"
                    title={`Delete ${d.name}`}
                    onClick={() => {
                      if (confirm(`Delete ${d.name}? This removes its .cells.json, .ans and .txt.`)) void S.deleteDesign(d.name);
                    }}
                  >
                    ✕
                  </button>
                </div>
              ))}
            </div>
            <form
              className="doc-new"
              onSubmit={(e) => {
                e.preventDefault();
                const name = new FormData(e.currentTarget).get("name") as string;
                close();
                void S.createDesign(name || "untitled");
              }}
            >
              <input name="name" placeholder="new design name" autoComplete="off" />
              <button type="submit" className="btn">new</button>
            </form>
            <div className="btn-row">
              <button
                type="button"
                className="btn"
                onClick={() => {
                  close();
                  void S.createDesign(`${doc.name}-copy`, doc);
                }}
              >
                duplicate
              </button>
              <button
                type="button"
                className="btn"
                onClick={() => {
                  close();
                  setDraft(doc.name);
                }}
              >
                rename
              </button>
            </div>
          </div>
        )}
      </Menu>
      {draft !== null && (
        <form
          className="rename-inline"
          onSubmit={(e) => {
            e.preventDefault();
            void S.renameDesign(draft);
            setDraft(null);
          }}
        >
          <input autoFocus value={draft} onChange={(e) => setDraft(e.target.value)} onBlur={() => setDraft(null)} onKeyDown={(e) => e.key === "Escape" && setDraft(null)} />
        </form>
      )}

      <span className={`save-state ${s.save}`}>{SAVE_LABEL[s.save]}</span>

      <div className="spacer" />

      <div className="seg">
        <button type="button" title="Undo (⌘Z)" disabled={!s.undo.length} onClick={S.undo}>↶</button>
        <button type="button" title="Redo (⇧⌘Z)" disabled={!s.redo.length} onClick={S.redo}>↷</button>
      </div>

      <Menu label="view" align="right">
        {() => (
          <div className="view-menu">
            <div className="pop-label">preview theme</div>
            <div className="seg">
              {THEMES.map((t) => (
                <button key={t.id} type="button" className={s.themeId === t.id ? "on" : ""} onClick={() => S.set({ themeId: t.id })}>
                  {t.label}
                </button>
              ))}
            </div>
            <div className="pop-label">font size · {s.fontSize}px</div>
            <input type="range" min={9} max={28} value={s.fontSize} onChange={(e) => S.set({ fontSize: Number(e.target.value) })} />
            <div className="pop-label">font family (use your terminal's)</div>
            <input className="text-input" value={s.fontFamily} onChange={(e) => S.set({ fontFamily: e.target.value })} />
            <label className="check">
              <input type="checkbox" checked={s.grid} onChange={(e) => S.set({ grid: e.target.checked })} /> cell grid (G)
            </label>
          </div>
        )}
      </Menu>

      <Menu label="export" align="right" className="btn accent">
        {(close) => {
          const run = (f: () => void) => () => {
            close();
            f();
          };
          return (
            <div className="export-menu">
              <button type="button" onClick={run(() => copy(toAnsi(grid()), "ANSI"))}>
                copy ANSI <span className="muted">paste into a terminal</span>
              </button>
              <button type="button" onClick={run(() => copy(toPlainText(grid()), "plain text"))}>
                copy plain text
              </button>
              <button type="button" onClick={run(() => copy(toGo(S.get().doc), "Go"))}>
                copy Go <span className="muted">lipgloss v2 layers</span>
              </button>
              <hr />
              <button type="button" onClick={run(() => download(toAnsi(grid()), `${doc.name}.ans`))}>
                download .ans
              </button>
              <button type="button" onClick={run(() => download(toGo(S.get().doc), `${goIdent(doc.name).toLowerCase()}.go`))}>
                download .go
              </button>
              <button type="button" onClick={run(() => download(JSON.stringify(S.get().doc, null, 2), `${doc.name}.cells.json`))}>
                download .cells.json
              </button>
            </div>
          );
        }}
      </Menu>
    </header>
  );
}
