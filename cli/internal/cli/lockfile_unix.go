//go:build unix

package cli

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive lock on path, creating it, and waits while
// another process (or goroutine) holds it. The lock goes with the process, so
// a crashed build leaves none behind.
func lockFile(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
