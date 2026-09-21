import { useSyncExternalStore } from "react";
import { compose, type Grid } from "./lib/compose";
import {
  newDoc,
  sanitizeDoc,
  uid,
  type BorderKind,
  type Doc,
  type Element,
  type LineKind,
  type PaintCell,
} from "./lib/model";
import { starterDoc } from "./starter";

export type Tool = "select" | "box" | "text" | "line" | "pencil" | "eraser" | "fill" | "picker";

export type SaveState = "idle" | "dirty" | "saving" | "saved" | "error";

export interface Marquee {
  x0: number;
  y0: number;
  x1: number;
  y1: number;
}

export interface State {
  doc: Doc;
  selected: string[];
  tool: Tool;
  brush: PaintCell;
  brushSize: number;
  recentChars: string[];
  boxBorder: BorderKind;
  lineKind: LineKind;
  editingId: string | null;
  hover: { x: number; y: number } | null;
  marquee: Marquee | null;
  dragging: boolean;
  themeId: string;
  fontSize: number;
  fontFamily: string;
  grid: boolean;
  designs: { name: string; updatedAt: number }[];
  dir: string;
  save: SaveState;
  undo: Doc[];
  redo: Doc[];
  toast: string | null;
  loaded: boolean;
}

const PREFS_KEY = "caveira-cells:prefs";
const DEFAULT_FONT = 'Menlo, "SF Mono", Monaco, "JetBrains Mono", "DejaVu Sans Mono", monospace';

function loadPrefs(): Partial<State> {
  try {
    return JSON.parse(localStorage.getItem(PREFS_KEY) ?? "{}");
  } catch {
    return {};
  }
}

const prefs = loadPrefs();

let state: State = {
  doc: newDoc("untitled"),
  selected: [],
  tool: "select",
  brush: { ch: "█", fg: "#D0463B", bg: "" },
  brushSize: 1,
  recentChars: ["█", "▓", "▒", "░", "─", "│", "╭", "╮", "╰", "╯", "•", "▚"],
  boxBorder: "rounded",
  lineKind: "light",
  editingId: null,
  hover: null,
  marquee: null,
  dragging: false,
  themeId: "caveira",
  fontSize: 13,
  fontFamily: DEFAULT_FONT,
  grid: false,
  designs: [],
  dir: "",
  save: "idle",
  undo: [],
  redo: [],
  toast: null,
  loaded: false,
  ...pick(prefs, ["brush", "brushSize", "recentChars", "boxBorder", "lineKind", "themeId", "fontSize", "fontFamily", "grid"]),
};

function pick<T extends object>(o: Partial<T>, keys: (keyof T)[]): Partial<T> {
  const out: Partial<T> = {};
  for (const k of keys) if (o[k] !== undefined) out[k] = o[k];
  return out;
}

const listeners = new Set<() => void>();

export function get(): State {
  return state;
}

// Every document change is autosaved, except when `quiet` says the new doc
// just came from disk.
export function set(patch: Partial<State> | ((s: State) => Partial<State>), quiet = false) {
  const p = typeof patch === "function" ? patch(state) : patch;
  const prevDoc = state.doc;
  state = { ...state, ...p };
  if (state.doc !== prevDoc && state.loaded && !quiet) scheduleSave();
  for (const l of listeners) l();
}

function subscribe(l: () => void) {
  listeners.add(l);
  return () => listeners.delete(l);
}

export function useEditor(): State {
  return useSyncExternalStore(subscribe, get);
}

subscribe(() => {
  const s = state;
  const keep = pick(s, ["brush", "brushSize", "recentChars", "boxBorder", "lineKind", "themeId", "fontSize", "fontFamily", "grid"]);
  localStorage.setItem(PREFS_KEY, JSON.stringify(keep));
});

// ---- the composed grid, cached per document ------------------------------

let gridCache: { doc: Doc; grid: Grid } | null = null;

export function gridOf(doc: Doc): Grid {
  if (gridCache?.doc !== doc) gridCache = { doc, grid: compose(doc) };
  return gridCache.grid;
}

// ---- document edits and history ------------------------------------------

const HISTORY = 200;
let coalesce: { key: string; at: number } | null = null;

