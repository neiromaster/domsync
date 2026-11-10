package app

import (
	"fmt"

	"github.com/gofrs/flock"
)

type runLock struct {
	file *flock.Flock
}

func lockPath(configPath string) string {
	return configPath + ".domsync.lock"
}

func acquireRunLock(path string) (*runLock, error) {
	file := flock.New(path)
	locked, err := file.TryLock()
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", path, err)
	}
	if !locked {
		return nil, fmt.Errorf("another domsync run is in progress (lock %s)", path)
	}
	return &runLock{file: file}, nil
}

func (l *runLock) release() error {
	if err := l.file.Unlock(); err != nil {
		return fmt.Errorf("unlock %s: %w", l.file.Path(), err)
	}
	return nil
}
