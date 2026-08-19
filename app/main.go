package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	wailsApp := NewApp()

	// Create application with options
	err := wails.Run(&options.App{
		Title:  "ccContextRead",
		Width:  1024,
		Height: 768,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 250, G: 249, B: 245, A: 1},
		OnStartup:        wailsApp.startup,
		OnShutdown:       wailsApp.shutdown,
		Bind: []interface{}{
			wailsApp,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
