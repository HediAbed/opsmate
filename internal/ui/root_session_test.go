package ui

import (
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/session"
)

func TestRootPersistSessionSurfacesStorageFailure(t *testing.T) {
	storageFailure := errors.New("storage failed")
	model := freshRoot(t)
	model.saveSessionState = func(session.SessionState) error { return storageFailure }

	model.persistSession()

	if model.err == nil || !errors.Is(model.err, storageFailure) {
		t.Fatalf("persistence error = %v, want storage failure", model.err)
	}
}
