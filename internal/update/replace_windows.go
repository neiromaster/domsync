//go:build windows

package update

import (
	"fmt"
	"os"
	"path/filepath"
)

func ReplaceExecutable(target string, data []byte) error {
	old := target + ".old"
	_ = os.Remove(old)
	if err := os.Rename(target, old); err != nil {
		return fmt.Errorf("move current binary: %w", err)
	}
	if err := writeNew(target, data); err != nil {
		if rerr := os.Rename(old, target); rerr != nil {
			return fmt.Errorf("install new binary: %w (restore failed: %v)", err, rerr)
		}
		return err
	}
	_ = os.Remove(old)
	return nil
}

func writeNew(target string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".domsync-update-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o755); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, target); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}
