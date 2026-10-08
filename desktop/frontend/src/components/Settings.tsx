import { useEffect, useState } from "react";
import { PanelLeft } from "lucide-react";
import { api, openURL } from "../lib/bridge";
import { tokens } from "../lib/format";
import { shortcut } from "../lib/platform";
import { applyTheme } from "../lib/theme";
import { getState, inputFrom, refreshCatalog, refreshChat, setState, toast, useStore } from "../lib/store";
import type { ModelOption, SettingsInput, Theme } from "../lib/types";
import { PathPicker } from "./PathPicker";
import { Plans } from "./Plans";

// Settings is a page in place of the chat; the sidebar stays, and
// opening a chat from it, Escape, or Cancel goes back.
export function Settings() {
  const settings = useStore((s) => s.settings)!;
  const version = useStore((s) => s.version);
  const [form, setForm] = useState<SettingsInput>(() => inputFrom(settings));
  const [models, setModels] = useState<ModelOption[] | null>(null);
  const [modelErr, setModelErr] = useState("");
  const [saving, setSaving] = useState(false);
  const workspace = useStore((s) => s.workspace);
  const [picking, setPicking] = useState(false);
  const sidebar = useStore((s) => s.sidebar);
  const plan = useStore((s) => s.plans.find((p) => p.id === s.plan));
  const [changing, setChanging] = useState(false);

  const set = <K extends keyof SettingsInput>(k: K, v: SettingsInput[K]) => setForm((f) => ({ ...f, [k]: v }));

  useEffect(() => {
    setModels(null);
    setModelErr("");
    api
      .models(form.local)
      .then((m) => setModels(m ?? []))
      .catch((e) => setModelErr(String(e)));
  }, [form.local]);

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
      refreshCatalog();
      const id = getState().chatId;
      if (id) await refreshChat(id);
    } catch (e) {
      toast(e);
    } finally {
      setSaving(false);
    }
  };

  const envKey = settings.apiKey && settings.keySource !== "config.json";
  const modelValue = form.local ? form.localModel : form.model;
  const listed = models?.some((m) => m.id === modelValue);

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
            {plan ? (
              <div className="row">
                <div className="row-head">
                  <div>
                    <div className="row-title">{plan.name}</div>
                    <div className="row-hint">
                      {[plan.price ? `$${plan.price} a month` : "", ...plan.lines].filter(Boolean).join(" · ")}
                    </div>
                  </div>
                  <button className="btn" onClick={() => setChanging(!changing)}>
                    {changing ? "Done" : "Change plan"}
                  </button>
                </div>
                {changing && (
                  <div className="row-plans">
                    <Plans />
                  </div>
                )}
              </div>
            ) : (
              <div className="row">
                <div className="row-title">Pick your weight class</div>
                <div className="row-hint">caveira runs on a plan. Nothing gets sent until you have one.</div>
                <div className="row-plans">
                  <Plans />
                </div>
              </div>
            )}
          </section>

          <section className="settings-section">
            <h2>Model</h2>
            <div className="row">
              <div className="row-title">Model runs on</div>
              <div className="segmented" style={{ marginTop: 9 }}>
                <button className={form.local ? "" : "on"} onClick={() => set("local", false)}>
                  abliteration.ai
                </button>
                <button className={form.local ? "on" : ""} onClick={() => set("local", true)}>
                  This machine
                </button>
              </div>
            </div>

            {form.local ? (
              <div className="row">
                <div className="row-title">Local server</div>
                <div className="row-hint">Ollama, or any OpenAI-compatible server. Nothing leaves this machine.</div>
                <input
                  className="field mono"
                  value={form.localBaseUrl}
                  onChange={(e) => set("localBaseUrl", e.target.value)}
                  spellCheck={false}
                />
              </div>
            ) : (
              <>
                <div className="row">
                  <div className="row-head">
                    <div className="row-title">API key</div>
                    {settings.keySource === "config.json" && form.apiKey === null && (
                      <button className="btn quiet" onClick={() => set("apiKey", "")}>
                        Remove
                      </button>
                    )}
                  </div>
                  <div className="row-hint">
                    {envKey ? (
                      <>
                        Using {settings.apiKey} from {settings.keySource}, which wins over a key saved here.
                      </>
                    ) : (
                      <>
                        Get one at{" "}
                        <button className="link" onClick={() => openURL("https://abliteration.ai")}>
                          abliteration.ai
                        </button>
                        . Saved in ~/.caveira/config.json, which the terminal client reads too.
                      </>
                    )}
                  </div>
                  <input
                    className="field mono"
                    type="password"
                    placeholder={
                      form.apiKey === "" ? "Will be removed" : settings.apiKey && !envKey ? settings.apiKey : "ak_…"
                    }
                    value={form.apiKey ?? ""}
                    onChange={(e) => set("apiKey", e.target.value || null)}
                    spellCheck={false}
                  />
                </div>
                <div className="row">
                  <div className="row-title">Endpoint</div>
                  <input
                    className="field mono"
                    value={form.baseUrl}
                    onChange={(e) => set("baseUrl", e.target.value)}
                    spellCheck={false}
                  />
                </div>
              </>
            )}

            <div className="row">
              <div className="row-title">Model for new chats</div>
              {modelErr ? (
                <div className="row-hint" style={{ color: "var(--red)" }}>
                  {modelErr}
                </div>
              ) : (
                <select
                  className="field"
                  value={modelValue}
                  onChange={(e) => set(form.local ? "localModel" : "model", e.target.value)}
                >
                  {form.local && <option value="">Pick one for me</option>}
                  {!listed && modelValue && <option value={modelValue}>{modelValue}</option>}
                  {!models && <option disabled>Loading…</option>}
                  {models?.map((m) => (
                    <option key={m.id} value={m.id} disabled={Boolean(m.unusable)}>
                      {m.name || m.id.replace(/^caveira\//, "")}
                      {m.unusable ? ` (${m.unusable})` : m.context ? ` · ${tokens(m.context)}` : ""}
                    </option>
                  ))}
                </select>
              )}
            </div>

            {!form.local && (
              <div className="row">
                <div className="row-title">Reasoning effort</div>
                <select className="field" value={form.effort} onChange={(e) => set("effort", e.target.value)}>
                  <option value="">Model default</option>
                  {settings.efforts.map((e) => (
                    <option key={e} value={e}>
                      {e}
                    </option>
                  ))}
                </select>
              </div>
            )}
          </section>

          <section className="settings-section">
            <h2>General</h2>
            <div className="row">
              <div className="row-head">
                <div>
                  <div className="row-title">Ask before commands and edits</div>
                  <div className="row-hint">Otherwise caveira works on its own until the task is done.</div>
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
