package helper

import (
	"errors"

	"golang.org/x/sys/unix"
)

// WatchProcess returns a channel that closes when process pid exits (or is
// already gone). The helper uses it to exit when the app that launched it
// goes away, even if the app crashes without sending stop.
func WatchProcess(pid int) <-chan struct{} {
	done := make(chan struct{})
	kq, err := unix.Kqueue()
	if err != nil {
		close(done)
		return done
	}
	change := unix.Kevent_t{Ident: uint64(pid), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT} // #nosec G115 -- pid is positive.
	go func() {
		defer close(done)
		defer func() { _ = unix.Close(kq) }()
		events := make([]unix.Kevent_t, 1)
		changes := []unix.Kevent_t{change}
		for {
			_, err := unix.Kevent(kq, changes, events, nil)
			if errors.Is(err, unix.EINTR) {
				changes = nil
				continue
			}
			return
		}
	}()
	return done
}
