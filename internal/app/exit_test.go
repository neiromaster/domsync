package app

import (
	"errors"
	"testing"
)

func TestExitCode(t *testing.T) {
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("nil = %d", got)
	}
	if got := ExitCode(errors.New("x")); got != 1 {
		t.Fatalf("plain = %d", got)
	}
	if got := ExitCode(withCode(5, errors.New("x"))); got != 5 {
		t.Fatalf("wrapped = %d", got)
	}
}
