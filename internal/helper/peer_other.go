//go:build !darwin

package helper

import "net"

// The helper ships for macOS only; other platforms fail closed so the
// package still type-checks for analysis.
func authorizedPeer(*net.UnixConn) bool { return false }

// WatchProcess reports every process as gone on unsupported platforms.
func WatchProcess(int) <-chan struct{} {
	done := make(chan struct{})
	close(done)
	return done
}