// Applies an edit to the document. `history` decides whether the state
// before it becomes an undo step: "push" always, "skip" never (for edits in
// the middle of a drag), or a key that merges bursts of the same edit.
export function edit(fn: (doc: Doc) => Doc, history: "push" | "skip" | { key: string } = "push") {
  const before = state.doc;
  const after = fn(before);
  if (after === before) return;
  let push = history === "push";
  if (typeof history === "object") {
    const now = Date.now();
    push = !(coalesce && coalesce.key === history.key && now - coalesce.at < 1200);
    coalesce = { key: history.key, at: now };
  } else coalesce = null;
  set({ doc: after, ...(push ? { undo: [...state.undo, before].slice(-HISTORY), redo: [] } : {}) });
}

// Records a finished gesture as one undo step.
export function commitGesture(before: Doc) {
  if (state.doc === before) return;
  coalesce = null;
  set({ undo: [...state.undo, before].slice(-HISTORY), redo: [] });
}

export function undo() {
  const prev = state.undo[state.undo.length - 1];
  if (!prev) return;
  coalesce = null;
  set({ doc: prev, undo: state.undo.slice(0, -1), redo: [...state.redo, state.doc], editingId: null, selected: keepExisting(prev) });
}

export function redo() {
  const next = state.redo[state.redo.length - 1];
  if (!next) return;
  coalesce = null;
  set({ doc: next, redo: state.redo.slice(0, -1), undo: [...state.undo, state.doc], editingId: null, selected: keepExisting(next) });
}

function keepExisting(doc: Doc): string[] {
  const ids = new Set(doc.elements.map((e) => e.id));
  return state.selected.filter((id) => ids.has(id));
}

export function updateElement<T extends Element>(id: string, patch: Partial<T> | ((e: T) => Partial<T>), history: Parameters<typeof edit>[1] = "push") {
  edit((doc) => {
    const i = doc.elements.findIndex((e) => e.id === id);
    if (i < 0) return doc;
    const el = doc.elements[i] as T;
    const p = typeof patch === "function" ? patch(el) : patch;
    const elements = doc.elements.slice();
    elements[i] = { ...el, ...p } as Element;
    return { ...doc, elements };
  }, history);
}

export function selectedElements(s: State = state): Element[] {
  const ids = new Set(s.selected);
  return s.doc.elements.filter((e) => ids.has(e.id));
}

export function addElement(el: Element, history: Parameters<typeof edit>[1] = "push") {
  edit((doc) => ({ ...doc, elements: [...doc.elements, el] }), history);
}

export function removeElements(ids: string[]) {
  const drop = new Set(ids);
  edit((doc) => ({ ...doc, elements: doc.elements.filter((e) => !drop.has(e.id)) }));
  set({ selected: state.selected.filter((id) => !drop.has(id)), editingId: drop.has(state.editingId ?? "") ? null : state.editingId });
}

export function cloneElements(els: Element[], dx: number, dy: number): Element[] {
  return els.map((e) => ({ ...structuredClone(e), id: uid(), x: e.x + dx, y: e.y + dy, name: e.name.endsWith(" copy") ? e.name : `${e.name} copy` }));
}

export function duplicateSelection() {
  const els = selectedElements();
  if (!els.length) return;
  const copies = cloneElements(els, 2, 1);
  edit((doc) => ({ ...doc, elements: [...doc.elements, ...copies] }));
  set({ selected: copies.map((c) => c.id) });
}

// Moves the selection one step up or down the stack, or to either end.
export function restack(where: "up" | "down" | "top" | "bottom") {
  const ids = new Set(state.selected);
  if (!ids.size) return;
  edit((doc) => {
    const els = doc.elements.slice();
    if (where === "top" || where === "bottom") {
      const moving = els.filter((e) => ids.has(e.id));
      const rest = els.filter((e) => !ids.has(e.id));
      return { ...doc, elements: where === "top" ? [...rest, ...moving] : [...moving, ...rest] };
    }
    if (where === "up") {
      for (let i = els.length - 2; i >= 0; i--) {
        if (ids.has(els[i]!.id) && !ids.has(els[i + 1]!.id)) [els[i], els[i + 1]] = [els[i + 1]!, els[i]!];
      }
    } else {
      for (let i = 1; i < els.length; i++) {
        if (ids.has(els[i]!.id) && !ids.has(els[i - 1]!.id)) [els[i], els[i - 1]] = [els[i - 1]!, els[i]!];
      }
    }
    return { ...doc, elements: els };
  });
}

export function moveElementTo(id: string, index: number) {
  edit((doc) => {
    const els = doc.elements.slice();
    const from = els.findIndex((e) => e.id === id);
    if (from < 0) return doc;
    const [el] = els.splice(from, 1);
    els.splice(Math.max(0, Math.min(els.length, index)), 0, el!);
    return { ...doc, elements: els };
  });
}

