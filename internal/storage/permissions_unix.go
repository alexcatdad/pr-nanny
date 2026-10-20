//go:build !windows

package storage

import "os"

func protectDirectory(path string) error {
	// Directories require owner execute permission for traversal; files use 0600.
	return os.Chmod(path, 0o700) // #nosec G302 -- 0700 protects a private directory, not a regular file.
}

func protectFile(path string) error { return os.Chmod(path, 0o600) }
