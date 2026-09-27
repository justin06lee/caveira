import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, Check, ChevronDown, Square } from "lucide-react";
import { api } from "../lib/bridge";
import { cost, tokens } from "../lib/format";
import { useDismiss } from "../lib/hooks";
import { patchChat, send, setState, toast, useStore, type Chat } from "../lib/store";
import type { ModelOption } from "../lib/types";

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

// Model lists are fetched once per provider per session.
const modelCache = new Map<boolean, ModelOption[]>();

function ModelChip({ chat }: { chat: Chat }) {
  const [open, setOpen] = useState(false);
  const [models, setModels] = useState<ModelOption[] | null>(() => modelCache.get(chat.local) ?? null);
  const [err, setErr] = useState("");
  const close = useCallback(() => setOpen(false), []);
  const ref = useDismiss<HTMLDivElement>(open, close);

  useEffect(() => {
    if (!open) return;
    const cached = modelCache.get(chat.local);
    if (cached) {
      setModels(cached);
      return;
    }
    setModels(null);
    setErr("");
    api
      .models(chat.local)
      .then((m) => {
        modelCache.set(chat.local, m ?? []);
        setModels(m ?? []);
      })
      .catch((e) => setErr(String(e)));
  }, [open, chat.local]);

  const choose = async (model: string, window: number, effort: string) => {
    try {
      const v = await api.setModel(chat.id, model, window, effort);
      patchChat(chat.id, (c) => ({ ...c, model: v.model, effort: v.effort, window: v.window }));
      setState((s) => (s.settings && !chat.local ? { settings: { ...s.settings, model: v.model, effort: v.effort } } : {}));
    } catch (e) {
      toast(e);
    }
  };

  const current = models?.find((m) => m.id === chat.model);
  const efforts = useStore((s) => s.settings?.efforts) ?? fallbackEfforts;
  const showEffort = !chat.local && !current?.noEffort;
  const name = chat.model.replace(/^caveira\//, "");

  return (
    <div ref={ref}>
      <button className={`chip${open ? " open" : ""}`} disabled={chat.running} onClick={() => setOpen(!open)}>
        <span>{name}</span>
        {chat.effort && !chat.local && <span style={{ color: "var(--faint)" }}>· {chat.effort}</span>}
        <ChevronDown size={12} />
      </button>
      {open && (
        <div className="menu model-menu">
          <div className="menu-label">{chat.local ? "Models on this machine" : "Model"}</div>
          {err && <div className="side-empty">{err}</div>}
          {!models && !err && <div className="side-empty">Loading…</div>}
          {models?.map((m) => (
            <button
              key={m.id}
              className="menu-item"
              disabled={Boolean(m.unusable)}
              onClick={() => {
                choose(m.id, m.context, m.noEffort ? "" : chat.effort);
                close();
              }}
            >
              <span className="grow">
                {m.id.replace(/^caveira\//, "")}
                <span className="sub">
                  {m.unusable || [m.context ? `${tokens(m.context)} context` : "", m.note].filter(Boolean).join(" · ")}
                </span>
              </span>
              {m.id === chat.model && <Check size={14} className="check" />}
            </button>
          ))}
          {showEffort && (
            <>
              <div className="menu-sep" />
              <div className="menu-label">Reasoning effort</div>
              <div className="efforts">
                {["", ...efforts].map((e) => (
                  <button
                    key={e || "default"}
                    className={`effort${chat.effort === e ? " on" : ""}`}
                    onClick={() => choose(chat.model, chat.window, e)}
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
