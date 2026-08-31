package session

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestSessionErrorContract(t *testing.T) {
	tests := []struct {
		name string
		err  *SessionError
		want failure.Code
	}{
		{name: "missing cause", err: &SessionError{}, want: failure.CodeUnknown},
		{name: "canceled", err: &SessionError{Err: context.Canceled}, want: failure.CodeCanceled},
		{name: "deadline", err: &SessionError{Err: context.DeadlineExceeded}, want: failure.CodeDeadlineExceeded},
		{name: "missing session", err: &SessionError{Err: ErrNoSession}, want: failure.CodeNotFound},
		{name: "missing file", err: &SessionError{Err: os.ErrNotExist}, want: failure.CodeNotFound},
		{name: "unsafe path", err: &SessionError{Err: ErrUnsafeSession}, want: failure.CodeInvalidArgument},
		{name: "oversized state", err: &SessionError{Err: ErrSessionTooLarge}, want: failure.CodeInvalidArgument},
		{name: "permission", err: &SessionError{Err: os.ErrPermission}, want: failure.CodePermissionDenied},
		{name: "internal", err: &SessionError{Err: errors.New("read failed")}, want: failure.CodeInternal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.err.FailureCode(); got != test.want {
				t.Fatalf("FailureCode() = %q, want %q", got, test.want)
			}
			if got := failure.CodeOf(test.err); got != test.want {
				t.Fatalf("CodeOf() = %q, want %q", got, test.want)
			}
		})
	}

	var nilError *SessionError
	if got := nilError.Error(); got != "session: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil {
		t.Fatal("nil error unwrapped to a cause")
	}
	if got := nilError.FailureCode(); got != failure.CodeUnknown {
		t.Fatalf("nil FailureCode() = %q", got)
	}
}
