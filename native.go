package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

type nativePlatform struct {
	app           *application.App
	popup         *application.WebviewWindow
	notifications *notifications.NotificationService
	mu            sync.Mutex
	ready         bool
	isolated      bool
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
	p.mu.Lock()
	p.ready = err == nil
	p.mu.Unlock()
}
func (p *nativePlatform) Notify(pr model.PullRequest, event core.Event) {
	p.mu.Lock()
	ready := p.ready
	p.mu.Unlock()
	if !ready {
		return
	}
	allowed, err := p.notifications.CheckNotificationAuthorization()
	if err != nil {
		return
	}
	if !allowed {
		allowed, err = p.notifications.RequestNotificationAuthorization()
		if err != nil || !allowed {
			return
		}
	}
	title := map[core.Event]string{core.EventCI: "CI status changed", core.EventApproval: "Approval status changed", core.EventReviewRequested: "Review requested", core.EventMerged: "Pull request merged", core.EventClosed: "Pull request closed"}[event]
	if title == "" {
		return
	}
	id := sha256.Sum256([]byte(fmt.Sprintf("%s:%s:%s:%s:%t", pr.ID, event, pr.UpdatedAt.String(), pr.CI, pr.Approved)))
	_ = p.notifications.SendNotification(notifications.NotificationOptions{
		ID: fmt.Sprintf("%x", id[:16]), Title: title, Subtitle: fmt.Sprintf("%s #%d", pr.Repository, pr.Number), Body: pr.Title,
	})
}
