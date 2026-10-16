//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// lockFile takes an exclusive lock on path, creating it, and waits while
// another process (or goroutine) holds it. The lock goes with the process, so
// a crashed build leaves none behind.
func lockFile(path string) (unlock func(), err error) {
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	ol := new(windows.Overlapped)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 1, 0, ol); err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}
