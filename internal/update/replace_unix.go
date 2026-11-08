//go:build !windows

package update

import (
	"fmt"
	"os"
	"path/filepath"
)

func ReplaceExecutable(target string, data []byte) error {
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
