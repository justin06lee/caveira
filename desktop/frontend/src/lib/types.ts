// Mirrors of the Go types in desktop/*.go, as they arrive over the bridge.

export interface Project {
  path: string;
  name: string;
  short: string;
  branch: string;
}

export interface SettingsView {
  model: string;
  effort: string;
  confirm: boolean;
  theme: Theme;
  efforts: string[];
  ready: boolean;
}

export type Theme = "system" | "light" | "dark";

export interface SettingsInput {
  model: string;
  effort: string;
  confirm: boolean;
  theme: Theme;
}

export interface Workspace {
  path: string;
  short: string;
}

export interface Boot {
  version: string;
  settings: SettingsView;
  projects: Project[];
  workspace: Workspace;
  onboarded: boolean;
  // plan is the plan this install is on, "" for none.
  plan: string;
  plans: Plan[];
}

export interface Plan {
  id: string;
  name: string;
  // price is US dollars a month.
  price: number;
  features: string[];
  popular?: boolean;
  // large says it runs the large models.
  large: boolean;
}

// The folder picker.

export interface Entry {
  name: string;
  dir: boolean;
  link?: boolean;
  repo?: boolean;
}

export interface Listing {
  path: string;
  name: string;
  short: string;
  entries: Entry[] | null;
}

// Importing from other agents.

export interface ImportProject {
  path: string;
  name: string;
  short: string;
  chats: number;
  done: number;
  updated: string;
}

export interface ImportApp {
  id: string;
  name: string;
  chats: number;
  projects: ImportProject[];
}

export interface ImportPick {
  app: string;
  dirs: string[];
}

export interface ImportResult {
  chats: number;
  projects: number;
  skipped: number;
  empty: number;
  failed: number;
}

export interface ImportProgress {
  done: number;
  total: number;
}

export interface ToolView {
  call: string;
  name: string;
  label: string;
  preview: string;
  kind: "read" | "write" | "execute";
  status: "running" | "approval" | "done" | "error";
  summary?: string;
  output?: string;
  diff?: string;
  ms?: number;
  proposed?: string;
}

export interface Item {
  id: string;
  // A paywall holds the messages above it until there is a plan; its
  // text is the model the plan lacks, if that is why.
  kind: "user" | "assistant" | "tool" | "notice" | "paywall";
  text?: string;
  reasoning?: string;
  streaming?: boolean;
  tool?: ToolView;
  tone?: "info" | "error";
}

export interface ChatView {
  id: string;
  dir: string;
  title: string;
  model: string;
  effort: string;
  window: number;
  context: number;
  cost: number;
  running: boolean;
  // ask says writes and commands wait for approval; otherwise every
  // permission is bypassed.
  ask: boolean;
  items: Item[];
  problem?: string;
  needsKey?: boolean;
}

export interface ChatHeader {
  id: string;
  // dir is the project the chat belongs to.
  dir: string;
  title: string;
  updated: string;
  running: boolean;
}

export interface TurnStats {
  ms: number;
  tools: number;
  cost: number;
}

export interface ChatEvent {
  chat: string;
  type: "item" | "delta" | "remove" | "usage" | "start" | "done" | "title";
  item?: Item;
  id?: string;
  text?: string;
  reasoning?: string;
  context?: number;
  cost?: number;
  title?: string;
  interrupted?: boolean;
  stats?: TurnStats;
}

export interface ModelOption {
  id: string;
  // name is what it is called on screen, under the heading group, beside
  // the logos of who makes it.
  name: string;
  group?: string;
  logos?: string[];
  context: number;
  note: string;
  unusable?: string;
  noEffort?: boolean;
  // plan is the cheapest plan that runs it, when Free does not.
  plan?: string;
}
