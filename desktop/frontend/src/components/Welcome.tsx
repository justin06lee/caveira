import { Folder } from "lucide-react";
import { makeProject, openPath, selectProject, useStore } from "../lib/store";
import { Logo } from "./Logo";
import { Browse } from "./OpenProject";
import { PathPicker } from "./PathPicker";

// Welcome is the window with no project open: the project picker, with
// the recent ones above it.
export function Welcome() {
  const projects = useStore((s) => s.projects);
  const workspace = useStore((s) => s.workspace);
  return (
    <>
      <header className="header drag" />
      <div className="welcome">
        <div className="welcome-inner">
          <div className="welcome-brand">
            <Logo size={36} />
            <h1>Open a project</h1>
          </div>
          {projects.length > 0 && (
            <div className="recent-chips">
              {projects.slice(0, 6).map((p) => (
                <button key={p.path} className="chip" title={p.short} onClick={() => selectProject(p)}>
                  <Folder size={13} />
                  {p.name}
                </button>
              ))}
            </div>
          )}
          <PathPicker
            base={workspace.path}
            mode="project"
            placeholder="type a project, or a name for a new one"
            onPick={openPath}
            onCreate={makeProject}
            aside={<Browse />}
          />
        </div>
      </div>
    </>
  );
}