export function nudge(dx: number, dy: number) {
  const ids = new Set(state.selected);
  if (!ids.size) return;
  edit(
    (doc) => ({ ...doc, elements: doc.elements.map((e) => (ids.has(e.id) && !e.locked ? { ...e, x: e.x + dx, y: e.y + dy } : e)) }),
    { key: "nudge:" + state.selected.join(",") },
  );
}

export function pickChar(ch: string) {
  set((s) => ({
    brush: { ...s.brush, ch },
    recentChars: [ch, ...s.recentChars.filter((c) => c !== ch)].slice(0, 16),
  }));
}

let toastTimer: ReturnType<typeof setTimeout> | undefined;
export function toast(msg: string) {
  clearTimeout(toastTimer);
  set({ toast: msg });
  toastTimer = setTimeout(() => set({ toast: null }), 1800);
}

// ---- persistence ---------------------------------------------------------

let saveTimer: ReturnType<typeof setTimeout> | undefined;
let saving: Promise<void> = Promise.resolve();

function scheduleSave() {
  clearTimeout(saveTimer);
  if (state.save !== "dirty") queueMicrotask(() => set({ save: "dirty" }));
  saveTimer = setTimeout(flushSave, 400);
}

export async function flushSave() {
  clearTimeout(saveTimer);
  const doc = state.doc;
  saving = saving.then(async () => {
    set({ save: "saving" });
    try {
      const r = await fetch(`/api/designs/${encodeURIComponent(doc.name)}`, {
        method: "PUT",
        headers: { "content-type": "application/json" },
        body: JSON.stringify(doc),
      });
      if (!r.ok) throw new Error(await r.text());
      set({ save: state.doc === doc ? "saved" : "dirty" });
    } catch (err) {
      console.error(err);
      set({ save: "error" });
    }
  });
  await saving;
}

async function refreshList() {
  const r = await fetch("/api/designs");
  const j = (await r.json()) as { dir: string; designs: State["designs"] };
  set({ designs: j.designs, dir: j.dir });
  return j.designs;
}

const LAST_KEY = "caveira-cells:last";

export async function openDesign(name: string) {
  if (state.loaded && state.save !== "saved" && state.save !== "idle") await flushSave();
  const r = await fetch(`/api/designs/${encodeURIComponent(name)}`);
  if (!r.ok) return toast(`couldn't open ${name}`);
  const doc = sanitizeDoc(await r.json(), name);
  localStorage.setItem(LAST_KEY, name);
  set({ doc, selected: [], editingId: null, undo: [], redo: [], save: "saved", loaded: true }, true);
}

export function slug(s: string): string {
  return s
    .toLowerCase()
    .trim()
    .replace(/[^a-z0-9_-]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 64);
}

export async function createDesign(raw: string, from?: Doc) {
  const base = slug(raw) || "untitled";
  const taken = new Set(state.designs.map((d) => d.name));
  let name = base;
  for (let i = 2; taken.has(name); i++) name = `${base}-${i}`;
  if (state.loaded) await flushSave();
  const doc: Doc = from ? { ...structuredClone(from), name } : newDoc(name);
  localStorage.setItem(LAST_KEY, name);
  set({ doc, selected: [], editingId: null, undo: [], redo: [], loaded: true });
  await flushSave();
  await refreshList();
}

export async function renameDesign(raw: string) {
  const name = slug(raw);
  const old = state.doc.name;
  if (!name || name === old) return;
  if (state.designs.some((d) => d.name === name)) return toast(`${name} already exists`);
  set({ doc: { ...state.doc, name } });
  await flushSave();
  await fetch(`/api/designs/${encodeURIComponent(old)}`, { method: "DELETE" });
  localStorage.setItem(LAST_KEY, name);
  await refreshList();
}

export async function deleteDesign(name: string) {
  await fetch(`/api/designs/${encodeURIComponent(name)}`, { method: "DELETE" });
  const rest = (await refreshList()).filter((d) => d.name !== name);
  if (state.doc.name !== name) return;
  set({ loaded: false });
  if (rest[0]) await openDesign(rest[0].name);
  else await createDesign("untitled");
}

export async function boot() {
  const designs = await refreshList();
  if (!designs.length) {
    const doc = starterDoc();
    set({ doc, loaded: true });
    await flushSave();
    await refreshList();
    localStorage.setItem(LAST_KEY, doc.name);
    return;
  }
  const last = localStorage.getItem(LAST_KEY);
  await openDesign(designs.some((d) => d.name === last) ? last! : designs[0]!.name);
}
