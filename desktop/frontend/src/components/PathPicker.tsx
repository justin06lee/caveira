import { useEffect, useLayoutEffect, useRef, useState, type KeyboardEvent, type MouseEvent, type ReactNode } from "react";
import {
  CornerLeftUp,
  Eye,
  EyeOff,
  File,
  FileArchive,
  FileAudio,
  FileCode,
  FileImage,
  FileJson,
  FileLock,
  FileSpreadsheet,
  FileSymlink,
  FileText,
  FileVideo,
  Folder,
  FolderGit2,
  FolderPlus,
  FolderSymlink,
  House,
  type LucideIcon,
} from "lucide-react";
import { api, errorText } from "../lib/bridge";
import { hiddenShortcut, mac } from "../lib/platform";
import type { Entry, Listing } from "../lib/types";

// PathPicker is a folder's contents under a box to type a path in, the way
// ls -a would show them. Typing narrows the list to what starts with the
// last part of the path; a slash goes into a folder; tab completes as far
// as the matches agree and, when they don't, steps through them; .. and
// enter go up a level for as long as the picker is open. Hidden entries
// show when asked for, or when what is typed starts with a dot.

export type PickerMode = "project" | "workspace";

interface Props {
  // base is the folder listed when nothing is typed; "" is home.
  base: string;
  mode: PickerMode;
  placeholder: string;
  // initial is typed in at the start.
  initial?: string;
  onPick: (path: string) => void;
  // onCreate, when given, offers to make a folder that is not there.
  onCreate?: (base: string, rel: string) => void;
  onCancel?: () => void;
  // aside sits in the footer, before the main button.
  aside?: ReactNode;
}

type Target =
  | { kind: "open"; path: string; name: string }
  | { kind: "go"; rel: string; name: string }
  | { kind: "create"; rel: string; name: string };

type Row =
  | { kind: "entry"; entry: Entry }
  | { kind: "go"; rel: string; label: string; home: boolean }
  | { kind: "create"; rel: string; name: string };

// Past this many rows the rest is counted, not drawn.
const ROW_CAP = 400;

