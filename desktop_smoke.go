package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

const desktopSmokeAutostartID = "io.github.alexcatdad.pr-nanny.smoke"

// runDesktopSmoke checks real tray, autostart, and notification integrations
// without authenticating, changing normal app preferences, or using real PR data.
func runDesktopSmoke(ctx context.Context, app *application.App, popup *application.WebviewWindow, tray *application.SystemTray, api notificationAPI) (err error) {
	if ctx == nil || app == nil || popup == nil || tray == nil || api == nil {
		return errors.New("desktop smoke setup: required native service is unavailable")
	}
	configCleanup, err := isolateDesktopSmokeConfig()
	if err != nil {
		return desktopSmokeError("temporary config", err)
	}
	defer func() {
		if cleanupErr := configCleanup(); cleanupErr != nil {
			err = errors.Join(err, desktopSmokeError("temporary config cleanup", cleanupErr))
		}
	}()
	if err := ctx.Err(); err != nil {
		return desktopSmokeError("context", err)
	}
	if err := probeTray(ctx, popup, tray); err != nil {
		return desktopSmokeError("tray", err)
	}

	status, err := app.Autostart.Status()
	if err != nil {
		return desktopSmokeError("autostart initial status", err)
	}
	if status.Enabled {
		return errors.New("desktop smoke autostart: registration already exists for this executable")
	}

	// Always attempt cleanup after the initial clean-state check, including
	// when enabling, verifying, or disabling the temporary registration fails.
	autostartClean := false
	defer func() {
		if autostartClean {
			return
		}
		if cleanupErr := app.Autostart.Disable(); cleanupErr != nil {
			err = errors.Join(err, desktopSmokeError("autostart cleanup", cleanupErr))
		}
	}()

	if err := ctx.Err(); err != nil {
		return desktopSmokeError("context", err)
	}
	if err := app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: desktopSmokeAutostartID}); err != nil {
		return desktopSmokeError("autostart enable", err)
	}
	status, err = app.Autostart.Status()
	if err != nil {
		return desktopSmokeError("autostart enabled status", err)
	}
	if !status.Enabled {
		return errors.New("desktop smoke autostart: enabled registration was not visible")
	}
	if err := app.Autostart.Disable(); err != nil {
		return desktopSmokeError("autostart disable", err)
	}
	status, err = app.Autostart.Status()
	if err != nil {
		return desktopSmokeError("autostart disabled status", err)
	}
	if status.Enabled {
		return errors.New("desktop smoke autostart: registration remained enabled after disable")
	}
	autostartClean = true

	// Shutdown runs even after a partial startup so a failed probe cannot leave
	// service resources open. This function is called only by the explicit CI
	// desktop-smoke path, which uses a unique app identity and temporary state.
	defer func() {
		if shutdownErr := api.ServiceShutdown(); shutdownErr != nil {
			err = errors.Join(err, desktopSmokeError("notification shutdown", shutdownErr))
		}
	}()
	if err := ctx.Err(); err != nil {
		return desktopSmokeError("context", err)
	}
	if err := api.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		return desktopSmokeError("notification startup", err)
	}
	allowed, err := api.CheckNotificationAuthorization()
	if err != nil {
		return desktopSmokeError("notification permission check", err)
	}
	if !allowed {
		return errors.New("desktop smoke notifications: operating system permission is unavailable")
	}
	if err := ctx.Err(); err != nil {
		return desktopSmokeError("context", err)
	}
	if err := api.SendNotification(notifications.NotificationOptions{
		ID:       "pr-nanny-desktop-smoke",
		Title:    "PR Nanny desktop smoke test",
		Body:     "Synthetic notification from the isolated CI smoke test.",
		Subtitle: "No GitHub data was used",
	}); err != nil {
		return desktopSmokeError("notification send", err)
	}
	return nil
}

func desktopSmokeError(stage string, err error) error {
	return fmt.Errorf("desktop smoke %s: %w", stage, err)
}

func isolateDesktopSmokeConfig() (func() error, error) {
	if runtime.GOOS != "linux" {
		return func() error { return nil }, nil
	}
	dir, err := os.MkdirTemp("", "pr-nanny-desktop-smoke-config-")
	if err != nil {
		return nil, err
	}
	previous, hadPrevious := os.LookupEnv("XDG_CONFIG_HOME")
	if err := os.Setenv("XDG_CONFIG_HOME", dir); err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return func() error {
		var restoreErr error
		if hadPrevious {
			restoreErr = os.Setenv("XDG_CONFIG_HOME", previous)
		} else {
			restoreErr = os.Unsetenv("XDG_CONFIG_HOME")
		}
		return errors.Join(restoreErr, os.RemoveAll(dir))
	}, nil
}
