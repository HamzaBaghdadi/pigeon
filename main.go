package main

import (
	"embed"
	"log"

	"github.com/HamzaBaghdadi/pigeon/internal/security"
	"github.com/HamzaBaghdadi/pigeon/internal/settings"
	"github.com/HamzaBaghdadi/pigeon/internal/store"
	"github.com/HamzaBaghdadi/pigeon/services"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

// The Next.js static export (frontend/dist) is embedded into the binary.
//
//go:embed all:frontend/dist
var assets embed.FS

func main() {
	prefsStore, err := settings.Open()
	if err != nil {
		log.Printf("settings: %v (using defaults)", err)
	}
	prefs := prefsStore.Get()

	dbPath, err := store.DefaultPath()
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	db, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer db.Close()

	core := services.NewCore(db, prefsStore)
	mail := services.NewMailService(core)
	accounts := services.NewAccountService(core)
	settingsSvc := services.NewSettingsService(prefsStore)

	app := application.New(application.Options{
		Name:        "Pigeon",
		Description: "A secure, open-source email client",
		Services: []application.Service{
			application.NewService(mail),
			application.NewService(accounts),
			application.NewService(settingsSvc),
		},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: security.Headers,
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: true,
		},
	})

	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Pigeon",
		Width:     1280,
		Height:    800,
		MinWidth:  900,
		MinHeight: 560,
		Mac: application.MacWindow{
			InvisibleTitleBarHeight: 50,
			Backdrop:                application.MacBackdropTranslucent,
			TitleBar:                application.MacTitleBarHiddenInset,
		},
		Windows: application.WindowsWindow{
			Theme: services.WindowTheme(prefs.Theme),
		},
		BackgroundColour: services.BackgroundFor(services.ResolveDark(prefs.Theme)),
		URL:              "/",
	})
	settingsSvc.AttachWindow(window)

	// When Windows switches light/dark, re-apply the user's choice: a forced
	// light or dark theme must win, and "system" must follow the change.
	app.Event.OnApplicationEvent(events.Windows.SystemThemeChanged, func(*application.ApplicationEvent) {
		settingsSvc.ApplyWindowTheme()
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