export function PathPicker({ base, mode, placeholder, initial = "", onPick, onCreate, onCancel, aside }: Props) {
  const [cwd, setCwd] = useState(base);
  const [query, setQuery] = useState(initial);
  const [hidden, setHidden] = useState(false);
  const [sel, setSel] = useState<number | null>(null);
  // menu is a tab cycle through matches, frozen as they were when it began.
  const [menu, setMenu] = useState<{ items: Entry[]; head: string; index: number } | null>(null);
  const [shown, setShown] = useState<{ key: string; data?: Listing; error?: string } | null>(null);
  const cache = useRef(new Map<string, Promise<Listing>>());
  const input = useRef<HTMLInputElement>(null);
  const list = useRef<HTMLDivElement>(null);

  useEffect(() => {
    setCwd(base);
  }, [base]);

  const [dirPart, leaf] = split(query);
  const key = cwd + "\u0000" + dirPart;

  useEffect(() => {
    let live = true;
    let p = cache.current.get(key);
    if (!p) {
      p = api.listDir(cwd, dirPart);
      cache.current.set(key, p);
      p.catch(() => cache.current.delete(key));
    }
    p.then(
      (data) => live && setShown({ key, data }),
      (e) => live && setShown({ key, error: errorText(e) }),
    );
    return () => {
      live = false;
    };
  }, [key, cwd, dirPart]);

  // What arrived last stays up until the next listing does, so typing a
  // slash does not flash an empty list.
  const data = shown?.data;
  const error = shown?.key === key ? shown.error : undefined;
  const fresh = shown?.key === key;

  const nav = leaf === ".." || (leaf === "~" && dirPart === "");
  const showHidden = hidden || leaf.startsWith(".");
  const all = (data?.entries ?? []).filter((e) => showHidden || !e.name.startsWith(".")).sort(byKind);
  const needle = leaf.toLowerCase();
  let matches = needle && !nav ? all.filter((e) => e.name.toLowerCase().startsWith(needle)) : all;
  let loose = false;
  if (needle && !nav && matches.length === 0) {
    matches = all.filter((e) => e.name.toLowerCase().includes(needle));
    loose = matches.length > 0;
  }
  if (nav) matches = [];

  const rows: Row[] = menu
    ? menu.items.map((entry) => ({ kind: "entry", entry }))
    : matches.map((entry) => ({ kind: "entry", entry }));
  if (!menu && nav) {
    rows.push(
      leaf === ".."
        ? { kind: "go", rel: dirPart + "..", label: "Up a level", home: false }
        : { kind: "go", rel: "~", label: "Home", home: true },
    );
  }
  const missing = Boolean(error?.startsWith("there is no"));
  const newName = missing ? query.replace(/\/+$/, "") : leaf;
  const canCreate = Boolean(onCreate) && fresh && !nav && !menu && newName !== "" &&
    (missing || (data && !all.some((e) => e.name === leaf)));
  if (canCreate && (missing || matches.length === 0)) {
    rows.push({ kind: "create", rel: missing ? newName : dirPart + leaf, name: newName });
  }

  const active = menu ? menu.index : sel ?? (leaf && rows.length > 0 ? 0 : null);

  const target: Target | null = (() => {
    const row = active !== null ? rows[active] : undefined;
    if (row?.kind === "go") return { kind: "go", rel: row.rel, name: row.label };
    if (row?.kind === "create") return { kind: "create", rel: row.rel, name: row.name };
    if (row?.kind === "entry") {
      return row.entry.dir && data && fresh ? { kind: "open", path: join(data.path, row.entry.name), name: row.entry.name } : null;
    }
    // The folder being listed is a choice when choosing a workspace, or
    // when a project's name was typed with a slash after it; otherwise
    // enter would open the workspace itself.
    if (leaf === "" && data && fresh && (mode === "workspace" || dirPart !== "")) {
      return { kind: "open", path: data.path, name: data.name || data.path };
    }
    return null;
  })();

  useLayoutEffect(() => {
    if (active === null) return;
    list.current?.querySelector(`[data-row="${active}"]`)?.scrollIntoView({ block: "nearest" });
  }, [active, menu]);

  const type = (q: string) => {
    setQuery(q);
    setSel(null);
    setMenu(null);
  };

  const act = async (t: Target | null) => {
    if (!t) return;
    if (t.kind === "open") onPick(t.path);
    else if (t.kind === "create") onCreate?.(cwd, t.rel);
    else {
      try {
        const l = await api.listDir(cwd, t.rel);
        setCwd(l.path);
        type("");
      } catch (e) {
        setShown({ key, error: errorText(e) });
      }
    }
  };

  const complete = (back: boolean) => {
    if (menu) {
      const n = menu.items.length;
      const index = (menu.index + (back ? n - 1 : 1)) % n;
      setMenu({ ...menu, index });
      setQuery(menu.head + menu.items[index].name);
      return;
    }
    if (nav || matches.length === 0) return;
    if (matches.length === 1) {
      const e = matches[0];
      type(dirPart + e.name + (e.dir ? "/" : ""));
      return;
    }
    const common = sharedPrefix(matches.map((m) => m.name));
    if (!loose && common.length > leaf.length) {
      type(dirPart + common);
      return;
    }
    // They agree no further: step through them, as a shell's menu does.
    const index = back ? matches.length - 1 : 0;
    setMenu({ items: matches, head: dirPart, index });
    setSel(null);
    setQuery(dirPart + matches[index].name);
  };

  const move = (by: number) => {
    if (rows.length === 0) return;
    if (menu) {
      const n = menu.items.length;
      const index = (menu.index + by + n) % n;
      setMenu({ ...menu, index });
      setQuery(menu.head + menu.items[index].name);
      return;
    }
    const from = active ?? (by > 0 ? -1 : rows.length);
    setSel(Math.min(rows.length - 1, Math.max(0, from + by)));
  };

  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (
      mac
        ? e.metaKey && e.shiftKey && (e.code === "Period" || e.key === "." || e.key === ">")
        : e.ctrlKey && !e.shiftKey && !e.altKey && e.key === "h"
    ) {
      e.preventDefault();
      setHidden((h) => !h);
      return;
    }
    switch (e.key) {
      case "Tab":
        e.preventDefault();
        complete(e.shiftKey);
        break;
      case "ArrowDown":
        e.preventDefault();
        move(1);
        break;
      case "ArrowUp":
        e.preventDefault();
        move(-1);
        break;
      case "Enter":
        e.preventDefault();
        act(target);
        break;
      case "Escape":
        e.preventDefault();
        e.stopPropagation();
        if (menu) setMenu(null);
        else if (query) type("");
        else if (cwd !== base) setCwd(base);
        else onCancel?.();
        break;
    }
  };

  const down = (e: MouseEvent, i: number) => {
    e.preventDefault(); // keep the caret in the box
    input.current?.focus();
    if (menu) {
      setMenu({ ...menu, index: i });
      setQuery(menu.head + menu.items[i].name);
    } else setSel(i);
  };

  const into = (entry: Entry) => {
    if (entry.dir) type((menu ? menu.head : dirPart) + entry.name + "/");
  };

  const verb = mode === "workspace" ? "use" : "open";
  const button = !target
    ? mode === "workspace"
      ? "Use this folder"
      : "Open"
    : target.kind === "create"
      ? `Create ${tail(target.name)}`
      : target.kind === "go"
        ? target.name
        : `${mode === "workspace" ? "Use" : "Open"} ${target.name}`;
  const drawn = rows.slice(0, ROW_CAP);
  const more = rows.length - drawn.length;
  const baseName = tail(base) || "home";

  return (
    <div className="picker">
      <label className="pick-input">
        <input
          ref={input}
          value={query}
          placeholder={placeholder}
          autoFocus
          spellCheck={false}
          autoCorrect="off"
          autoCapitalize="off"
          onChange={(e) => type(e.target.value)}
          onKeyDown={onKey}
        />
        <button
          type="button"
          className={`icon-btn pick-eye${hidden ? " on" : ""}`}
          title={`${hidden ? "Hide" : "Show"} hidden files (${hiddenShortcut})`}
          onMouseDown={(e) => e.preventDefault()}
          onClick={() => setHidden(!hidden)}
        >
          {hidden ? <Eye size={15} /> : <EyeOff size={15} />}
        </button>
      </label>

      {cwd !== base && (
        <div className="pick-where">
          <CornerLeftUp size={12} />
          <span className="grow">in {data?.short ?? cwd}</span>
          <button className="link" onMouseDown={(e) => e.preventDefault()} onClick={() => setCwd(base)}>
            back to {baseName}
          </button>
        </div>
      )}

      <div className="pick-list" ref={list}>
        <div className="pick-stub" />
        {drawn.map((row, i) => (
          <PickRow
            key={row.kind === "entry" ? "e:" + row.entry.name : row.kind}
            row={row}
            index={i}
            last={i === drawn.length - 1 && more === 0}
            on={i === active}
            hint={i === active && target ? (target.kind === "go" ? "go" : target.kind === "create" ? "create" : verb) : ""}
            needle={menu ? "" : leaf}
            loose={loose}
            onDown={(e) => down(e, i)}
            onOpen={() => (row.kind === "entry" ? into(row.entry) : act(target))}
          />
        ))}
        {more > 0 && <div className="pick-more">… {more} more</div>}
        {rows.length === 0 && fresh && (
          <div className="pick-empty">
            {error ? error : leaf ? `Nothing here starts with “${leaf}”` : all.length === 0 && data?.entries?.length ? "Only hidden files here" : "Empty folder"}
          </div>
        )}
      </div>

      <div className="pick-foot">
        <div className="pick-keys">
          {menu ? (
            <span>
              {menu.index + 1} of {menu.items.length} · <kbd>tab</kbd> next
            </span>
          ) : leaf && matches.length > 1 ? (
            <span>
              {matches.length} matches · <kbd>tab</kbd>
            </span>
          ) : (
            <>
              <span>
                <kbd>tab</kbd> complete
              </span>
              <span>
                <kbd>..</kbd> up
              </span>
            </>
          )}
        </div>
        {aside}
        <button className="btn primary" disabled={!target} onClick={() => act(target)}>
          {button}
          <kbd className="on-primary">↵</kbd>
        </button>
      </div>
    </div>
  );
}

