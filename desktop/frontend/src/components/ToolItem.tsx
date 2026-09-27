import { memo, useState } from "react";
import {
  ChevronRight,
  FilePen,
  FilePlus2,
  FileText,
  FolderTree,
  Files,
  Hand,
  Search,
  SquareTerminal,
  Wrench,
  type LucideIcon,
} from "lucide-react";
import { api } from "../lib/bridge";
import { diffStat, duration } from "../lib/format";
import type { Item, ToolView } from "../lib/types";

const icons: Record<string, LucideIcon> = {
  read_file: FileText,
  write_file: FilePlus2,
  edit_file: FilePen,
  bash: SquareTerminal,
  glob: Files,
  grep: Search,
  list_dir: FolderTree,
};

export const ToolItem = memo(function ToolItem({ item, chatId }: { item: Item; chatId: string }) {
  const t = item.tool!;
  const [open, setOpen] = useState(false);
  if (t.status === "approval") return <Approval tool={t} chatId={chatId} />;

  const Icon = icons[t.name] ?? Wrench;
  const hasBody = Boolean(t.diff || t.output);
  const running = t.status === "running";

  return (
    <div className={`tool item${t.status === "error" ? " error" : ""}`}>
      <button
        className={`tool-row${hasBody ? " can-open" : ""}`}
        onClick={() => hasBody && setOpen(!open)}
        title={t.ms ? `${t.label} · ${duration(t.ms)}` : undefined}
      >
        <Icon size={14} className="tool-icon" />
        <span className={`tool-label${running ? " shimmer" : ""}`}>{t.label}</span>
        <span className="tool-preview">{t.preview}</span>
        {!running && <Summary tool={t} />}
        {hasBody && <ChevronRight size={13} className={`tool-chev${open ? " open" : ""}`} />}
      </button>
      {open && (
        <div className="tool-body">{t.diff ? <Diff text={t.diff} /> : <pre>{t.output}</pre>}</div>
      )}
    </div>
  );
});

function Summary({ tool }: { tool: ToolView }) {
  if (tool.diff && tool.status !== "error") {
    const { add, del } = diffStat(tool.diff);
    return (
      <span className="tool-summary diff-stat">
        <span className="plus">+{add}</span> <span className="minus">−{del}</span>
      </span>
    );
  }
  return <span className="tool-summary">{tool.summary}</span>;
}

function Diff({ text }: { text: string }) {
  return (
    <div className="diff">
      {text.split("\n").map((l, i) => {
        const cls = l.startsWith("@@") ? "hunk" : l[0] === "+" ? "add" : l[0] === "-" ? "del" : "";
        return (
          <div key={i} className={`diff-line ${cls}`}>
            {cls === "add" || cls === "del" ? (
              <>
                <span className="sign">{l[0]}</span>
                {l.slice(1)}
              </>
            ) : (
              l || " "
            )}
          </div>
        );
      })}
    </div>
  );
}

function Approval({ tool, chatId }: { tool: ToolView; chatId: string }) {
  const answer = (d: "allow" | "always" | "deny") => api.answer(chatId, tool.call, d);
  const question =
    tool.kind === "execute" ? "caveira wants to run a command" : `caveira wants to ${tool.label.toLowerCase()} a file`;
  return (
    <div className="approval item">
      <div className="approval-q">
        <Hand size={14} />
        {question}
      </div>
      {tool.proposed ? (
        <div className="approval-diff">
          <div className="approval-path">{tool.preview}</div>
          <Diff text={tool.proposed} />
        </div>
      ) : (
        <div className="approval-what">{tool.preview}</div>
      )}
      <div className="approval-actions">
        <button className="btn quiet" onClick={() => answer("deny")}>
          Deny
        </button>
        <button className="btn" onClick={() => answer("always")}>
          Always allow
        </button>
        <button className="btn primary" autoFocus onClick={() => answer("allow")}>
          Allow
        </button>
      </div>
    </div>
  );
}
