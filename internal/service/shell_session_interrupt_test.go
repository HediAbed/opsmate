package service

import (
	"errors"
	"testing"
)

func TestShellSession_Interrupt_NilSessionReturnsError(t *testing.T) {
	var s *ShellSession
	if err := s.Interrupt(); !errors.Is(err, ErrShellNoProcess) {
		t.Errorf("nil session interrupt = %v, want ErrShellNoProcess", err)
	}
}

func TestShellSession_Interrupt_NoProcessReturnsError(t *testing.T) {
	s := &ShellSession{}
	if err := s.Interrupt(); !errors.Is(err, ErrShellNoProcess) {
		t.Errorf("session without cmd = %v, want ErrShellNoProcess", err)
	}
}
