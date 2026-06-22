//go:build unix

package store

import (
	"os"
	"syscall"
)

// lockFile opens (creating if needed) path and takes a blocking exclusive flock
// on it. The lock is advisory and is released when the returned function runs
// (closing the descriptor) or when the process exits, so a crash never leaves a
// stale lock behind.
func lockFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, err
	}
	return f.Close, nil
}
