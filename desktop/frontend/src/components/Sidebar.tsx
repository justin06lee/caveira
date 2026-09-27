import { useCallback, useEffect, useState } from "react";
import {
  Check,
  ChevronsUpDown,
  Folder,
  FolderOpen,
  FolderPlus,
  PanelLeft,
  SquarePen,
  Trash2,
  X,
  Settings2,
  GitBranch,
} from "lucide-react";
import { api } from "../lib/bridge";
import { ago } from "../lib/format";
import { useDismiss } from "../lib/hooks";
import {
  chooseProject,
  deleteChat,
  forgetProject,
  newChat,
  openChat,
  selectProject,
  setState,
  useStore,
} from "../lib/store";
import { Logo } from "./Logo";

export function Sidebar({ hidden }: { hidden: boolean }) {
  const headers = useStore((s) => s.headers);
  const chatId = useStore((s) => s.chatId);
  const chats = useStore((s) => s.chats);
  const settings = useStore((s) => s.settings);
  const [, tick] = useState(0);

  // Relative times stay roughly true.
  useEffect(() => {
    const t = window.setInterval(() => tick((n) => n + 1), 60_000);
    return () => window.clearInterval(t);
  }, []);

  const current = chatId ? chats[chatId] : undefined;
  const unsaved = current && !headers.some((h) => h.id === current.id);

  return (
    <aside className={`sidebar${hidden ? " hidden" : ""}`}>
      <div className="side-top drag">
        <button
          className="icon-btn no-drag"
          title="Hide sidebar (⌘\)"
          onClick={() => setState(() => ({ sidebar: false }))}
        >
          <PanelLeft size={16} />
        </button>
      </div>

      <div className="brand">
        <Logo size={24} />
        <span className="brand-name">caveira</span>
      </div>

      <ProjectPicker />

      <button className="new-chat" onClick={() => newChat()}>
        <SquarePen size={15} />
        New chat
        <kbd>⌘N</kbd>
      </button>

      <div className="side-label">Chats</div>
      <div className="chat-list">
        {unsaved && (
          <div className="chat-row current">
            <span className="chat-title">{firstLine(current.items.find((i) => i.kind === "user")?.text) || "New chat"}</span>
            {current.running && <span className="dot" />}
          </div>
        )}
        {headers.map((h) => (
          <ChatRow
            key={h.id}
            id={h.id}
            title={h.title || "Untitled"}
            when={ago(h.updated)}
            current={h.id === chatId}
            running={chats[h.id]?.running ?? h.running}
          />
        ))}
        {headers.length === 0 && !unsaved && <div className="side-empty">No chats here yet.</div>}
      </div>

      <div className="side-bottom">
        <button className="settings-btn" onClick={() => setState(() => ({ settingsOpen: true }))}>
          <Settings2 size={15} />
          Settings
          <span className="provider">{settings?.local ? "local model" : ""}</span>
        </button>
      </div>
    </aside>
  );
}

function firstLine(s?: string): string {
  return (s ?? "").trim().split("\n")[0].slice(0, 80);
}

function ChatRow(props: { id: string; title: string; when: string; current: boolean; running: boolean }) {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const t = window.setTimeout(() => setArmed(false), 2500);
    return () => window.clearTimeout(t);
  }, [armed]);

  return (
    <button
      className={`chat-row${props.current ? " current" : ""}`}
      onClick={() => openChat(props.id)}
      onMouseLeave={() => setArmed(false)}
      title={props.title}
    >
      <span className="chat-title">{props.title}</span>
      {props.running ? <span className="dot" /> : <span className="chat-when">{props.when}</span>}
      {!props.running && (
        <span
          role="button"
          className={`del${armed ? " armed" : ""}`}
          title={armed ? "Click again to delete" : "Delete"}
          onClick={(e) => {
            e.stopPropagation();
            if (armed) deleteChat(props.id);
            else setArmed(true);
          }}
        >
          <Trash2 size={13} />
        </span>
      )}
    </button>
  );
}

function ProjectPicker() {
  const project = useStore((s) => s.project);
  const projects = useStore((s) => s.projects);
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  const ref = useDismiss<HTMLDivElement>(open, close);

  if (!project) return null;
  return (
    <div ref={ref} style={{ position: "relative" }}>
      <button className={`project-btn${open ? " open" : ""}`} onClick={() => setOpen(!open)}>
        <Folder size={16} className="folder" />
        <span className="project-lines">
          <div className="project-name">{project.name}</div>
          <div className="project-meta">
            {project.branch ? (
              <>
                <GitBranch size={10} style={{ verticalAlign: "-1px", marginRight: 3 }} />
                {project.branch}
              </>
            ) : (
              project.short
            )}
          </div>
        </span>
        <ChevronsUpDown size={14} className="chev" />
      </button>
      {open && (
        <div className="menu" style={{ top: "calc(100% + 4px)", left: 8, right: 8 }}>
          <div className="menu-label">Projects</div>
          {projects.map((p) => (
            <button
              key={p.path}
              className="menu-item"
              onClick={() => {
                close();
                if (p.path !== project.path) selectProject(p);
              }}
            >
              <span className="grow">
                {p.name}
                <span className="sub">{p.short}</span>
              </span>
              {p.path === project.path ? (
                <Check size={14} className="check" />
              ) : (
                <span
                  role="button"
                  className="icon-btn"
                  style={{ width: 22, height: 22 }}
                  title="Remove from this list"
                  onClick={(e) => {
                    e.stopPropagation();
                    forgetProject(p);
                  }}
                >
                  <X size={12} />
                </span>
              )}
            </button>
          ))}
          <div className="menu-sep" />
          <button
            className="menu-item"
            onClick={() => {
              close();
              chooseProject();
            }}
          >
            <FolderPlus size={15} className="muted-icon" />
            <span className="grow">Open folder…</span>
            <kbd>⌘O</kbd>
          </button>
          <button
            className="menu-item"
            onClick={() => {
              close();
              api.reveal(project.path);
            }}
          >
            <FolderOpen size={15} className="muted-icon" />
            <span className="grow">Show in Finder</span>
          </button>
        </div>
      )}
    </div>
  );
}
