import { useEffect, useState } from "react";
import { PanelLeft } from "lucide-react";
import { api } from "../lib/bridge";
import { effortName, tokens } from "../lib/format";
import { shortcut } from "../lib/platform";
import { applyTheme } from "../lib/theme";
import { getState, inputFrom, refreshChat, setState, toast, useStore } from "../lib/store";
import type { SettingsInput, Theme } from "../lib/types";
import { ModelLogos } from "./ModelLogos";
import { PathPicker } from "./PathPicker";
import { Plans } from "./Plans";
import { Select } from "./Select";

// Settings is a page in place of the chat; the sidebar stays, and
// opening a chat from it, Escape, or Cancel goes back. The plans are
// always at the top.
export function Settings() {
  const settings = useStore((s) => s.settings)!;
  const version = useStore((s) => s.version);
  const models = useStore((s) => s.catalog);
  const [form, setForm] = useState<SettingsInput>(() => inputFrom(settings));
  const [saving, setSaving] = useState(false);
  const workspace = useStore((s) => s.workspace);
  const [picking, setPicking] = useState(false);
  const sidebar = useStore((s) => s.sidebar);

  const set = <K extends keyof SettingsInput>(k: K, v: SettingsInput[K]) => setForm((f) => ({ ...f, [k]: v }));

  const close = () => {
    applyTheme(settings.theme);
    setState(() => ({ settingsOpen: false }));
  };

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const save = async () => {
    setSaving(true);
    try {
      const view = await api.saveSettings(form);
      setState(() => ({ settings: view, settingsOpen: false }));
      applyTheme(view.theme);
      const id = getState().chatId;
      if (id) await refreshChat(id);
    } catch (e) {
      toast(e);
    } finally {
      setSaving(false);
    }
  };

  const listed = models.some((m) => m.id === form.model);

  return (
    <>
      <header className={`header drag${sidebar ? "" : " inset"}`}>
        {sidebar ? (
          <span className="header-spacer" />
        ) : (
          <button
            className="icon-btn no-drag"
            title={`Show sidebar (${shortcut("\\")})`}
            onClick={() => setState(() => ({ sidebar: true }))}
          >
            <PanelLeft size={16} />
          </button>
        )}
        <div className="header-title" />
        <span className="header-spacer" />
      </header>

      <div className="scroll settings-page">
        <div className="settings-column" role="region" aria-label="Settings">
          <h1>Settings</h1>

          <section className="settings-section">
            <h2>Plan</h2>
            <div className="row-plans">
              <Plans />
            </div>
          </section>

          <section className="settings-section">
            <h2>Model</h2>
            <div className="row">
              <div className="row-title">Model for new chats</div>
              <Select
                label="Model for new chats"
                value={form.model}
                placeholder="Loading…"
                onChange={(v) => set("model", v)}
                options={[
                  ...(!listed && form.model ? [{ value: form.model, label: form.model }] : []),
                  ...models.map((m) => ({
                    value: m.id,
                    label: (
                      <>
                        <ModelLogos logos={m.logos} />
                        <span>{m.name || m.id}</span>
                      </>
                    ),
                    aside: m.context ? `${tokens(m.context)} context` : undefined,
                    sub: m.unusable || (m.context ? `${tokens(m.context)} token context` : undefined),
                    disabled: Boolean(m.unusable),
                  })),
                ]}
              />
            </div>

            <div className="row">
              <div className="row-title">Reasoning effort</div>
              <Select
                label="Reasoning effort"
                value={form.effort}
                onChange={(v) => set("effort", v)}
                options={["", ...settings.efforts].map((e) => ({
                  value: e,
                  label: e ? effortName(e) : "Model default",
                }))}
              />
            </div>
          </section>

          <section className="settings-section">
            <h2>General</h2>
            <div className="row">
              <div className="row-head">
                <div>
                  <div className="row-title">Ask permissions in new chats</div>
                  <div className="row-hint">
                    Otherwise new chats bypass every permission and work on their own until the task is done. Each chat
                    switches with the picker under its composer, or ⇧Tab.
                  </div>
                </div>
                <button
                  className={`switch${form.confirm ? " on" : ""}`}
                  role="switch"
                  aria-checked={form.confirm}
                  onClick={() => set("confirm", !form.confirm)}
                />
              </div>
            </div>

            <div className="row">
              <div className="row-head">
                <div>
                  <div className="row-title">Workspace</div>
                  <div className="row-hint">
                    {workspace.path ? (
                      <>
                        Projects open from <span className="mono">{workspace.short}</span>.
                      </>
                    ) : (
                      "Projects open from your home folder."
                    )}
                  </div>
                </div>
                {!picking && (
                  <button className="btn" onClick={() => setPicking(true)}>
                    Change
                  </button>
                )}
              </div>
              {picking && (
                <div className="row-picker">
                  <PathPicker
                    base=""
                    mode="workspace"
                    placeholder="type the folder"
                    initial={workspace.short ? workspace.short + "/" : ""}
                    onPick={async (path) => {
                      try {
                        const ws = await api.setWorkspace(path);
                        setState(() => ({ workspace: ws }));
                        setPicking(false);
                      } catch (e) {
                        toast(e);
                      }
                    }}
                    onCancel={() => setPicking(false)}
                    aside={
                      <button className="btn quiet" onClick={() => setPicking(false)}>
                        Cancel
                      </button>
                    }
                  />
                </div>
              )}
            </div>

            <div className="row">
              <div className="row-head">
                <div>
                  <div className="row-title">Chats from other agents</div>
                  <div className="row-hint">Bring over chats and projects from Claude Code, Codex, and OpenCode.</div>
                </div>
                <button className="btn" onClick={() => setState(() => ({ settingsOpen: false, importOpen: true }))}>
                  Import…
                </button>
              </div>
            </div>

            <div className="row">
              <div className="row-head">
                <div className="row-title">Appearance</div>
                <div className="segmented">
                  {(["system", "light", "dark"] as Theme[]).map((t) => (
                    <button
                      key={t}
                      className={form.theme === t ? "on" : ""}
                      onClick={() => {
                        set("theme", t);
                        applyTheme(t);
                      }}
                    >
                      {t[0].toUpperCase() + t.slice(1)}
                    </button>
                  ))}
                </div>
              </div>
            </div>
          </section>
        </div>
      </div>

      <div className="settings-foot">
        <div className="settings-foot-inner">
          <span className="version">caveira {version}</span>
          <div style={{ display: "flex", gap: 8 }}>
            <button className="btn quiet" onClick={close}>
              Cancel
            </button>
            <button className="btn primary" disabled={saving} onClick={save}>
              Save
            </button>
          </div>
        </div>
      </div>
    </>
  );
}
