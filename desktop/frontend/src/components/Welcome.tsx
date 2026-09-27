import { Folder, FolderOpen } from "lucide-react";
import { chooseProject, selectProject, useStore } from "../lib/store";
import { Logo } from "./Logo";

// Welcome is the window with no project open: the first launch, or after
// the last one was removed from the list.
export function Welcome() {
  const projects = useStore((s) => s.projects);
  return (
    <>
      <header className="header drag" />
      <div className="hero">
        <Logo size={64} />
        <h1>caveira</h1>
        <p className="sub">A coding agent for abliterated models.</p>
        <div className="hero-actions">
          <button className="btn primary big" onClick={() => chooseProject()}>
            <FolderOpen size={16} />
            Open a folder
          </button>
          <kbd>⌘O</kbd>
        </div>
        {projects.length > 0 && (
          <div className="recents">
            <div className="side-label">Recent</div>
            {projects.map((p) => (
              <button key={p.path} className="recent" onClick={() => selectProject(p)}>
                <Folder size={15} />
                <span className="grow">
                  {p.name}
                  <span className="path">{p.short}</span>
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
    </>
  );
}
