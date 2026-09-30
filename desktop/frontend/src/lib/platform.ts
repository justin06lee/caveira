// What differs between the Mac and Linux windows. On the Mac the title
// bar is hidden under the traffic lights and the menu carries the
// shortcuts; on Linux the window keeps its own title bar, has no menu, and
// the page takes the shortcuts itself (see App).

export const mac = /Mac|iPhone|iPad/.test(navigator.platform);

// shortcut writes a key the way the platform does: ⌘N on the Mac, Ctrl+N
// elsewhere.
export function shortcut(key: string): string {
  return mac ? `⌘${key}` : `Ctrl+${key}`;
}

// The hidden-files toggle is the Finder's ⌘⇧. on the Mac and GTK's Ctrl+H
// on Linux.
export const hiddenShortcut = mac ? "⌘⇧." : "Ctrl+H";

export const revealLabel = mac ? "Show in Finder" : "Open in Files";

export const thisComputer = mac ? "this Mac" : "this computer";

// Shortcuts the page handles itself where there is no menu to.
export function menuAction(e: KeyboardEvent): string | null {
  if (mac || !e.ctrlKey || e.altKey || e.metaKey || e.shiftKey) return null;
  switch (e.key) {
    case "n":
      return "new-chat";
    case "o":
      return "open-project";
    case ",":
      return "settings";
    case "\\":
      return "toggle-sidebar";
  }
  return null;
}
