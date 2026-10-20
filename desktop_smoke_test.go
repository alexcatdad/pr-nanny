package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsolateDesktopSmokeConfigRestoresEnvironmentAndRemovesFiles(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux notification service stores categories under XDG_CONFIG_HOME")
	}
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "previous-config"))
	previous := os.Getenv("XDG_CONFIG_HOME")
	cleanup, err := isolateDesktopSmokeConfig()
	if err != nil {
		t.Fatal(err)
	}
	temporary := os.Getenv("XDG_CONFIG_HOME")
	if temporary == previous {
		t.Fatal("smoke config reused the normal config directory")
	}
	if err := os.WriteFile(filepath.Join(temporary, "marker"), []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if got := os.Getenv("XDG_CONFIG_HOME"); got != previous {
		t.Fatalf("XDG_CONFIG_HOME = %q, want restored value %q", got, previous)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatalf("temporary config directory still exists: %v", err)
	}
}