function PickRow(props: {
  row: Row;
  index: number;
  last: boolean;
  on: boolean;
  hint: string;
  needle: string;
  loose: boolean;
  onDown: (e: MouseEvent) => void;
  onOpen: () => void;
}) {
  const { row } = props;
  let Icon: LucideIcon;
  let label: ReactNode;
  let cls = "pick-row";
  if (row.kind === "entry") {
    const e = row.entry;
    Icon = iconFor(e);
    label = (
      <>
        <Name name={e.name} needle={props.needle} loose={props.loose} />
        {e.dir && <span className="slash">/</span>}
      </>
    );
    cls += e.dir ? " dir" : " file";
    if (e.name.startsWith(".")) cls += " dotfile";
  } else if (row.kind === "go") {
    Icon = row.home ? House : CornerLeftUp;
    label = row.label;
    cls += " dir";
  } else {
    Icon = FolderPlus;
    label = (
      <>
        New folder <b>{row.name}</b>
      </>
    );
    cls += " dir create";
  }
  return (
    <div
      className={cls + (props.on ? " on" : "")}
      data-row={props.index}
      onMouseDown={props.onDown}
      onDoubleClick={props.onOpen}
    >
      <Elbow last={props.last} />
      <div className="pick-main">
        <Icon size={15} className="pick-icon" />
        <span className="pick-name">{label}</span>
        {row.kind === "entry" && row.entry.link && <span className="pick-tag">link</span>}
        {props.hint && <span className="pick-hint">↵ {props.hint}</span>}
      </div>
    </div>
  );
}

