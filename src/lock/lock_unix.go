//go:build !windows

package lock

import (
	"os"

	"golang.org/x/sys/unix"
)

func OpenFile(path string, flag int, perm os.FileMode, lockExternal bool) (*os.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}

	if lockExternal {
		if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
			_ = f.Close()
			return nil, err
		}
	}

	return f, nil
}

func UnlockAndClose(f *os.File) error {
	if f == nil {
		return nil
	}
	_ = unix.Flock(int(f.Fd()), unix.LOCK_UN)
	return f.Close()
}
