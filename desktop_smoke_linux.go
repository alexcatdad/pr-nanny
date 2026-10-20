//go:build linux && !android && !server

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/wailsapp/wails/v3/pkg/application"
)

// probeTray activates this process's exported StatusNotifierItem and verifies
// the attached popup follows the same native hide/show path as a panel click.
func probeTray(ctx context.Context, popup *application.WebviewWindow, _ *application.SystemTray) error {
	if popup == nil || !popup.IsVisible() {
		return fmt.Errorf("smoke popup was not initially visible")
	}
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return fmt.Errorf("connect to session bus")
	}
	defer conn.Close()

	name := fmt.Sprintf("org.kde.StatusNotifierItem-%d-1", os.Getpid())
	if err := waitStatusNotifierOwner(ctx, conn, name); err != nil {
		return fmt.Errorf("wait for exported tray item")
	}
	item := conn.Object(name, dbus.ObjectPath("/StatusNotifierItem"))
	callActivate := func() error {
		callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return item.CallWithContext(callCtx, "org.kde.StatusNotifierItem.Activate", 0, int32(0), int32(0)).Err
	}
	if err := callActivate(); err != nil {
		return fmt.Errorf("activate exported tray item")
	}
	if err := waitPopupVisibility(ctx, popup, false); err != nil {
		return fmt.Errorf("tray activation did not hide popup")
	}
	if err := callActivate(); err != nil {
		return fmt.Errorf("activate exported tray item again")
	}
	if err := waitPopupVisibility(ctx, popup, true); err != nil {
		return fmt.Errorf("second tray activation did not show popup")
	}
	return nil
}

func waitStatusNotifierOwner(ctx context.Context, conn *dbus.Conn, name string) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		var owner string
		callCtx, cancel := context.WithTimeout(ctx, time.Second)
		err := conn.BusObject().CallWithContext(callCtx, "org.freedesktop.DBus.GetNameOwner", 0, name).Store(&owner)
		cancel()
		if err == nil && owner != "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("tray item did not register")
		case <-ticker.C:
		}
	}
}

func waitPopupVisibility(ctx context.Context, popup *application.WebviewWindow, visible bool) error {
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if popup.IsVisible() == visible {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-deadline.C:
			return fmt.Errorf("visibility state did not change")
		case <-ticker.C:
		}
	}
}
