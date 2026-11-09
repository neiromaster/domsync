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
