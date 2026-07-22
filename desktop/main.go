package main

import (
	"embed"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"

	"github.com/Mike0165115321/Aetox/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

// webviewUserDataDir returns where a WebView2 instance should store its
// profile (cache/cookies/IndexedDB) — always an explicit, Aetox-owned path
// under config.DataRoot() (ARCHITECTURE.md §14), never Wails'/go-webview2's
// own silent default (%AppData%\<exe-name>, which used to differ between the
// dev binary and the real one — two profiles for the same app). Empty return
// is only a last-resort fallback if DataRoot() itself fails.
func webviewUserDataDir(name string) string {
	root, err := config.DataRoot()
	if err != nil || root == "" {
		return ""
	}
	return filepath.Join(root, "webview", name)
}

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:     "Aetox Desktop",
		Width:     1440,
		Height:    900,
		MinWidth:  1100,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 11, G: 15, B: 22, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		DragAndDrop: &options.DragAndDrop{
			EnableFileDrop: true,
		},
		Windows: &windows.Options{
			WebviewUserDataPath: webviewUserDataDir("app"),
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
