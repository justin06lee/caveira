import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { setState } from "../lib/store";
import { ImportPanel } from "./ImportPanel";

// ImportSheet is the import from other agents after the first run, from
// the File menu or Settings.
export function ImportSheet() {
  const [busy, setBusy] = useState(false);
  const close = () => !busy && setState(() => ({ importOpen: false }));

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  return (
    <div className="scrim" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div className="sheet import-sheet" role="dialog" aria-label="Import from other agents">
        <div className="sheet-head">
          <h2>Import from other agents</h2>
          <button className="icon-btn" onClick={close} disabled={busy} aria-label="Close">
            <X size={16} />
          </button>
        </div>
        <div className="sheet-body">
          <p className="import-intro">
            Chats and projects from Claude Code, Codex, and OpenCode. The originals stay where they are, and a chat
            brought over before is not brought over twice.
          </p>
          <ImportPanel skipLabel="Cancel" onBusy={setBusy} onDone={() => setState(() => ({ importOpen: false }))} />
        </div>
      </div>
    </div>
  );
}
