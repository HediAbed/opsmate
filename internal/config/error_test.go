package config

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/HediAbed/opsmate/internal/failure"
)

func TestDotEnvErrorContract(t *testing.T) {
	tests := []struct {
		name string
		err  *DotEnvError
		want failure.Code
	}{
		{name: "missing cause", err: &DotEnvError{}, want: failure.CodeUnknown},
		{name: "canceled", err: &DotEnvError{Err: context.Canceled}, want: failure.CodeCanceled},
		{name: "deadline", err: &DotEnvError{Err: context.DeadlineExceeded}, want: failure.CodeDeadlineExceeded},
		{name: "invalid line", err: &DotEnvError{Err: ErrInvalidDotEnvLine}, want: failure.CodeInvalidArgument},
		{name: "permission", err: &DotEnvError{Err: os.ErrPermission}, want: failure.CodePermissionDenied},
		{name: "internal", err: &DotEnvError{Err: errors.New("read failed")}, want: failure.CodeInternal},
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

	var nilError *DotEnvError
	if got := nilError.Error(); got != "dotenv: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil {
		t.Fatal("nil error unwrapped to a cause")
	}
	if got := nilError.FailureCode(); got != failure.CodeUnknown {
		t.Fatalf("nil FailureCode() = %q", got)
	}
}
