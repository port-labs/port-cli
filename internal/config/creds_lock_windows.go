//go:build windows

package config

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

func withFileLock(lockPath string, fn func() error) error {
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("failed opening credentials lock: %w", err)
	}
	defer f.Close()

	handle := windows.Handle(f.Fd())
	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(
		handle,
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,
		1,
		0,
		overlapped,
	); err != nil {
		return fmt.Errorf("failed acquiring credentials lock: %w", err)
	}
	defer windows.UnlockFileEx(handle, 0, 1, 0, overlapped)

	return fn()
}
