package command

import (
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestErrorFailureCode(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want failure.Code
	}{
		{name: "missing cause", err: &Error{}, want: failure.CodeUnknown},
		{name: "empty command", err: &Error{Err: ErrEmptyCommand}, want: failure.CodeInvalidArgument},
		{name: "forbidden command", err: &Error{Err: ErrForbiddenCommand}, want: failure.CodePermissionDenied},
		{name: "sensitive data", err: &Error{Err: ErrSensitiveDataCommand}, want: failure.CodePermissionDenied},
		{name: "namespace scope", err: &Error{Err: ErrCommandScope}, want: failure.CodePermissionDenied},
		{name: "unknown cause", err: &Error{Err: errors.New("failed")}, want: failure.CodeUnknown},
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

	var nilError *Error
	if got := nilError.Error(); got != "kubectl policy: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("nil policy error did not return the empty error contract")
	}
}
