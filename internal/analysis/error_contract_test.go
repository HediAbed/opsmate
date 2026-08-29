package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/failure"
)

func TestCommandPolicyErrorFailureCode(t *testing.T) {
	tests := []struct {
		name string
		err  *CommandPolicyError
		want failure.Code
	}{
		{name: "missing cause", err: &CommandPolicyError{}, want: failure.CodeUnknown},
		{name: "empty command", err: &CommandPolicyError{Err: ErrEmptyCommand}, want: failure.CodeInvalidArgument},
		{name: "forbidden command", err: &CommandPolicyError{Err: ErrForbiddenCommand}, want: failure.CodePermissionDenied},
		{name: "sensitive data", err: &CommandPolicyError{Err: ErrSensitiveDataCommand}, want: failure.CodePermissionDenied},
		{name: "namespace scope", err: &CommandPolicyError{Err: ErrCommandScope}, want: failure.CodePermissionDenied},
		{name: "unknown cause", err: &CommandPolicyError{Err: errors.New("failed")}, want: failure.CodeUnknown},
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

	var nilError *CommandPolicyError
	if got := nilError.Error(); got != "kubectl policy: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("nil policy error did not return the empty error contract")
	}
}

func TestScreenContextErrorFailureCode(t *testing.T) {
	tests := []struct {
		name string
		err  *ScreenContextError
		want failure.Code
	}{
		{name: "missing cause", err: &ScreenContextError{}, want: failure.CodeUnknown},
		{name: "unsupported resource", err: &ScreenContextError{Err: ErrUnsupportedBrowserResource}, want: failure.CodeInvalidArgument},
		{name: "internal", err: &ScreenContextError{Err: errors.New("encode failed")}, want: failure.CodeInternal},
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

	var nilError *ScreenContextError
	if got := nilError.Error(); got != "screen context: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("nil screen-context error did not return the empty error contract")
	}
}

func TestSSEStreamErrorFailureCode(t *testing.T) {
	tests := []struct {
		name string
		err  *SSEStreamError
		want failure.Code
	}{
		{name: "missing cause", err: &SSEStreamError{}, want: failure.CodeUnknown},
		{name: "canceled", err: &SSEStreamError{Err: context.Canceled}, want: failure.CodeCanceled},
		{name: "deadline", err: &SSEStreamError{Err: context.DeadlineExceeded}, want: failure.CodeDeadlineExceeded},
		{name: "oversized event", err: &SSEStreamError{Stage: StreamStageRead, Err: ErrSSEEventTooLarge}, want: failure.CodeInternal},
		{name: "oversized stream", err: &SSEStreamError{Stage: StreamStageLimit, Err: ErrSSEStreamTooLarge}, want: failure.CodeInternal},
		{name: "read", err: &SSEStreamError{Stage: StreamStageRead, Err: errors.New("read failed")}, want: failure.CodeUnavailable},
		{name: "decode", err: &SSEStreamError{Stage: StreamStageDecode, Err: errors.New("decode failed")}, want: failure.CodeInternal},
		{name: "limit", err: &SSEStreamError{Stage: StreamStageLimit, Err: errors.New("limit failed")}, want: failure.CodeInternal},
		{name: "unknown stage", err: &SSEStreamError{Stage: "other", Err: errors.New("failed")}, want: failure.CodeUnknown},
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

	var nilError *SSEStreamError
	if got := nilError.Error(); got != "SSE stream: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("nil stream error did not return the empty error contract")
	}
}
