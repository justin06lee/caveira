import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import {
  ChevronRight,
  Folder,
  FolderOpen,
  FolderPlus,
  FolderSearch,
  PanelLeft,
  Plus,
  Settings2,
  SquarePen,
  Trash2,
  X,
} from "lucide-react";
import { api } from "../lib/bridge";
import { ago } from "../lib/format";
import { useDismiss } from "../lib/hooks";
import {
  deleteChat,
  forgetProject,
  newChat,
  openChat,
  setState,
  toggleFolder,
  useStore,
  type State,
} from "../lib/store";
import type { ChatHeader, Project } from "../lib/types";
import { Logo } from "./Logo";

// PEEK is how many chats an open folder shows before "Show more".
const PEEK = 6;

// The sidebar is the projects as folders, each holding its chats, so any
// chat in any project is one click away.
export function Sidebar({ hidden }: { hidden: boolean }) {
  const projects = useStore((s) => s.projects);
  const headers = useStore((s) => s.headers);
  const expanded = useStore((s) => s.expanded);
  const chatId = useStore((s) => s.chatId);
  const here = useStore((s) => s.project?.path);
  const draftKey = useStore(draftsOf);
  const live = useStore(liveKey);
  const local = useStore((s) => s.settings?.local);
  const [, tick] = useState(0);

  // Relative times stay roughly true.
  useEffect(() => {
    const t = window.setInterval(() => tick((n) => n + 1), 60_000);
    return () => window.clearInterval(t);
  }, []);

  const byDir = useMemo(() => {
    const m = new Map<string, ChatHeader[]>();
    for (const h of headers) {
      const list = m.get(h.dir);
      if (list) list.push(h);
      else m.set(h.dir, [h]);
    }
    return m;
  }, [headers]);

  const drafts = useMemo(() => {
    const m = new Map<string, string[]>();
    for (const line of draftKey.split("\n").filter(Boolean)) {
      const [dir, id] = line.split("\t");
      m.set(dir, [...(m.get(dir) ?? []), id]);
    }
    return m;
  }, [draftKey]);

  // A chat open in this window is running if the window says so; one it
  // has not opened, if the list said so when it was read.
  const running = useMemo(() => {
    const known = new Map(
      live
        .split(" ")
        .filter(Boolean)
        .map((x) => [x.slice(0, -2), x.endsWith(":1")] as const),
    );
    return (id: string, listed = false) => known.get(id) ?? listed;
  }, [live]);

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

      <button className="new-chat" onClick={() => newChat()}>
        <SquarePen size={15} />
        New chat
        <kbd>⌘N</kbd>
      </button>

      <div className="side-label">
        <span>Projects</span>
        <button
          className="icon-btn"
          title="Open a project (⌘O)"
          onClick={() => setState(() => ({ pickerOpen: true }))}
        >
          <FolderPlus size={14} />
        </button>
      </div>
      <div className="folders">
        {projects.map((p) => (
          <ProjectFolder
            key={p.path}
            project={p}
            chats={byDir.get(p.path) ?? []}
            open={expanded.includes(p.path)}
            here={p.path === here}
            chatId={chatId}
            drafts={drafts.get(p.path) ?? []}
            running={running}
          />
        ))}
        {projects.length === 0 && <div className="side-empty">No projects yet.</div>}
      </div>

      <div className="side-bottom">
        <button className="settings-btn" onClick={() => setState(() => ({ settingsOpen: true }))}>
          <Settings2 size={15} />
          Settings
          <span className="provider">{local ? "local model" : ""}</span>
        </button>
      </div>
    </aside>
  );
}

// draftsOf is the chats open in this window that are not saved yet, as
// "dir\tid" lines: the open one before anything is sent, and any whose
// first turn is still going (a chat is saved when a turn ends).
function draftsOf(s: State): string {
  const saved = new Set(s.headers.map((h) => h.id));
  return Object.values(s.chats)
    .filter((c) => !saved.has(c.id) && (c.id === s.chatId || c.items.length > 0))
    .map((c) => `${c.dir}\t${c.id}`)
    .join("\n");
}

// liveKey says which chats open in this window are running, as a string,
// so the sidebar draws again when one starts or stops rather than on
// every token.
function liveKey(s: State): string {
  return Object.values(s.chats)
    .map((c) => `${c.id}:${c.running ? 1 : 0}`)
    .join(" ");
}

