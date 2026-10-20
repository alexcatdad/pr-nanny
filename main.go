package main

import (
	"embed"
	"io"
	"log/slog"
	"os"
	"runtime"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/websecurity"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/icons"
)

//go:embed all:assets
var assets embed.FS

func main() {
	app := application.New(application.Options{
		Name: "PR Nanny", Description: "A passive GitHub pull-request monitor",
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets), Middleware: websecurity.Middleware, DisableLogging: true},
		Mac:    application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
	})
	popup := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "popup", Title: "PR Nanny", Width: 420, Height: 580, Frameless: true, AlwaysOnTop: true, Hidden: true,
		HideOnEscape: true, HideOnFocusLost: true, DisableResize: true, DevToolsEnabled: false, DefaultContextMenuDisabled: true,
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
	})
	popup.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { popup.Hide(); e.Cancel() })
	tray := app.SystemTray.New()
	tray.SetTooltip("PR Nanny")
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(icons.SystrayMacTemplate)
	} else {
		tray.SetIcon(icons.SystrayLight)
	}
	tray.AttachWindow(popup).WindowOffset(5)
	menu := app.NewMenu()
	menu.Add("Settings").OnClick(func(*application.Context) { popup.SetURL("/?settings=1"); popup.Show(); popup.Focus() })
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	// CI smoke mode proves native event-loop startup without authentication or OS preference changes.
	if len(os.Args) == 2 && os.Args[1] == "--smoke-test" {
		popup.OnWindowEvent(events.Common.WindowShow, func(*application.WindowEvent) {})
		go func() { time.Sleep(3 * time.Second); app.Quit() }()
	}
	if err := app.Run(); err != nil {
		os.Exit(1)
	}
}
