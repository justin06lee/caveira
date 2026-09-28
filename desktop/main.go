// Command caveira-desktop is caveira as a desktop app: the same agent as
// the terminal client, in a window, built with Wails.
package main

import (
	"context"
	"embed"
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/menu"
	"github.com/wailsapp/wails/v2/pkg/menu/keys"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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
	})
	if err != nil {
		log.Fatal(err)
	}
}
