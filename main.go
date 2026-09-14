package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func init() {
	application.RegisterEvent[ProgressEvent]("tts:progress")
	application.RegisterEvent[ProgressEvent]("podcast:progress")
}

func main() {
	app := application.New(application.Options{
		Name:        "oido-tts",
		Description: "Local text-to-speech",
		Services: []application.Service{
			application.NewService(NewApp()),
		},
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:  "oido-tts",
		Width:  1100,
		Height: 720,
		Mac: application.MacWindow{
			Backdrop:                application.MacBackdropLiquidGlass,
			InvisibleTitleBarHeight: 40, // makes the window draggable from the top
			LiquidGlass: application.MacLiquidGlass{
				Style:        application.LiquidGlassStyleLight,
				Material:     application.NSVisualEffectMaterialHUDWindow,
				CornerRadius: 20.0,
				TintColor:    &application.RGBA{Red: 0, Green: 100, Blue: 200, Alpha: 50},
			},
		},
		URL: "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
