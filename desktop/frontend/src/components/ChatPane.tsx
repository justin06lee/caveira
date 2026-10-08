import { Folder, GitBranch, PanelLeft, TriangleAlert } from "lucide-react";
import { shortcut } from "../lib/platform";
import { setState, useStore, type Chat } from "../lib/store";
import { Composer } from "./Composer";
import { greeting } from "../lib/rebel";
import { Mark } from "./Logo";
import { Transcript } from "./Transcript";

// HERO_MARK is the eye's width over an empty chat.
const HERO_MARK = 76;

export function ChatPane() {
  const chat = useStore((s) => (s.chatId ? s.chats[s.chatId] : undefined));
  const project = useStore((s) => s.project);
  const sidebar = useStore((s) => s.sidebar);

  const title = chat && chat.items.length > 0 ? chat.title || firstUser(chat) : "";

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
        <div className="header-title">{title}</div>
        <span className="header-spacer" />
      </header>

      {!chat ? (
        // Holds the empty chat's shape while it loads, so nothing jumps.
        <div className="hero" aria-busy="true">
          <Mark size={HERO_MARK} />
          <h1 style={{ visibility: "hidden" }}>·</h1>
          <p className="sub" style={{ visibility: "hidden" }}>
            ·
          </p>
          <div style={{ height: 92 }} />
        </div>
      ) : chat.items.length === 0 ? (
        <div className="hero">
          <Mark size={HERO_MARK} />
          <h1>{greeting(chat.id)}</h1>
          <p className="sub">
            <Folder size={13} /> {project?.name}
            {project?.branch && (
              <>
                <GitBranch size={13} style={{ marginLeft: 6 }} /> {project.branch}
              </>
            )}
          </p>
          {chat.problem && <Problem chat={chat} />}
          <Composer chat={chat} />
        </div>
      ) : (
        <>
          <Transcript chat={chat} />
          <div className="dock">
            {chat.problem && <Problem chat={chat} />}
            <Composer chat={chat} />
          </div>
        </>
      )}
    </>
  );
}

function firstUser(chat: Chat): string {
  return chat.items.find((i) => i.kind === "user")?.text?.split("\n")[0] ?? "";
}

function Problem({ chat }: { chat: Chat }) {
  return (
    <div className="banner" style={{ width: "100%", maxWidth: 740 }}>
      <TriangleAlert size={15} style={{ color: "var(--amber)", flex: "none" }} />
      <span className="grow">{chat.problem}</span>
      <button className="btn" onClick={() => setState(() => ({ settingsOpen: true }))}>
        Settings
      </button>
    </div>
  );
}
