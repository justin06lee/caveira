// The Go side, as Wails exposes it on window.go and window.runtime. Called
// directly rather than through generated bindings, so the frontend builds
// on its own and the types stay the ones in types.ts.

import type {
  Boot,
  ChatEvent,
  ChatHeader,
  ChatView,
  ImportApp,
  ImportPick,
  ImportProgress,
  ImportResult,
  Listing,
  ModelOption,
  Project,
  SettingsInput,
  SettingsView,
  Workspace,
} from "./types";

declare global {
  interface Window {
    go?: { main: { App: Record<string, (...args: unknown[]) => Promise<unknown>> } };
    runtime?: {
      EventsOn(name: string, cb: (...data: unknown[]) => void): () => void;
      ClipboardSetText(text: string): Promise<boolean>;
      BrowserOpenURL(url: string): void;
    };
  }
}

function call<T>(method: string, ...args: unknown[]): Promise<T> {
  const app = window.go?.main.App;
  if (!app) return Promise.reject(new Error("not running inside the caveira app"));
  return app[method](...args) as Promise<T>;
}

export const api = {
  boot: () => call<Boot>("Boot"),
  openProject: (dir: string) => call<Project>("OpenProject", dir),
  chooseProject: () => call<Project>("ChooseProject"),
  forgetProject: (dir: string) => call<void>("ForgetProject", dir),
  reveal: (dir: string) => call<void>("Reveal", dir),
  projects: () => call<Project[] | null>("Projects"),
  listDir: (base: string, rel: string) => call<Listing>("ListDir", base, rel),
  makeFolder: (base: string, rel: string) => call<Project>("MakeFolder", base, rel),
  setWorkspace: (dir: string) => call<Workspace>("SetWorkspace", dir),
  suggestWorkspace: () => call<Workspace>("SuggestWorkspace"),
  finishOnboarding: () => call<void>("FinishOnboarding"),
  scanImports: () => call<ImportApp[] | null>("ScanImports"),
  importChats: (picks: ImportPick[]) => call<ImportResult>("Import", picks),
  settings: () => call<SettingsView>("Settings"),
  saveSettings: (s: SettingsInput) => call<SettingsView>("SaveSettings", s),
  models: (local: boolean) => call<ModelOption[] | null>("Models", local),
  newChat: (dir: string) => call<ChatView>("NewChat", dir),
  openChat: (id: string) => call<ChatView>("OpenChat", id),
  chats: (dir: string) => call<ChatHeader[] | null>("Chats", dir),
  deleteChat: (id: string) => call<void>("DeleteChat", id),
  send: (id: string, text: string) => call<void>("Send", id, text),
  stop: (id: string) => call<void>("Stop", id),
  answer: (id: string, callId: string, decision: "allow" | "always" | "deny") =>
    call<void>("Answer", id, callId, decision),
  setModel: (id: string, model: string, window: number, effort: string) =>
    call<ChatView>("SetModel", id, model, window, effort),
};

export function onChatEvent(cb: (e: ChatEvent) => void): () => void {
  return window.runtime?.EventsOn("chat", (e) => cb(e as ChatEvent)) ?? (() => {});
}

export function onImport(cb: (p: ImportProgress) => void): () => void {
  return window.runtime?.EventsOn("import", (p) => cb(p as ImportProgress)) ?? (() => {});
}

export function onMenu(cb: (action: string) => void): () => void {
  return window.runtime?.EventsOn("menu", (a) => cb(a as string)) ?? (() => {});
}

export async function copyText(text: string): Promise<void> {
  if (window.runtime) {
    await window.runtime.ClipboardSetText(text);
    return;
  }
  await navigator.clipboard.writeText(text);
}

export function openURL(url: string): void {
  if (window.runtime) window.runtime.BrowserOpenURL(url);
  else window.open(url, "_blank", "noopener");
}

export function errorText(e: unknown): string {
  if (e instanceof Error) return e.message;
  return String(e);
}
