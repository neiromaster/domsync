package app

import "errors"

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }

func (e *exitError) Unwrap() error { return e.err }

func withCode(code int, err error) error {
	if err == nil {
		return nil
	}
	return &exitError{code: code, err: err}
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var wrapped *exitError
	if errors.As(err, &wrapped) {
		return wrapped.code
	}
	return 1
}

type runLevelError struct {
	err error
}

func (e *runLevelError) Error() string { return e.err.Error() }

func (e *runLevelError) Unwrap() error { return e.err }

func withRunLevel(err error) error {
	if err == nil {
		return nil
	}
	return &runLevelError{err: err}
}

func isRunLevel(err error) bool {
	var target *runLevelError
	return errors.As(err, &target)
}
