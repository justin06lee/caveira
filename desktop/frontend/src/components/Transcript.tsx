import { memo, useEffect, useLayoutEffect, useRef, useState } from "react";
import { ArrowDown, ChevronRight, CircleAlert, Info } from "lucide-react";
import { cost, duration, plural } from "../lib/format";
import { pickVerb } from "../lib/rebel";
import { useStore, type Chat } from "../lib/store";
import type { Item } from "../lib/types";
import { Mark } from "./Logo";
import { Markdown } from "./Markdown";
import { Plans } from "./Plans";
import { SkullLoader } from "./SkullLoader";
import { ToolItem } from "./ToolItem";

// Transcript keeps to the bottom while you are there, and stays put when
// you have scrolled up to read.
export function Transcript({ chat }: { chat: Chat }) {
  const ref = useRef<HTMLDivElement>(null);
  const stick = useRef(true);
  const [away, setAway] = useState(false);

  useEffect(() => {
    stick.current = true;
    setAway(false);
    const el = ref.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [chat.id]);

  // Content grows from streaming, tool output opening, code blocks
  // closing; while you are at the bottom, stay there.
  useLayoutEffect(() => {
    const el = ref.current;
    const column = el?.firstElementChild;
    if (!el || !column) return;
    const follow = () => {
      if (stick.current) el.scrollTop = el.scrollHeight;
    };
    follow();
    const ro = new ResizeObserver(follow);
    ro.observe(column);
    return () => ro.disconnect();
  }, [chat.id]);

  const onScroll = () => {
    const el = ref.current!;
    const near = el.scrollHeight - el.scrollTop - el.clientHeight < 80;
    stick.current = near;
    if (near === away) setAway(!near);
  };

  const last = chat.items[chat.items.length - 1];
  const busy =
    chat.running &&
    !(last?.kind === "assistant" && last.streaming && last.text) &&
    !(last?.kind === "tool" && (last.tool?.status === "running" || last.tool?.status === "approval"));

  return (
    <>
      <div className="scroll" ref={ref} onScroll={onScroll}>
        <div className="column">
          {chat.items.map((it) => (
            <div key={it.id} style={{ display: "contents" }}>
              <ItemView item={it} chatId={chat.id} />
              {chat.turns
                .filter((t) => t.after === it.id)
                .map((t, i) => (
                  <div key={i} className="turn-stats">
                    {[duration(t.stats.ms), t.stats.tools ? plural(t.stats.tools, "tool") : "", cost(t.stats.cost)]
                      .filter(Boolean)
                      .join(" · ")}
                  </div>
                ))}
            </div>
          ))}
          {busy && <Working started={chat.started} />}
        </div>
      </div>
      {away && (
        <button
          className="to-bottom"
          title="Jump to the latest"
          onClick={() => {
            const el = ref.current!;
            stick.current = true;
            setAway(false);
            el.scrollTo({ top: el.scrollHeight, behavior: "smooth" });
          }}
        >
          <ArrowDown size={15} />
        </button>
      )}
    </>
  );
}

function ItemView({ item, chatId }: { item: Item; chatId: string }) {
  switch (item.kind) {
    case "user":
      return (
        <div className="user item">
          <div className="user-bubble">{item.text}</div>
        </div>
      );
    case "assistant":
      return <Assistant item={item} />;
    case "tool":
      return <ToolItem item={item} chatId={chatId} />;
    case "paywall":
      return <Paywall item={item} />;
    case "notice":
      return (
        <div className={`notice item${item.tone === "error" ? " error" : ""}`}>
          {item.tone === "error" ? <CircleAlert size={14} /> : <Info size={14} />}
          <span>{item.text}</span>
        </div>
      );
  }
}

const Assistant = memo(function Assistant({ item }: { item: Item }) {
  const thinking = Boolean(item.streaming && !item.text);
  return (
    <div className="assistant item">
      {item.reasoning && <Thinking text={item.reasoning} live={thinking} />}
      {item.text && <Markdown text={item.text} />}
    </div>
  );
});

function Thinking({ text, live }: { text: string; live: boolean }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="thinking">
      <button className="thinking-toggle" onClick={() => setOpen(!open)}>
        <ChevronRight size={13} className={`tool-chev${open ? " open" : ""}`} />
        <span className={live ? "shimmer" : ""}>{live ? "Thinking" : "Thought"}</span>
      </button>
      {open && <div className="thinking-body">{text.trim()}</div>}
    </div>
  );
}

// Working is the terminal client's status line, in the window: the skull
// in dots for its spinner, a rebel verb for this stretch of work, and how
// long the turn has run. Each stretch (the model thinking, or going back
// to work on a tool's result) mounts it afresh, with a new verb.
function Working({ started }: { started?: number }) {
  const [verb] = useState(pickVerb);
  const [, tick] = useState(0);
  useEffect(() => {
    const t = window.setInterval(() => tick((n) => n + 1), 1000);
    return () => window.clearInterval(t);
  }, []);
  const ms = started ? Math.max(1000, Date.now() - started) : 0;
  return (
    <div className="working" aria-label="Working">
      <SkullLoader />
      <span className="shimmer">{verb}…</span>
      {ms > 0 && <span className="working-time">{duration(Math.floor(ms / 1000) * 1000)}</span>}
    </div>
  );
}

// Paywall holds the messages above it until there is a plan that runs
// them.
function Paywall({ item }: { item: Item }) {
  const plans = useStore((s) => s.plans);
  const smallest = plans.find((p) => p.large)?.name ?? "a paid plan";
  return (
    <div className="paywall item">
      <div className="paywall-head">
        <Mark size={26} />
        <div>
          <div className="paywall-title">
            {item.text ? `${item.text} fights in a heavier class` : "Join the resistance"}
          </div>
          <div className="paywall-sub">
            {item.text
              ? `It comes with ${smallest} and up. Pick a plan and your message goes through, or switch models below.`
              : "caveira runs on a plan. Pick one and your message goes through."}
          </div>
        </div>
      </div>
      <Plans locked={item.text || undefined} held />
    </div>
  );
}
