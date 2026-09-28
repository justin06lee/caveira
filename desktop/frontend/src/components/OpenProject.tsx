import { chooseProject, makeProject, openPath, setState, useStore } from "../lib/store";
import { PathPicker } from "./PathPicker";

// OpenProject is the project picker over the window (⌘O): the workspace's
// folders, to open one or make a new one.
export function OpenProject() {
  const workspace = useStore((s) => s.workspace);
  const close = () => setState(() => ({ pickerOpen: false }));
  return (
    <div className="scrim top" onMouseDown={(e) => e.target === e.currentTarget && close()}>
      <div className="pick-sheet" role="dialog" aria-label="Open a project">
        <PathPicker
          base={workspace.path}
          mode="project"
          placeholder="Open a project…"
          onPick={openPath}
          onCreate={makeProject}
          onCancel={close}
          aside={<Browse />}
        />
      </div>
    </div>
  );
}

// Browse is the Finder's own folder dialog, for when that is easier.
export function Browse() {
  return (
    <button className="btn quiet" onClick={() => chooseProject()}>
      Browse…
    </button>
  );
}
