import { useEffect } from "react";
import { X } from "lucide-react";
import { onMenu } from "./lib/bridge";
import { applyTheme } from "./lib/theme";
import { boot, chooseProject, listen, newChat, setState, toast, useStore } from "./lib/store";
import { Sidebar } from "./components/Sidebar";
import { ChatPane } from "./components/ChatPane";
import { Welcome } from "./components/Welcome";
import { Settings } from "./components/Settings";

export function App() {
  const ready = useStore((s) => s.ready);
  const project = useStore((s) => s.project);
  const sidebar = useStore((s) => s.sidebar);
  const settingsOpen = useStore((s) => s.settingsOpen);
  const theme = useStore((s) => s.settings?.theme ?? "system");
  const message = useStore((s) => s.toast);

  useEffect(() => {
    const stopEvents = listen();
    const stopMenu = onMenu((action) => {
      switch (action) {
        case "new-chat":
          newChat();
          break;
        case "open-folder":
          chooseProject();
          break;
        case "settings":
          setState(() => ({ settingsOpen: true }));
          break;
        case "toggle-sidebar":
          setState((s) => ({ sidebar: !s.sidebar }));
          break;
      }
    });
    boot().catch(toast);
    return () => {
      stopEvents();
      stopMenu();
    };
  }, []);

  useEffect(() => applyTheme(theme), [theme]);

  if (!ready) return <div className="app drag" />;

  return (
    <div className="app">
      <Sidebar hidden={!sidebar || !project} />
      <main className="main">{project ? <ChatPane /> : <Welcome />}</main>
      {settingsOpen && <Settings />}
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
