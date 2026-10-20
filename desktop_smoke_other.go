//go:build !linux || android || server

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// probeTray exercises Wails' attached-window toggle on platforms where a
// portable native tray activation injection is unavailable.
func probeTray(ctx context.Context, popup *application.WebviewWindow, tray *application.SystemTray) error {
	if popup == nil || tray == nil || !popup.IsVisible() {
		return fmt.Errorf("smoke popup was not initially visible")
	}
	tray.ToggleWindow()
	if err := waitPopupVisibility(ctx, popup, false); err != nil {
		return fmt.Errorf("tray toggle did not hide popup")
	}
	tray.ToggleWindow()
	if err := waitPopupVisibility(ctx, popup, true); err != nil {
		return fmt.Errorf("second tray toggle did not show popup")
	}
	return nil
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
