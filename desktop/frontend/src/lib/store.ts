// One small store for the whole window. Go is the source of truth for
// transcripts; this holds what has been fetched and folds in the events
// that arrive while turns run, a frame at a time.

import { useSyncExternalStore } from "react";
import { api, errorText, onChatEvent } from "./bridge";
import type {
  ChatEvent,
  ChatHeader,
  ChatView,
  Item,
  Project,
  SettingsInput,
  SettingsView,
  TurnStats,
  Workspace,
} from "./types";

export interface Chat extends ChatView {
  // turns closes each finished turn with its stats, after the item that
  // ended it.
  turns: { after: string; stats: TurnStats }[];
}

export interface State {
  ready: boolean;
  version: string;
  settings: SettingsView | null;
  projects: Project[];
  // project is where the open chat is, and where ⌘N (Ctrl+N) starts one.
  project: Project | null;
  // headers is every project's saved chats, newest first; the sidebar
  // sorts them into its folders by dir.
  headers: ChatHeader[];
  // expanded is the project folders open in the sidebar, by path.
  expanded: string[];
  chatId: string | null;
  chats: Record<string, Chat>;
  settingsOpen: boolean;
  sidebar: boolean;
  toast: string | null;
  // onboarded is false until the first-run steps are done or skipped.
  onboarded: boolean;
  // workspace is where the project picker opens; empty means home.
  workspace: Workspace;
  pickerOpen: boolean;
  importOpen: boolean;
}

let state: State = {
  ready: false,
  version: "",
  settings: null,
  projects: [],
  project: null,
  headers: [],
  expanded: [],
  chatId: null,
  chats: {},
  settingsOpen: false,
  sidebar: true,
  toast: null,
  onboarded: true,
  workspace: { path: "", short: "" },
  pickerOpen: false,
  importOpen: false,
};

const listeners = new Set<() => void>();

export function getState(): State {
  return state;
}

export function setState(fn: (s: State) => Partial<State>): void {
  state = { ...state, ...fn(state) };
  listeners.forEach((l) => l());
}

export function useStore<T>(select: (s: State) => T): T {
  return useSyncExternalStore(
    (l) => {
      listeners.add(l);
      return () => listeners.delete(l);
    },
    () => select(state),
  );
}

// inputFrom is a settings change that changes nothing, to start from.
export function inputFrom(s: SettingsView): SettingsInput {
  return {
    local: s.local,
    apiKey: null,
    baseUrl: s.baseUrl,
    model: s.model,
    effort: s.effort,
    confirm: s.confirm,
    localBaseUrl: s.localBaseUrl,
    localModel: s.localModel,
    theme: s.theme,
  };
}

let toastTimer: number | undefined;

export function toast(e: unknown): void {
  const text = errorText(e);
  setState(() => ({ toast: text }));
  window.clearTimeout(toastTimer);
  toastTimer = window.setTimeout(() => setState(() => ({ toast: null })), 6000);
}

const LAST_PROJECT = "caveira.project";
const LAST_CHAT = "caveira.chat";
const EXPANDED = "caveira.expanded";

function loadExpanded(): string[] | null {
  try {
    const v = JSON.parse(localStorage.getItem(EXPANDED) ?? "null");
    return Array.isArray(v) ? v.filter((x) => typeof x === "string") : null;
  } catch {
    return null;
  }
}

function saveExpanded(expanded: string[]): void {
  localStorage.setItem(EXPANDED, JSON.stringify(expanded));
}

// toggleFolder opens or closes a project folder in the sidebar.
export function toggleFolder(path: string): void {
  setState((s) => ({
    expanded: s.expanded.includes(path) ? s.expanded.filter((x) => x !== path) : [...s.expanded, path],
  }));
  saveExpanded(getState().expanded);
}

// expand opens the folder the open chat is in, however it got opened.
function expand(path: string): void {
  if (getState().expanded.includes(path)) return;
  setState((s) => ({ expanded: [...s.expanded, path] }));
  saveExpanded(getState().expanded);
}