function ProjectFolder(props: {
  project: Project;
  chats: ChatHeader[];
  open: boolean;
  here: boolean;
  chatId: string | null;
  drafts: string[];
  running: (id: string, listed?: boolean) => boolean;
}) {
  const { project, chats, drafts, open, chatId, running } = props;
  const [all, setAll] = useState(false);
  const [armed, setArmed] = useArmed();
  const [menu, setMenu] = useState<{ x: number; y: number } | null>(null);

  // Folded, the folder says when a chat inside it is working.
  const busy = !open && (drafts.some((id) => running(id)) || chats.some((h) => running(h.id, h.running)));

  // The open chat is always on show, and so is everything above it.
  const at = chats.findIndex((h) => h.id === chatId);
  const shown = all ? chats : chats.slice(0, Math.max(PEEK, at + 1));
  const more = chats.length - shown.length;

  return (
    <div className="folder">
      <button
        className={`folder-row${props.here ? " here" : ""}`}
        onClick={() => toggleFolder(project.path)}
        onContextMenu={(e) => {
          e.preventDefault();
          setMenu({ x: e.clientX, y: e.clientY });
        }}
        onMouseLeave={() => setArmed(false)}
        title={project.branch ? `${project.short} · ${project.branch}` : project.short}
      >
        <ChevronRight size={12} className={`chev${open ? " open" : ""}`} />
        {open ? <FolderOpen size={15} className="folder-icon" /> : <Folder size={15} className="folder-icon" />}
        <span className="folder-name">{project.name}</span>
        {busy && <span className="dot" />}
        <span className="folder-actions">
          <span
            role="button"
            className="act"
            title="New chat here"
            onClick={(e) => {
              e.stopPropagation();
              newChat(project.path);
            }}
          >
            <Plus size={14} />
          </span>
          <span
            role="button"
            className={`act${armed ? " armed" : ""}`}
            title={armed ? "Click again to remove it" : "Remove from the sidebar (the folder and its chats stay)"}
            onClick={(e) => {
              e.stopPropagation();
              if (armed) forgetProject(project);
              else setArmed(true);
            }}
          >
            <X size={13} />
          </span>
        </span>
      </button>

      {open && (
        <div className="folder-chats">
          {drafts.map((id) => (
            <DraftRow key={id} id={id} current={id === chatId} />
          ))}
          {shown.map((h) => (
            <ChatRow
              key={h.id}
              id={h.id}
              title={h.title || "Untitled"}
              when={ago(h.updated)}
              current={h.id === chatId}
              running={running(h.id, h.running)}
            />
          ))}
          {more > 0 && (
            <button className="more-row" onClick={() => setAll(true)}>
              Show {more} more
            </button>
          )}
          {all && chats.length > PEEK && (
            <button className="more-row" onClick={() => setAll(false)}>
              Show less
            </button>
          )}
          {chats.length === 0 && drafts.length === 0 && <div className="folder-empty">No chats yet</div>}
        </div>
      )}

      {menu && <FolderMenu project={project} at={menu} onClose={() => setMenu(null)} />}
    </div>
  );
}

// FolderMenu is a folder's right-click menu.
function FolderMenu(props: { project: Project; at: { x: number; y: number }; onClose: () => void }) {
  const { project, onClose } = props;
  const ref = useDismiss<HTMLDivElement>(true, onClose);
  const [pos, setPos] = useState(props.at);

  // Kept inside the window.
  useLayoutEffect(() => {
    const r = ref.current?.getBoundingClientRect();
    if (!r) return;
    setPos({
      x: Math.min(props.at.x, window.innerWidth - r.width - 8),
      y: Math.min(props.at.y, window.innerHeight - r.height - 8),
    });
  }, [props.at, ref]);

  return (
    <div ref={ref} className="menu" style={{ position: "fixed", left: pos.x, top: pos.y, minWidth: 210 }}>
      <button
        className="menu-item"
        onClick={() => {
          onClose();
          newChat(project.path);
        }}
      >
        <SquarePen size={15} className="muted-icon" />
        <span className="grow">New chat</span>
      </button>
      <button
        className="menu-item"
        onClick={() => {
          onClose();
          api.reveal(project.path);
        }}
      >
        <FolderSearch size={15} className="muted-icon" />
        <span className="grow">Show in Finder</span>
      </button>
      <div className="menu-sep" />
      <button
        className="menu-item"
        onClick={() => {
          onClose();
          forgetProject(project);
        }}
      >
        <X size={15} className="muted-icon" />
        <span className="grow">
          Remove from sidebar
          <span className="sub">The folder and its chats stay</span>
        </span>
      </button>
    </div>
  );
}

// DraftRow is a chat not saved yet: new, or in its first turn.
function DraftRow(props: { id: string; current: boolean }) {
  const title = useStore((s) => firstLine(s.chats[props.id]?.items.find((i) => i.kind === "user")?.text));
  const running = useStore((s) => s.chats[props.id]?.running ?? false);
  const ref = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (props.current) ref.current?.scrollIntoView({ block: "nearest" });
  }, [props.current]);
  return (
    <button
      ref={ref}
      className={`chat-row${props.current ? " current" : ""}`}
      onClick={() => openChat(props.id)}
      title={title || "New chat"}
    >
      <span className="chat-title">{title || "New chat"}</span>
      {running && <span className="dot" />}
    </button>
  );
}

function firstLine(s?: string): string {
  return (s ?? "").trim().split("\n")[0].slice(0, 80);
}

// useArmed is a delete that asks for a second click, forgetting the
// first after a moment.
function useArmed(): [boolean, (v: boolean) => void] {
  const [armed, setArmed] = useState(false);
  useEffect(() => {
    if (!armed) return;
    const t = window.setTimeout(() => setArmed(false), 2500);
    return () => window.clearTimeout(t);
  }, [armed]);
  return [armed, useCallback((v: boolean) => setArmed(v), [])];
}

function ChatRow(props: { id: string; title: string; when: string; current: boolean; running: boolean }) {
  const [armed, setArmed] = useArmed();
  const ref = useRef<HTMLButtonElement>(null);

  // The open chat is kept in view, however it was opened.
  useEffect(() => {
    if (props.current) ref.current?.scrollIntoView({ block: "nearest" });
  }, [props.current]);

  return (
    <button
      ref={ref}
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
