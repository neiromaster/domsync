package app

import (
	"io/fs"
	"os"
	"path/filepath"
)

func writeLocalAtomic(path string, data []byte) (err error) {
	target := path
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil {
		target = resolved
	}
	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(target); statErr == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".domsync-*.tmp")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
		}
	}()
	if _, err = tmp.Write(data); err != nil {
		return err
	}
	if err = tmp.Sync(); err != nil {
		return err
	}
	if err = tmp.Chmod(mode); err != nil {
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), target)
}
