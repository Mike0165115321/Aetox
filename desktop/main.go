package main

import (
	"embed"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

// webviewUserDataDir returns where a WebView2 instance should store its
// profile (cache/cookies/IndexedDB). Empty keeps Wails'/go-webview2's own
// default (%AppData%\<name>) — the normal, portable behavior for a real
// install; nothing changes for end users. Set AETOX_WEBVIEW_DATA_DIR (e.g. in
// wails-dev.bat) to redirect it off the system drive during development,
// where repeated `wails dev` runs otherwise grow this without bound.
func webviewUserDataDir(name string) string {
	base := strings.TrimSpace(os.Getenv("AETOX_WEBVIEW_DATA_DIR"))
	if base == "" {
		return ""
	}
	return filepath.Join(base, name)
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
