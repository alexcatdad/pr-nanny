package main

import (
	"context"
	"errors"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alexcatdad/pr-nanny/internal/core"
	"github.com/alexcatdad/pr-nanny/internal/model"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

func TestTrayDisplayBackend(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GTK backend selection is Linux-specific")
	}
	for _, tc := range []struct {
		name, display, wayland, backend, want string
	}{
		{"mixed session", ":0", "wayland-0", "", "x11,wayland"},
		{"explicit Wayland", ":0", "wayland-0", "wayland", "wayland"},
		{"Wayland only", "", "wayland-0", "", ""},
		{"X11 only", ":0", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DISPLAY", tc.display)
			t.Setenv("WAYLAND_DISPLAY", tc.wayland)
			t.Setenv("GDK_BACKEND", tc.backend)
			configureTrayDisplay()
			if got := os.Getenv("GDK_BACKEND"); got != tc.want {
				t.Fatalf("GDK_BACKEND = %q, want %q", got, tc.want)
			}
		})
	}
}

type fakeNotifications struct {
	mu             sync.Mutex
	startupErr     error
	shutdowns      int
	checkAllowed   bool
	checkErr       error
	requestAllowed bool
	requestErr     error
	sendErr        error
	request        func() (bool, error)
	requests       int
	sends          []notifications.NotificationOptions
}

func (f *fakeNotifications) ServiceStartup(context.Context, application.ServiceOptions) error {
	return f.startupErr
}
func (f *fakeNotifications) ServiceShutdown() error {
	f.mu.Lock()
	f.shutdowns++
	f.mu.Unlock()
	return nil
}
func (f *fakeNotifications) CheckNotificationAuthorization() (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkAllowed, f.checkErr
}
func (f *fakeNotifications) RequestNotificationAuthorization() (bool, error) {
	f.mu.Lock()
	f.requests++
	f.mu.Unlock()
	var allowed bool
	var err error
	if f.request != nil {
		allowed, err = f.request()
	} else {
		allowed, err = f.requestAllowed, f.requestErr
	}
	f.mu.Lock()
	f.checkAllowed = allowed
	f.mu.Unlock()
	return allowed, err
}
func (f *fakeNotifications) SendNotification(opts notifications.NotificationOptions) error {
	f.mu.Lock()
	f.sends = append(f.sends, opts)
	f.mu.Unlock()
	return f.sendErr
}

func (f *fakeNotifications) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.requests, len(f.sends)
}

func testPR() model.PullRequest {
	return model.PullRequest{ID: "pr-1", Repository: "acme/widgets", Number: 42, Title: "Add a widget"}
}

func TestNativeNotificationRequestsAuthorizationOncePerSession(t *testing.T) {
	api := &fakeNotifications{requestAllowed: true}
	p := &nativePlatform{notifications: api}
	p.deliver(testPR(), core.EventCI)
	p.deliver(testPR(), core.EventApproval)
	requests, sends := api.counts()
	if requests != 1 || sends != 2 {
		t.Fatalf("requests=%d sends=%d, want one permission request and two sends", requests, sends)
	}
}

func TestNativeNotificationDenialIsNotPromptedAgainThisSession(t *testing.T) {
	api := &fakeNotifications{requestAllowed: false}
	p := &nativePlatform{notifications: api}
	p.deliver(testPR(), core.EventCI)
	p.deliver(testPR(), core.EventApproval)
	requests, sends := api.counts()
	if requests != 1 || sends != 0 {
		t.Fatalf("requests=%d sends=%d, want one denied request and no sends", requests, sends)
	}
}

func TestNativeNotificationSettingChangedDuringPermissionPromptSuppressesSend(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	api := &fakeNotifications{request: func() (bool, error) {
		close(started)
		<-release
		return true, nil
	}}
	var enabled atomic.Bool
	enabled.Store(true)
	p := &nativePlatform{notifications: api, enabled: func(string, core.Event) bool { return enabled.Load() }}
	done := make(chan struct{})
	go func() {
		defer close(done)
		p.deliver(testPR(), core.EventCI)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("permission request did not start")
	}
	enabled.Store(false)
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("delivery did not finish")
	}
	requests, sends := api.counts()
	if requests != 1 || sends != 0 {
		t.Fatalf("requests=%d sends=%d, want one request and no send after disabling", requests, sends)
	}
}

func TestNativeNotificationStartupFailureUsesSafeWarning(t *testing.T) {
	api := &fakeNotifications{startupErr: errors.New("private platform detail")}
	var warning string
	p := &nativePlatform{notifications: api, warning: func(message string) { warning = message }}
	p.startNotifications(context.Background(), application.ServiceOptions{})
	if warning != "OS notifications are unavailable. Check your desktop notification service and permissions." {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if strings.Contains(warning, "private platform detail") {
		t.Fatalf("platform error leaked into warning: %q", warning)
	}
	if p.dispatcher != nil {
		t.Fatal("dispatcher started despite notification startup failure")
	}
}

func TestNativeNotificationStopCancelsDispatcherAndShutsDownService(t *testing.T) {
	api := &fakeNotifications{}
	p := &nativePlatform{notifications: api}
	p.startNotifications(context.Background(), application.ServiceOptions{})
	if p.dispatcher == nil || p.cancel == nil {
		t.Fatal("notification dispatcher was not started")
	}
	p.stopNotifications()
	if p.dispatcher != nil {
		t.Fatal("dispatcher remained available after stop")
	}
	api.mu.Lock()
	shutdowns := api.shutdowns
	api.mu.Unlock()
	if shutdowns != 1 {
		t.Fatalf("ServiceShutdown called %d times, want 1", shutdowns)
	}
}

func TestNativeNotificationSendFailureUsesSafeWarning(t *testing.T) {
	api := &fakeNotifications{checkAllowed: true, sendErr: errors.New("private platform detail")}
	var warning string
	p := &nativePlatform{notifications: api, warning: func(message string) { warning = message }}
	p.deliver(testPR(), core.EventCI)
	if warning != "An OS notification could not be delivered. Check your desktop notification service and permissions." {
		t.Fatalf("unexpected warning: %q", warning)
	}
	if strings.Contains(warning, "private platform detail") {
		t.Fatalf("platform error leaked into warning: %q", warning)
	}
}

func TestNotificationTextEscapesLinuxMarkupOnly(t *testing.T) {
	malicious := `<a href="https://example.invalid">open</a><img src="file:///tmp/private.png">`
	if got, want := notificationText(malicious, "linux"), `&lt;a href=&#34;https://example.invalid&#34;&gt;open&lt;/a&gt;&lt;img src=&#34;file:///tmp/private.png&#34;&gt;`; got != want {
		t.Fatalf("Linux notification text = %q, want %q", got, want)
	}
	for _, platform := range []string{"darwin", "windows"} {
		if got := notificationText(malicious, platform); got != malicious {
			t.Fatalf("%s notification text changed: %q", platform, got)
		}
	}

}
