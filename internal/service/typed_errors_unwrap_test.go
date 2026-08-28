package service

import (
	"errors"
	"testing"
)

func TestCRDError_UnwrapReturnsInner(t *testing.T) {
	inner := errors.New("inner")
	e := &CRDError{Operation: "list", Err: inner}
	if got := e.Unwrap(); !errors.Is(got, inner) {
		t.Errorf("Unwrap = %v, want inner err", got)
	}
}

func TestHelmError_UnwrapReturnsInner(t *testing.T) {
	inner := errors.New("inner")
	e := &HelmError{Err: inner}
	if got := e.Unwrap(); !errors.Is(got, inner) {
		t.Errorf("Unwrap = %v, want inner err", got)
	}
}

func TestCRDError_ErrorWithoutResource(t *testing.T) {
	e := &CRDError{Operation: "list", Err: errors.New("denied")}
	if got := e.Error(); got != "crd list: denied" {
		t.Errorf("Error = %q, want 'crd list: denied'", got)
	}
}

func TestCRDError_ErrorWithResource(t *testing.T) {
	e := &CRDError{Operation: "list-instances", Resource: "x.example.com", Err: errors.New("denied")}
	got := e.Error()
	if got != "crd list-instances x.example.com: denied" {
		t.Errorf("Error = %q", got)
	}
}

func TestCRDError_ErrorUnknownWhenCauseMissing(t *testing.T) {
	e := &CRDError{Operation: "list"}
	if got := e.Error(); got != "crd list: unknown error" {
		t.Errorf("Error = %q", got)
	}
}

func TestHelmError_ErrorPrefersStderr(t *testing.T) {
	e := &HelmError{Stderr: "release nope not found", Err: errors.New("exit 1")}
	if got := e.Error(); got != "helm: release nope not found" {
		t.Errorf("Error should prefer stderr; got %q", got)
	}
}

func TestHelmError_ErrorFallsBackToErr(t *testing.T) {
	e := &HelmError{Err: errors.New("connect refused")}
	if got := e.Error(); got != "helm: connect refused" {
		t.Errorf("Error should fall back to err; got %q", got)
	}
}

func TestHelmError_ErrorUnknownWhenBlank(t *testing.T) {
	e := &HelmError{}
	if got := e.Error(); got != "helm: unknown error" {
		t.Errorf("Error with no fields = %q", got)
	}
}
