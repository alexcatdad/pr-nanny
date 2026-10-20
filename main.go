package main

import (
	"embed"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	appcore "github.com/alexcatdad/pr-nanny/internal/app"
	"github.com/alexcatdad/pr-nanny/internal/auth"
	"github.com/alexcatdad/pr-nanny/internal/core"
	ghapi "github.com/alexcatdad/pr-nanny/internal/github"
	"github.com/alexcatdad/pr-nanny/internal/storage"
	"github.com/alexcatdad/pr-nanny/internal/websecurity"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/tray.png
var trayIcon []byte

//go:embed assets/icon.png
var appIcon []byte
var version = "0.1.0-dev"

func main() {
	smoke := len(os.Args) == 2 && os.Args[1] == "--smoke-test"
	preview := developmentPreview()
	configDir, err := os.UserConfigDir()
	if err != nil {
		os.Exit(1)
	}
	dataDir := filepath.Join(configDir, "pr-nanny")
	if smoke || preview {
		dataDir, err = os.MkdirTemp("", "pr-nanny-smoke-")
		if err != nil {
			os.Exit(1)
		}
		defer os.RemoveAll(dataDir)
	}
	native := &nativePlatform{notifications: notifications.New(), isolated: smoke || preview}
	store := storage.New(dataDir)
	// Preserve corrupted non-secret data and restart with a fresh baseline.
	recovered := false
	if _, err := store.Settings(); errors.Is(err, storage.ErrCorrupt) {
		if store.QuarantineCorrupt("settings.json") != nil {
			os.Exit(1)
		}
		recovered = true
	}
	if _, err := store.LoadState(); errors.Is(err, storage.ErrCorrupt) {
		if store.QuarantineCorrupt("state.json") != nil {
			os.Exit(1)
		}
		recovered = true
	}
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	manager := auth.NewManager(auth.NativeStore{}, auth.HTTPValidator{Client: client}, auth.GHCLI{})
	controller, err := appcore.New(store, manager, ghapi.NewClient(client), native, native)
	if err != nil {
		os.Exit(1)
	}
	if recovered {
		controller.SetWarning("Invalid local data was preserved and reset. Review your Settings and PR bells.")
	}
	native.warning = controller.SetWarning
	native.enabled = func(event core.Event) bool {
		snapshot, _ := controller.GetSnapshot()
		s := snapshot.Settings
		if !s.Notifications {
			return false
		}
		switch event {
		case core.EventCI:
			return s.Events.CI
		case core.EventApproval:
			return s.Events.Approval
		case core.EventReviewRequested:
			return s.Events.ReviewRequested
		case core.EventMerged:
			return s.Events.Merged
		case core.EventClosed:
			return s.Events.Closed
		}
		return false
	}
	service := &Service{controller: controller, native: native, smoke: smoke || preview, ready: make(chan struct{})}
	var smokeReady atomic.Bool
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if preview {
		logger = previewLogger()
	}
	app := application.New(application.Options{
		Name: "PR Nanny", Description: "A passive GitHub pull-request monitor", Icon: appIcon,
		Services: []application.Service{application.NewService(service)},
		Logger:   logger,
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets), Middleware: websecurity.Middleware, DisableLogging: true},
		Mac:      application.MacOptions{ActivationPolicy: application.ActivationPolicyAccessory},
		PostShutdown: func() {
			if smoke || preview {
				_ = os.RemoveAll(dataDir)
			}
			if smoke && !smokeReady.Load() {
				os.Exit(1)
			}
		},
	})
	native.app = app
	popup := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "popup", Title: "PR Nanny", Width: 440, Height: 600, Frameless: true, AlwaysOnTop: true, Hidden: !smoke,
		HideOnEscape: true, HideOnFocusLost: !smoke && !preview, DisableResize: true, DevToolsEnabled: false, DefaultContextMenuDisabled: true,
		Windows: application.WindowsWindow{HiddenOnTaskbar: true},
	})
	native.popup = popup
	popup.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) { popup.Hide(); e.Cancel() })
	tray := app.SystemTray.New()
	tray.SetTooltip("PR Nanny")
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(appIcon)
	}
	tray.AttachWindow(popup).WindowOffset(5)
	if smoke || preview {
		app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { popup.Show(); popup.Focus() })
	}
	menu := app.NewMenu()
	menu.Add("Settings").OnClick(func(*application.Context) { popup.SetURL("/?settings=1"); popup.Show(); popup.Focus() })
	menu.Add("Quit").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)
	if smoke {
		// Readiness requires the actual embedded webview to call the bound service.
		// This checks native launch + frontend + IPC, not physical tray interaction.
		go func() {
			select {
			case <-service.ready:
				smokeReady.Store(true)
				time.Sleep(500 * time.Millisecond)
			case <-time.After(75 * time.Second):
			}
			app.Quit()
		}()
	}
	if err := app.Run(); err != nil {
		os.Exit(1)
	}
	if smoke && !smokeReady.Load() {
		os.Exit(1)
	}
}
