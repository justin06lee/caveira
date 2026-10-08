import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, Check, ChevronDown, Square } from "lucide-react";
import { api } from "../lib/bridge";
import { cost, tokens } from "../lib/format";
import { useDismiss } from "../lib/hooks";
import { patchChat, send, setState, toast, useStore, type Chat } from "../lib/store";
import type { ModelOption } from "../lib/types";
import { ModelLogos } from "./ModelLogos";

// Drafts outlive switching chats.
const drafts = new Map<string, string>();

export function Composer({ chat }: { chat: Chat }) {
  const [text, setText] = useState(() => drafts.get(chat.id) ?? "");
  const ref = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    setText(drafts.get(chat.id) ?? "");
    ref.current?.focus();
  }, [chat.id]);

  useLayoutEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.style.height = "auto";
    el.style.height = `${el.scrollHeight}px`;
    el.style.overflowY = el.scrollHeight > el.clientHeight ? "auto" : "hidden";
  }, [text]);

  const update = (v: string) => {
    setText(v);
    drafts.set(chat.id, v);
  };

  // The draft is cleared before sending: the first message of a chat
  // moves the composer from the middle of the window to the bottom, and
  // the one that comes up there starts from the draft.
  const submit = async () => {
    const t = text.trim();
    if (!t || chat.running) return;
    update("");
    if (!(await send(t))) update(text);
  };

  return (
    <div className="composer">
      <textarea
        ref={ref}
        rows={1}
        value={text}
        placeholder={chat.items.length ? "Reply to caveira…" : "Ask a question, or describe a task…"}
        onChange={(e) => update(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
            e.preventDefault();
            submit();
          } else if (e.key === "Escape" && chat.running) {
            e.preventDefault();
            api.stop(chat.id);
          }
        }}
      />
      <div className="composer-bar">
        <ModelChip chat={chat} />
        <Meter chat={chat} />
        {chat.running ? (
          <button className="send" title="Stop (esc)" onClick={() => api.stop(chat.id)}>
            <Square size={11} fill="currentColor" />
          </button>
        ) : (
          <button className="send" title="Send (↵)" disabled={!text.trim()} onClick={submit}>
            <ArrowUp size={17} strokeWidth={2.2} />
          </button>
        )}
      </div>
    </div>
  );
}

function Meter({ chat }: { chat: Chat }) {
  const share = chat.window > 0 ? Math.min(1, chat.context / chat.window) : 0;
  const r = 5.5;
  const c = 2 * Math.PI * r;
  const spent = cost(chat.cost);
  if (!chat.context && !spent) return <span className="meter" />;
  return (
    <span
      className="meter"
      title={chat.window ? `${tokens(chat.context)} of ${tokens(chat.window)} tokens in context` : undefined}
    >
      {spent}
      {chat.context > 0 && chat.window > 0 && <span>{Math.max(1, Math.round(share * 100))}%</span>}
      {chat.context > 0 && chat.window > 0 && (
        <svg className="ring" width="14" height="14" viewBox="0 0 14 14">
          <circle className="track" cx="7" cy="7" r={r} fill="none" strokeWidth="1.6" />
          <circle
            className="fill"
            cx="7"
            cy="7"
            r={r}
            fill="none"
            strokeWidth="1.6"
            strokeDasharray={c}
            strokeDashoffset={c * (1 - share)}
            strokeLinecap="round"
          />
        </svg>
      )}
    </span>
  );
}

// ModelChip is the model under the composer, and the menu that switches
// it: the models caveira offers on abliteration.ai, each beside its
// makers' logos, then the reasoning effort.
function ModelChip({ chat }: { chat: Chat }) {
  const [open, setOpen] = useState(false);
  const catalog = useStore((s) => s.catalog);
  const close = useCallback(() => setOpen(false), []);
  const ref = useDismiss<HTMLDivElement>(open, close);

  const choose = async (m: ModelOption, effort: string) => {
    try {
      const v = await api.setModel(chat.id, m.id, m.context, m.noEffort ? "" : effort);
      patchChat(chat.id, (c) => ({ ...c, ...v, items: v.items ?? c.items, turns: c.turns }));
      setState((s) => (s.settings ? { settings: { ...s.settings, model: v.model, effort: v.effort } } : {}));
    } catch (e) {
      toast(e);
    }
  };

  const current = catalog.find((m) => m.id === chat.model);
  const efforts = useStore((s) => s.settings?.efforts) ?? fallbackEfforts;
  const name = current?.name ?? chat.model;

  const groups: { label: string; models: ModelOption[] }[] = [];
  for (const m of catalog) {
    const label = m.group ?? "Models";
    const g = groups.find((x) => x.label === label);
    if (g) g.models.push(m);
    else groups.push({ label, models: [m] });
  }

  return (
    <div ref={ref}>
      <button
        className={`chip model-chip${open ? " open" : ""}`}
        disabled={chat.running}
        onClick={() => setOpen(!open)}
      >
        <ModelLogos logos={current?.logos} height={11} />
        <span>{name}</span>
        {chat.effort && <span style={{ color: "var(--faint)" }}>· {chat.effort}</span>}
        <ChevronDown size={12} />
      </button>
      {open && (
        <div className="menu model-menu">
          {groups.map((g) => (
            <div key={g.label}>
              <div className="menu-group">{g.label}</div>
              {g.models.map((m) => (
                <button
                  key={m.id}
                  className="menu-item model-item"
                  disabled={Boolean(m.unusable)}
                  onClick={() => {
                    close();
                    if (m.id !== chat.model) choose(m, chat.effort);
                  }}
                >
                  <ModelLogos logos={m.logos} />
                  <span className="grow">
                    {m.name || m.id}
                    {m.unusable && <span className="sub">{m.unusable}</span>}
                  </span>
                  {m.id === chat.model && <Check size={15} className="check" />}
                </button>
              ))}
            </div>
          ))}
          {!catalog.length && <div className="side-empty">Loading…</div>}
          {!current?.noEffort && (
            <>
              <div className="menu-sep" />
              <div className="menu-group">Reasoning effort</div>
              <div className="efforts">
                {["", ...efforts].map((e) => (
                  <button
                    key={e || "default"}
                    className={`effort${chat.effort === e ? " on" : ""}`}
                    onClick={() => choose(current ?? { id: chat.model, name, context: chat.window, note: "" }, e)}
                  >
                    {e || "default"}
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      )}
    </div>
  );
}

const fallbackEfforts = ["none", "minimal", "low", "medium", "high", "xhigh", "max"];
