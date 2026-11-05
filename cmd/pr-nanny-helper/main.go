// Command pr-nanny-helper owns PR Nanny's GitHub monitoring, credentials and
// local state, and serves them to the menu-bar app over a private unix socket
// (see internal/helper). It exits when the process that started it exits.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	appcore "github.com/alexcatdad/pr-nanny/internal/app"
	"github.com/alexcatdad/pr-nanny/internal/auth"
	ghapi "github.com/alexcatdad/pr-nanny/internal/github"
	"github.com/alexcatdad/pr-nanny/internal/helper"
	"github.com/alexcatdad/pr-nanny/internal/notify"
	"github.com/alexcatdad/pr-nanny/internal/storage"
)

// Packaging sets version via -ldflags "-X main.version=…".
var version = "dev"

const (
	exitFailure        = 1
	exitAlreadyRunning = 3
)

func main() {
	showVersion := flag.Bool("version", false, "print the version and exit")
	dataDir := flag.String("data-dir", "", "private data directory (default ~/Library/Application Support/pr-nanny)")
	flag.Parse()
	if *showVersion {
		fmt.Printf("pr-nanny-helper %s (%s) protocol %d\n", version, commit(), helper.ProtocolVersion)
		return
	}
	os.Exit(run(*dataDir))
}

func run(dir string) int {
	if dir == "" {
		base, err := os.UserConfigDir()
		if err != nil {
			return fail("the user data directory is unavailable")
		}
		dir = filepath.Join(base, "pr-nanny")
	}
	lock, err := storage.LockInstance(dir)
	if errors.Is(err, storage.ErrLocked) {
		fmt.Fprintln(os.Stderr, "pr-nanny-helper: another PR Nanny process is already running")
		return exitAlreadyRunning
	}
	if err != nil {
		return fail("the instance lock is unavailable")
	}
	defer func() { _ = lock.Close() }()

	store := storage.New(dir)
	// Preserve corrupted non-secret data and restart with a fresh baseline.
	recovered := false
	for _, load := range []struct {
		name string
		err  func() error
	}{
		{"settings.json", func() error { _, err := store.Settings(); return err }},
		{"state.json", func() error { _, err := store.LoadState(); return err }},
	} {
		if errors.Is(load.err(), storage.ErrCorrupt) {
			if store.QuarantineCorrupt(load.name) != nil {
				return fail("local data could not be recovered")
			}
			recovered = true
		}
	}
	client := &http.Client{Timeout: 25 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	manager := auth.NewManager(auth.NativeStore{}, auth.HTTPValidator{Client: client}, auth.GHCLI{})
	queue := notify.NewQueue()
	// Launch at login and opening URLs belong to the app, so no Autostart.
	controller, err := appcore.New(store, manager, ghapi.NewClient(client), queue, nil)
	if err != nil {
		return fail("the backend could not start")
	}
	defer controller.Stop()
	queue.SetFilter(controller.NotificationAllowed)
	if recovered {
		controller.SetWarning("Invalid local data was preserved and reset. Review your Settings and PR bells.")
	}

	listener, err := helper.Listen(filepath.Join(dir, helper.SocketName))
	if err != nil {
		return fail("the helper socket could not be created")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	if parent := os.Getppid(); parent > 1 {
		go func() {
			select {
			case <-helper.WatchProcess(parent):
				cancel()
			case <-ctx.Done():
			}
		}()
	}
	go controller.Run()
	server := &helper.Server{Backend: controller, Notifications: queue, Version: version, Commit: commit(), OnStop: cancel}
	_ = server.Serve(ctx, listener)
	return 0
}

// fail prints a fixed message; upstream error text is never printed.
func fail(message string) int {
	fmt.Fprintln(os.Stderr, "pr-nanny-helper:", message)
	return exitFailure
}

func commit() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				return setting.Value
			}
		}
	}
	return "unknown"
}
