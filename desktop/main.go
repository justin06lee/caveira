// Command caveira-desktop is caveira as a desktop app: the same agent as
// the terminal client, in a window, built with Wails.
package main

import (
	"context"
	"embed"
	"log"
	"os"
	goruntime "runtime"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

// version is set by the Makefile from the git describe output.
var version = "dev"

func main() {
	// A Finder launch starts with a bare PATH; the user's login shell knows
	// where go, bun, and the rest live. Loaded while the window opens.
	go loadShellEnv()

	// WebKitGTK's DMA-BUF renderer paints a blank window under Xvfb and on
	// NVIDIA's drivers, Jetson included; the shared-memory path works
	// everywhere.
	if goruntime.GOOS == "linux" && os.Getenv("WEBKIT_DISABLE_DMABUF_RENDERER") == "" {
		os.Setenv("WEBKIT_DISABLE_DMABUF_RENDERER", "1")
	}

	app := NewApp(version)

	appMenu := menu.NewMenu()
	appMenu.Append(menu.AppMenu())
	file := appMenu.AddSubmenu("File")
	file.AddText("New Chat", keys.CmdOrCtrl("n"), func(*menu.CallbackData) { app.emit("menu", "new-chat") })
	file.AddText("Open Project…", keys.CmdOrCtrl("o"), func(*menu.CallbackData) { app.emit("menu", "open-project") })
	file.AddSeparator()
	file.AddText("Import from Other Agents…", nil, func(*menu.CallbackData) { app.emit("menu", "import") })
	file.AddSeparator()
	file.AddText("Settings…", keys.CmdOrCtrl(","), func(*menu.CallbackData) { app.emit("menu", "settings") })
	appMenu.Append(menu.EditMenu())
	view := appMenu.AddSubmenu("View")
	view.AddText("Toggle Sidebar", keys.CmdOrCtrl("\\"), func(*menu.CallbackData) { app.emit("menu", "toggle-sidebar") })
	appMenu.Append(menu.WindowMenu())
	// On Linux the menu would be a GTK menu bar across the top of the
	// window; the page takes the same shortcuts instead (App.tsx), and
	// Import is in Settings.
	if goruntime.GOOS != "darwin" {
		appMenu = nil
	}

	err := wails.Run(&options.App{
		Title:     "caveira",
		Width:     1180,
		Height:    780,
		MinWidth:  720,
		MinHeight: 480,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		Menu:             appMenu,
		BackgroundColour: &options.RGBA{R: 248, G: 247, B: 242, A: 255},
		OnStartup:        app.startup,
		OnBeforeClose: func(ctx context.Context) bool {
			app.stopAll()
			return false
		},
		Bind: []any{app},
		Mac: &mac.Options{
			TitleBar:             mac.TitleBarHidden(),
			Appearance:           mac.DefaultAppearance,
			WebviewIsTransparent: false,
			About: &mac.AboutInfo{
				Title:   "caveira",
				Message: "A coding agent for abliterated models.\n" + version,
				Icon:    icon,
			},
		},
		Linux: &linux.Options{
			Icon: icon,
			// The window's WM_CLASS, which GNOME matches to caveira.desktop.
			ProgramName: "caveira",
			// What Wails picks when Linux options are left out: compositing
			// on the GPU leaves the window blank on some NVIDIA setups.
			WebviewGpuPolicy: linux.WebviewGpuPolicyNever,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
