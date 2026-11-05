package storage

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

// ErrLocked means another PR Nanny process already owns the data directory.
var ErrLocked = errors.New("another PR Nanny process is using this data directory")

// ErrLockUnavailable means the instance lock could not be created safely.
var ErrLockUnavailable = errors.New("the PR Nanny instance lock is unavailable")

// InstanceLock is an advisory flock on <dir>/pr-nanny.lock held for the
// process lifetime, so only one process reads and writes settings and state.
type InstanceLock struct{ file *os.File }

// LockInstance creates the private data directory if needed and takes the
// exclusive, non-blocking instance lock. The kernel releases it on exit.
func LockInstance(dir string) (*InstanceLock, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, ErrLockUnavailable
	}
	if err := protectDirectory(dir); err != nil {
		return nil, ErrLockUnavailable
	}
	path := filepath.Join(dir, "pr-nanny.lock")
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_CLOEXEC|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, ErrLockUnavailable
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 || !ownedByCurrentUser(info) {
		_ = file.Close()
		return nil, ErrLockUnavailable
	}
	for {
		err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if errors.Is(err, syscall.EWOULDBLOCK) {
		_ = file.Close()
		return nil, ErrLocked
	}
	if err != nil {
		_ = file.Close()
		return nil, ErrLockUnavailable
	}
	return &InstanceLock{file: file}, nil
}

// Close releases the lock. It is safe to call more than once.
func (l *InstanceLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	err := l.file.Close()
	l.file = nil
	return err
}

func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}
