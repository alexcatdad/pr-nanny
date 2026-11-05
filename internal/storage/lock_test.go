package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLockInstanceIsExclusiveAndReleasable(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "pr-nanny")
	first, err := LockInstance(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := LockInstance(dir); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock error = %v, want ErrLocked", err)
	}
	info, err := os.Stat(filepath.Join(dir, "pr-nanny.lock"))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("lock file mode = %v, %v", info, err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	again, err := LockInstance(dir)
	if err != nil {
		t.Fatalf("lock after release: %v", err)
	}
	_ = again.Close()
}

func TestLockInstanceRejectsSymlinkAndLooseLockFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "pr-nanny.lock")); err != nil {
		t.Fatal(err)
	}
	if _, err := LockInstance(dir); !errors.Is(err, ErrLockUnavailable) {
		t.Fatalf("symlinked lock error = %v", err)
	}
	loose := t.TempDir()
	if err := os.WriteFile(filepath.Join(loose, "pr-nanny.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LockInstance(loose); !errors.Is(err, ErrLockUnavailable) {
		t.Fatalf("group-readable lock error = %v", err)
	}
}