// boot comes back to the project and chat the window last showed. The
// project is in place before the first frame, so the window does not
// flash the welcome screen on its way there.
export async function boot(): Promise<void> {
  const b = await api.boot();
  const last = localStorage.getItem(LAST_PROJECT);
  const project = b.projects.find((p) => p.path === last) ?? b.projects[0] ?? null;
  const headers = (await api.chats("").catch(() => null)) ?? [];
  setState(() => ({
    ready: true,
    version: b.version,
    settings: b.settings,
    projects: b.projects,
    project,
    headers,
    expanded: loadExpanded() ?? [],
    onboarded: b.onboarded,
    workspace: b.workspace,
  }));
  if (project) await selectProject(project, localStorage.getItem(LAST_CHAT) ?? undefined);
}

// openPath opens the folder at path as the project, from the picker.
export async function openPath(path: string): Promise<void> {
  setState(() => ({ pickerOpen: false }));
  await selectProject({ path, name: "", short: "", branch: "" });
}

// makeProject creates a folder under base and opens it.
export async function makeProject(base: string, rel: string): Promise<void> {
  try {
    const p = await api.makeFolder(base, rel);
    setState(() => ({ pickerOpen: false }));
    await selectProject(p);
  } catch (e) {
    toast(e);
  }
}

export async function refreshProjects(): Promise<void> {
  const projects = (await api.projects()) ?? [];
  setState(() => ({ projects }));
}

// selectProject switches to p and opens a fresh chat there, or the chat
// resume names if it belongs to p.
export async function selectProject(p: Project, resume?: string): Promise<void> {
  try {
    await enter(p.path);
    const s = getState();
    if (resume && s.headers.some((h) => h.id === resume && h.dir === s.project?.path)) await openChat(resume);
    else await newChat();
  } catch (e) {
    toast(e);
  }
}

function freshen(p: Project): void {
  setState((s) => ({
    projects: s.projects.map((x) => (x.path === p.path ? p : x)),
    project: s.project?.path === p.path ? p : s.project,
  }));
}

// enter makes dir the project, opening its folder in the sidebar. A
// project already there keeps its place, so the folders do not shuffle
// under the pointer; a new one goes on top.
async function enter(dir: string): Promise<void> {
  const fresh = await api.openProject(dir);
  localStorage.setItem(LAST_PROJECT, fresh.path);
  const known = getState().projects.some((x) => x.path === fresh.path);
  setState((s) => ({
    project: fresh,
    projects: known ? s.projects.map((x) => (x.path === fresh.path ? fresh : x)) : [fresh, ...s.projects],
  }));
  expand(fresh.path);
  if (!known) await refreshHeaders();
}

export async function chooseProject(): Promise<void> {
  try {
    const p = await api.chooseProject();
    if (p.path) {
      setState(() => ({ pickerOpen: false }));
      await selectProject(p);
    }
  } catch (e) {
    toast(e);
  }
}

// forgetProject takes p out of the sidebar. Its folder and its chats stay
// where they are; opening it again brings them back.
export async function forgetProject(p: Project): Promise<void> {
  try {
    await api.forgetProject(p.path);
  } catch (e) {
    toast(e);
    return;
  }
  setState((s) => ({
    projects: s.projects.filter((x) => x.path !== p.path),
    expanded: s.expanded.filter((x) => x !== p.path),
  }));
  saveExpanded(getState().expanded);
  const s = getState();
  if (s.project?.path === p.path) {
    localStorage.removeItem(LAST_PROJECT);
    setState(() => ({ project: null, chatId: null }));
    if (s.projects[0]) await selectProject(s.projects[0]);
  }
}

function withTurns(v: ChatView, old?: Chat): Chat {
  return { ...v, items: v.items ?? [], turns: old?.turns ?? [] };
}

// newChat starts a chat in the project at dir, or in the current one.
export async function newChat(dir?: string): Promise<void> {
  try {
    if (dir && dir !== getState().project?.path) await enter(dir);
    const p = getState().project;
    if (!p) return;
    setState(() => ({ chatId: null }));
    const v = await api.newChat(p.path);
    localStorage.setItem(LAST_CHAT, v.id);
    setState((s) => ({ chatId: v.id, chats: { ...s.chats, [v.id]: withTurns(v, s.chats[v.id]) } }));
  } catch (e) {
    toast(e);
  }
}

