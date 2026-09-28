import { useEffect, useState } from "react";
import { Check, ChevronRight, Minus } from "lucide-react";
import { api, errorText, onImport } from "../lib/bridge";
import { ago } from "../lib/format";
import { refreshHeaders, refreshProjects } from "../lib/store";
import type { ImportApp, ImportProgress, ImportResult } from "../lib/types";

// ImportPanel offers the chats other agents left on this machine, by app
// and folder, and brings over the ones picked. Folders already brought
// over in full are shown but not picked again.

interface Props {
  // onDone is called when the user moves on, with what was imported.
  onDone: (result: ImportResult | null) => void;
  // skipLabel names the way out before anything is imported.
  skipLabel: string;
  // onBusy hears when an import starts and ends.
  onBusy?: (busy: boolean) => void;
}

export function ImportPanel({ onDone, skipLabel, onBusy }: Props) {
  const [apps, setApps] = useState<ImportApp[] | null>(null);
  const [picked, setPicked] = useState<Record<string, Set<string>>>({});
  const [open, setOpen] = useState<Record<string, boolean>>({});
  const [progress, setProgress] = useState<ImportProgress | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .scanImports()
      .then((found) => {
        const list = found ?? [];
        setApps(list);
        const pick: Record<string, Set<string>> = {};
        for (const a of list) pick[a.id] = new Set(a.projects.filter((p) => p.done < p.chats).map((p) => p.path));
        setPicked(pick);
      })
      .catch((e) => {
        setApps([]);
        setError(errorText(e));
      });
  }, []);

  const waiting = (appId: string, path: string) => {
    const p = apps?.find((a) => a.id === appId)?.projects.find((x) => x.path === path);
    return p ? p.chats - p.done : 0;
  };
  const total = Object.entries(picked).reduce(
    (n, [app, dirs]) => n + [...dirs].reduce((m, d) => m + waiting(app, d), 0),
    0,
  );

  const toggle = (app: string, dirs: string[], on: boolean) =>
    setPicked((p) => {
      const next = new Set(p[app]);
      for (const d of dirs) {
        if (on) next.add(d);
        else next.delete(d);
      }
      return { ...p, [app]: next };
    });

  const run = async () => {
    setProgress({ done: 0, total });
    onBusy?.(true);
    const stop = onImport(setProgress);
    try {
      const picks = Object.entries(picked)
        .filter(([, dirs]) => dirs.size > 0)
        .map(([app, dirs]) => ({ app, dirs: [...dirs] }));
      const r = await api.importChats(picks);
      setResult(r);
      await refreshProjects();
      await refreshHeaders();
    } catch (e) {
      setError(errorText(e));
    } finally {
      stop();
      setProgress(null);
      onBusy?.(false);
    }
  };

  const busy = progress !== null;

  return (
    <div className="import">
      {apps === null ? (
        <div className="import-note shimmer">Looking for chats from Claude Code, Codex, and OpenCode…</div>
      ) : apps.length === 0 ? (
        <div className="import-note">
          {error || "There are no chats from Claude Code, Codex, or OpenCode on this Mac."}
        </div>
      ) : result ? (
        <div className="import-done">
          <div className="import-done-head">
            <Check size={16} />
            {result.chats > 0
              ? `Brought over ${plural(result.chats, "chat")}${result.projects ? ` and ${plural(result.projects, "project")}` : ""}.`
              : "Nothing new to bring over."}
          </div>
          <div className="import-done-sub">
            {[
              result.skipped && `${result.skipped} already here`,
              result.empty && `${result.empty} empty, left out`,
              result.failed && `${result.failed} could not be read`,
            ]
              .filter(Boolean)
              .join(" · ") || "They are in the sidebar under their projects. The originals are untouched."}
          </div>
        </div>
      ) : (
        <div className="import-apps">
          {apps.map((a) => {
            const dirs = a.projects.filter((p) => p.done < p.chats).map((p) => p.path);
            const on = picked[a.id] ?? new Set<string>();
            const all = dirs.length > 0 && dirs.every((d) => on.has(d));
            const some = !all && dirs.some((d) => on.has(d));
            return (
              <div key={a.id} className={`import-app${open[a.id] ? " open" : ""}`}>
                <div className="import-app-head">
                  <Tick
                    state={all ? "on" : some ? "some" : "off"}
                    disabled={busy || dirs.length === 0}
                    onClick={() => toggle(a.id, dirs, !all)}
                  />
                  <button className="import-app-title" onClick={() => setOpen((o) => ({ ...o, [a.id]: !o[a.id] }))}>
                    <span className="grow">
                      <span className="import-app-name">{a.name}</span>
                      <span className="import-meta">
                        {plural(a.chats, "chat")} · {plural(a.projects.length, "project")}
                        {dirs.length === 0 && " · all imported"}
                      </span>
                    </span>
                    <ChevronRight size={15} className="chev" />
                  </button>
                </div>
                {open[a.id] && (
                  <div className="import-projects">
                    {a.projects.map((p) => {
                      const done = p.done >= p.chats;
                      return (
                        <div key={p.path} className={`import-project${done ? " done" : ""}`} title={p.path}>
                          <Tick
                            state={done || on.has(p.path) ? "on" : "off"}
                            disabled={busy || done}
                            onClick={() => toggle(a.id, [p.path], !on.has(p.path))}
                          />
                          <span className="grow">
                            {p.name}
                            <span className="sub">{p.short}</span>
                          </span>
                          <span className="import-meta">
                            {done ? "imported" : plural(p.chats - p.done, "chat")} · {ago(p.updated)}
                          </span>
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {progress && (
        <div className="import-progress">
          <div className="bar">
            <div style={{ width: `${progress.total ? (100 * progress.done) / progress.total : 0}%` }} />
          </div>
          <span>
            {progress.done} of {progress.total}
          </span>
        </div>
      )}
      {error && apps && apps.length > 0 && <div className="import-error">{error}</div>}

      <div className="step-actions">
        {result || apps?.length === 0 ? (
          <button className="btn primary big" autoFocus onClick={() => onDone(result)}>
            Continue
          </button>
        ) : (
          <>
            <button className="btn quiet big" disabled={busy} onClick={() => onDone(null)}>
              {skipLabel}
            </button>
            <button className="btn primary big" disabled={busy || apps === null || total === 0} onClick={run}>
              {busy ? "Importing…" : total > 0 ? `Import ${plural(total, "chat")}` : "Import"}
            </button>
          </>
        )}
      </div>
    </div>
  );
}

function Tick({ state, disabled, onClick }: { state: "on" | "off" | "some"; disabled?: boolean; onClick: () => void }) {
  return (
    <button
      className={`tick ${state}`}
      role="checkbox"
      aria-checked={state === "some" ? "mixed" : state === "on"}
      disabled={disabled}
      onClick={onClick}
    >
      {state === "on" && <Check size={11} strokeWidth={3} />}
      {state === "some" && <Minus size={11} strokeWidth={3} />}
    </button>
  );
}

function plural(n: number, word: string): string {
  return `${n.toLocaleString()} ${word}${n === 1 ? "" : "s"}`;
}
