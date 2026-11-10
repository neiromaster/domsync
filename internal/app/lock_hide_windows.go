package app

import "golang.org/x/sys/windows"

func hideLockFile(path string) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return
	}
	attrs, err := windows.GetFileAttributes(ptr)
	if err != nil {
		return
	}
	attrs &^= windows.FILE_ATTRIBUTE_NORMAL
	_ = windows.SetFileAttributes(ptr, attrs|windows.FILE_ATTRIBUTE_HIDDEN)
}