// Name marks the part of a name that matched what was typed.
function Name({ name, needle, loose }: { name: string; needle: string; loose: boolean }) {
  if (!needle) return <>{name}</>;
  const at = loose ? name.toLowerCase().indexOf(needle.toLowerCase()) : 0;
  if (at < 0) return <>{name}</>;
  return (
    <>
      {name.slice(0, at)}
      <span className="hit">{name.slice(at, at + needle.length)}</span>
      {name.slice(at + needle.length)}
    </>
  );
}

// Elbow is the line from the box down to a row and into it.
function Elbow({ last }: { last: boolean }) {
  return (
    <svg className="elbow" width="28" height="30" viewBox="0 0 28 30" aria-hidden="true">
      <path d={last ? "M9 0 V11 Q9 15 13 15 H23" : "M9 0 V30 M9 11 Q9 15 13 15 H23"} />
      <path d="M19.5 11.5 L23 15 L19.5 18.5" />
    </svg>
  );
}

function split(q: string): [string, string] {
  const i = q.lastIndexOf("/");
  return i < 0 ? ["", q] : [q.slice(0, i + 1), q.slice(i + 1)];
}

function join(dir: string, name: string): string {
  return dir.endsWith("/") ? dir + name : dir + "/" + name;
}

function tail(path: string): string {
  const parts = path.replace(/\/+$/, "").split("/");
  return parts[parts.length - 1] ?? "";
}

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

// Folders first, then files, each in the order a person would sort them.
function byKind(a: Entry, b: Entry): number {
  if (a.dir !== b.dir) return a.dir ? -1 : 1;
  return collator.compare(a.name.replace(/^\./, ""), b.name.replace(/^\./, ""));
}

// sharedPrefix is how far the names agree, ignoring case, spelled as the
// first one spells it.
function sharedPrefix(names: string[]): string {
  let n = names[0].length;
  const first = names[0].toLowerCase();
  for (const name of names.slice(1)) {
    const lower = name.toLowerCase();
    let i = 0;
    while (i < n && i < lower.length && lower[i] === first[i]) i++;
    n = i;
  }
  return names[0].slice(0, n);
}

const kinds: [LucideIcon, string[]][] = [
  [FileImage, ["png", "jpg", "jpeg", "gif", "webp", "svg", "heic", "bmp", "ico", "icns", "tif", "tiff", "avif", "psd"]],
  [FileVideo, ["mp4", "mov", "mkv", "webm", "avi", "m4v"]],
  [FileAudio, ["mp3", "wav", "flac", "m4a", "ogg", "aac", "aiff"]],
  [FileArchive, ["zip", "tar", "gz", "tgz", "bz2", "xz", "7z", "rar", "dmg", "zst"]],
  [FileSpreadsheet, ["csv", "tsv", "xls", "xlsx", "numbers"]],
  [FileJson, ["json", "jsonc", "jsonl", "yaml", "yml", "toml", "lock", "xml", "plist", "ini", "conf"]],
  [FileText, ["md", "mdx", "txt", "rtf", "pdf", "doc", "docx", "pages", "log"]],
  [FileLock, ["pem", "key", "crt", "p12", "gpg"]],
  [
    FileCode,
    ["ts", "tsx", "js", "jsx", "mjs", "cjs", "go", "rs", "py", "rb", "java", "kt", "swift", "c", "h", "cpp", "cc", "hpp",
      "cs", "php", "lua", "sh", "zsh", "bash", "fish", "sql", "html", "css", "scss", "vue", "svelte", "zig", "dart", "ex",
      "exs", "ml", "hs", "nim", "mod", "sum", "wasm"],
  ],
];

function iconFor(e: Entry): LucideIcon {
  if (e.dir) return e.link ? FolderSymlink : e.repo ? FolderGit2 : Folder;
  if (e.link) return FileSymlink;
  const name = e.name.toLowerCase();
  if (name.startsWith(".env")) return FileLock;
  if (name === "makefile" || name === "dockerfile") return FileCode;
  const ext = name.includes(".") ? name.slice(name.lastIndexOf(".") + 1) : "";
  for (const [icon, exts] of kinds) if (exts.includes(ext)) return icon;
  return File;
}
