package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"html"
	"os"
	"runtime"
	"sync"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/alexcatdad/pr-nanny/internal/notify"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Wails beta.33 can position tray windows only through X11. Prefer XWayland
// on mixed sessions, without overriding an explicit backend or requiring X11
// on a desktop that only provides Wayland.
func configureTrayDisplay() {
	if runtime.GOOS == "linux" && os.Getenv("GDK_BACKEND") == "" &&
		os.Getenv("DISPLAY") != "" && os.Getenv("WAYLAND_DISPLAY") != "" {
		_ = os.Setenv("GDK_BACKEND", "x11,wayland")
	}
}

type nativePlatform struct {
	app           *application.App
	popup         *application.WebviewWindow
	notifications notificationAPI
	mu            sync.Mutex
	dispatcher    *notify.Dispatcher
	cancel        context.CancelFunc
	warning       func(string)
	enabled       func(string, core.Event) bool
	authAttempted bool // accessed only by the single delivery worker
	isolated      bool
}

// notificationAPI keeps platform notification behavior replaceable in native
// adapter tests while matching the Wails beta.33 service methods we use.
type notificationAPI interface {
	ServiceStartup(context.Context, application.ServiceOptions) error
	ServiceShutdown() error
	CheckNotificationAuthorization() (bool, error)
	RequestNotificationAuthorization() (bool, error)
	SendNotification(notifications.NotificationOptions) error
}

func (p *nativePlatform) SetAutostart(enabled bool) error {
	if p.isolated {
		return nil
	}
	if enabled {
		return p.app.Autostart.EnableWithOptions(application.AutostartOptions{Identifier: "io.github.alexcatdad.pr-nanny"})
	}
	return p.app.Autostart.Disable()
}
func (p *nativePlatform) OpenURL(url string) error { return p.app.Browser.OpenURL(url) }
func (p *nativePlatform) HidePopup()               { p.popup.Hide() }
func (p *nativePlatform) startNotifications(ctx context.Context, options application.ServiceOptions) {
	err := p.notifications.ServiceStartup(ctx, options)
	if err != nil {
		p.warn("OS notifications are unavailable. Check your desktop notification service and permissions.")
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	p.mu.Lock()
	p.cancel = cancel
	p.dispatcher = notify.New(ctx, p.deliver, func() {
		p.warn("Some OS notifications could not be queued. Check notification permissions.")
	})
	p.mu.Unlock()
}
func (p *nativePlatform) Notify(pr model.PullRequest, event core.Event) {
	p.mu.Lock()
	dispatcher := p.dispatcher
	p.mu.Unlock()
	if dispatcher != nil {
		dispatcher.Notify(pr, event)
	}
}
func (p *nativePlatform) stopNotifications() {
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.dispatcher = nil
	p.mu.Unlock()
	_ = p.notifications.ServiceShutdown()
}
func (p *nativePlatform) warn(message string) {
	if p.warning != nil {
		p.warning(message)
	}
}
func (p *nativePlatform) deliver(pr model.PullRequest, event core.Event) {
	if p.enabled != nil && !p.enabled(pr.ID, event) {
		return
	}
	allowed, err := p.notifications.CheckNotificationAuthorization()
	if err != nil {
		p.warn("OS notification permissions could not be checked. Review your operating system settings.")
		return
	}
	if !allowed {
		if p.authAttempted {
			return
		}
		p.authAttempted = true
		allowed, err = p.notifications.RequestNotificationAuthorization()
		if err != nil || !allowed {
			p.warn("OS notifications could not be enabled. Allow PR Nanny notifications in your operating system settings.")
			return
		}
	}
	// Settings can change while an operating-system permission prompt is open.
	if p.enabled != nil && !p.enabled(pr.ID, event) {
		return
	}
	title := map[core.Event]string{core.EventCI: "CI status changed", core.EventApproval: "Approval status changed", core.EventReviewRequested: "Review requested", core.EventMerged: "Pull request merged", core.EventClosed: "Pull request closed"}[event]
	if title == "" {
		return
	}
	id := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s:%t", pr.ID, event, pr.UpdatedAt.String(), pr.CI, pr.Approved)))
	if err := p.notifications.SendNotification(notifications.NotificationOptions{
		ID: fmt.Sprintf("%x", id[:16]), Title: title, Subtitle: notificationText(fmt.Sprintf("%s #%d", pr.Repository, pr.Number), runtime.GOOS), Body: notificationText(pr.Title, runtime.GOOS),
	}); err != nil {
		p.warn("An OS notification could not be delivered. Check your desktop notification service and permissions.")
	}
}

func notificationText(value, platform string) string {
	// Freedesktop notification bodies support XML markup, links and images.
	// Wails forwards Linux body/subtitle strings directly to the D-Bus daemon.
	if platform == "linux" {
		return html.EscapeString(value)
	}
	return value
}