// openChat opens a saved chat, switching to its project if it is in
// another one.
export async function openChat(id: string): Promise<void> {
  localStorage.setItem(LAST_CHAT, id);
  const s = getState();
  const dir = s.headers.find((h) => h.id === id)?.dir ?? s.chats[id]?.dir;
  const owner = dir && dir !== s.project?.path ? s.projects.find((p) => p.path === dir) : undefined;
  if (owner) {
    localStorage.setItem(LAST_PROJECT, owner.path);
    setState(() => ({ project: owner }));
    // Read again behind it: the branch may have moved since the list was.
    api.openProject(owner.path).then(freshen, () => {});
  }
  if (dir) expand(dir);
  setState(() => ({ chatId: id }));
  try {
    const v = await api.openChat(id);
    setState((s) => ({ chats: { ...s.chats, [id]: withTurns(v, s.chats[id]) } }));
  } catch (e) {
    toast(e);
  }
}

export async function refreshChat(id: string): Promise<void> {
  const v = await api.openChat(id);
  setState((s) => ({ chats: { ...s.chats, [id]: withTurns(v, s.chats[id]) } }));
}

export async function refreshHeaders(): Promise<void> {
  const headers = (await api.chats("")) ?? [];
  setState(() => ({ headers }));
}

export async function deleteChat(id: string): Promise<void> {
  try {
    await api.deleteChat(id);
    setState((s) => {
      const chats = { ...s.chats };
      delete chats[id];
      return { chats, headers: s.headers.filter((h) => h.id !== id) };
    });
    if (getState().chatId === id) {
      const dir = getState().project?.path;
      const next = getState().headers.find((h) => h.dir === dir);
      if (next) await openChat(next.id);
      else await newChat();
    }
  } catch (e) {
    toast(e);
  }
}

export async function send(text: string): Promise<boolean> {
  const id = getState().chatId;
  if (!id) return false;
  try {
    await api.send(id, text);
    patchChat(id, (c) => ({ ...c, running: true }));
    return true;
  } catch (e) {
    toast(e);
    return false;
  }
}

export function patchChat(id: string, fn: (c: Chat) => Chat): void {
  setState((s) => (s.chats[id] ? { chats: { ...s.chats, [id]: fn(s.chats[id]) } } : {}));
}

// Events arrive per token; they are folded in once per frame.
let queue: ChatEvent[] = [];
let scheduled = false;

export function listen(): () => void {
  return onChatEvent((e) => {
    queue.push(e);
    if (!scheduled) {
      scheduled = true;
      requestAnimationFrame(flush);
    }
  });
}

function flush() {
  scheduled = false;
  const events = queue;
  queue = [];
  let finished = false;
  setState((s) => {
    const chats = { ...s.chats };
    for (const e of events) {
      const c = chats[e.chat];
      if (!c) continue;
      chats[e.chat] = fold(c, e);
      if (e.type === "done") finished = true;
    }
    return { chats };
  });
  if (finished) refreshHeaders().catch(() => {});
}

function fold(c: Chat, e: ChatEvent): Chat {
  switch (e.type) {
    case "item": {
      const it = e.item!;
      const i = c.items.findIndex((x) => x.id === it.id);
      const items = i < 0 ? [...c.items, it] : c.items.map((x, j) => (j === i ? it : x));
      return { ...c, items, running: true };
    }
    case "delta":
      return {
        ...c,
        items: c.items.map((x): Item =>
          x.id === e.id
            ? { ...x, text: (x.text ?? "") + (e.text ?? ""), reasoning: (x.reasoning ?? "") + (e.reasoning ?? "") }
            : x,
        ),
      };
    case "remove":
      return { ...c, items: c.items.filter((x) => x.id !== e.id) };
    case "usage":
      return { ...c, context: e.context ?? c.context, cost: e.cost ?? c.cost };
    case "model":
      return { ...c, model: e.model ?? c.model, window: e.window ?? c.window };
    case "done": {
      const last = c.items[c.items.length - 1];
      const turns = last && e.stats ? [...c.turns, { after: last.id, stats: e.stats }] : c.turns;
      return {
        ...c,
        running: false,
        title: e.title || c.title,
        items: c.items.map((x) => (x.streaming ? { ...x, streaming: false } : x)),
        turns,
      };
    }
  }
  return c;
}
