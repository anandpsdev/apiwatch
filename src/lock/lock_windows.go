//go:build windows

package lock

import (
	"io"
	"os"

	"golang.org/x/sys/windows"
)

func OpenFile(path string, flag int, perm os.FileMode, lockExternal bool) (*os.File, error) {
	if !lockExternal {
		return os.OpenFile(path, flag, perm)
	}

	path16, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}

	var access uint32 = windows.GENERIC_READ | windows.GENERIC_WRITE
	var disposition uint32 = windows.OPEN_EXISTING
	if flag&os.O_CREATE != 0 {
		disposition = windows.OPEN_ALWAYS
	}

	var shareMode uint32 = 0

	h, err := windows.CreateFile(
		path16,
		access,
		shareMode,
		nil,
		disposition,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return nil, err
	}

	f := os.NewFile(uintptr(h), path)
	if flag&os.O_APPEND != 0 {
		if _, err := f.Seek(0, io.SeekEnd); err != nil {
			f.Close()
			return nil, err
		}
	}
	return f, nil
}

func UnlockAndClose(f *os.File) error {
	if f == nil {
		return nil
	}
	return f.Close()
}
