import { useEffect } from "react";
import { X } from "lucide-react";
import { api, onMenu } from "./lib/bridge";
import { menuAction } from "./lib/platform";
import { applyTheme } from "./lib/theme";
import { boot, getState, listen, newChat, setState, toast, useStore } from "./lib/store";
import { Sidebar } from "./components/Sidebar";
import { ChatPane } from "./components/ChatPane";
import { Welcome } from "./components/Welcome";
import { Settings } from "./components/Settings";
import { Onboarding } from "./components/Onboarding";
import { OpenProject } from "./components/OpenProject";
import { ImportSheet } from "./components/ImportSheet";

export function App() {
  const ready = useStore((s) => s.ready);
  const project = useStore((s) => s.project);
  const sidebar = useStore((s) => s.sidebar);
  const settingsOpen = useStore((s) => s.settingsOpen);
  const theme = useStore((s) => s.settings?.theme ?? "system");
  const message = useStore((s) => s.toast);
  const onboarded = useStore((s) => s.onboarded);
  const pickerOpen = useStore((s) => s.pickerOpen);
  const importOpen = useStore((s) => s.importOpen);

  useEffect(() => {
    const stopEvents = listen();
    const run = (action: string) => {
      // The first run finishes before the menu does anything.
      if (!getState().onboarded) return;
      switch (action) {
        case "new-chat":
          newChat();
          break;
        case "open-project":
          setState(() => ({ pickerOpen: true, settingsOpen: false }));
          break;
        case "import":
          setState(() => ({ importOpen: true, settingsOpen: false }));
          break;
        case "settings":
          setState(() => ({ settingsOpen: true }));
          break;
        case "toggle-sidebar":
          setState((s) => ({ sidebar: !s.sidebar }));
          break;
      }
    };
    const stopMenu = onMenu(run);
    // Linux has no menu, so the page takes its shortcuts.
    const onKey = (e: KeyboardEvent) => {
      const action = menuAction(e);
      if (action) {
        e.preventDefault();
        run(action);
      }
    };
    window.addEventListener("keydown", onKey);
    // A double-click on the title bar zooms the window, as in any window.
    // Wails starts no drag on the second click, so it reaches the page.
    const onDown = (e: MouseEvent) => {
      if (e.detail !== 2 || e.button !== 0 || !(e.target instanceof Element)) return;
      if (getComputedStyle(e.target).getPropertyValue("--wails-draggable").trim() === "drag")
        api.titleBarDoubleClick().catch(() => {});
    };
    window.addEventListener("mousedown", onDown);
    boot().catch(toast);
    return () => {
      stopEvents();
      stopMenu();
      window.removeEventListener("keydown", onKey);
      window.removeEventListener("mousedown", onDown);
    };
  }, []);

  useEffect(() => applyTheme(theme), [theme]);

  if (!ready) return <div className="app drag" />;

  return (
    <div className="app">
      {onboarded && <Sidebar hidden={!sidebar || !project} />}
      <main className="main">
        {!onboarded ? <Onboarding /> : settingsOpen ? <Settings /> : project ? <ChatPane /> : <Welcome />}
      </main>
      {pickerOpen && <OpenProject />}
      {importOpen && <ImportSheet />}
      {message && (
        <div className="toast" role="status">
          <span>{message}</span>
          <button className="no-drag" onClick={() => setState(() => ({ toast: null }))} aria-label="Dismiss">
            <X size={14} />
          </button>
        </div>
      )}
    </div>
  );
}
