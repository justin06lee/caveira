import { useState } from "react";
import { api, openURL } from "../lib/bridge";
import { getState, inputFrom, newChat, setState, toast } from "../lib/store";
import type { SettingsInput } from "../lib/types";

// KeyCard is the first thing a new install sees: a key, or the way to a
// local model instead.
export function KeyCard() {
  const [key, setKey] = useState("");
  const [busy, setBusy] = useState(false);

  const save = async (change: Partial<SettingsInput>) => {
    const s = getState().settings!;
    setBusy(true);
    try {
      const view = await api.saveSettings({ ...inputFrom(s), ...change });
      setState(() => ({ settings: view }));
      await newChat();
    } catch (e) {
      toast(e);
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="key-card">
      <p>
        Paste a key from{" "}
        <button className="link" onClick={() => openURL("https://abliteration.ai")}>
          abliteration.ai
        </button>
        . It is kept in ~/.caveira/config.json, where the terminal client finds it too.
      </p>
      <form
        className="key-row"
        onSubmit={(e) => {
          e.preventDefault();
          if (key.trim()) save({ apiKey: key.trim() });
        }}
      >
        <input
          className="field mono"
          type="password"
          placeholder="ak_…"
          autoFocus
          value={key}
          onChange={(e) => setKey(e.target.value)}
          spellCheck={false}
        />
        <button className="btn primary" disabled={!key.trim() || busy}>
          Save
        </button>
      </form>
      <div className="key-alt">
        Or{" "}
        <button className="link" disabled={busy} onClick={() => save({ local: true })}>
          use a model on this machine
        </button>{" "}
        through Ollama.
      </div>
    </div>
  );
}
