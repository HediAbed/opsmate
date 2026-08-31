package analysis

import (
	"context"
	"errors"
	"testing"

	"github.com/HediAbed/opsmate/internal/analysis/provider"
	"github.com/HediAbed/opsmate/internal/failure"
)

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
		err  *provider.SSEStreamError
		want failure.Code
	}{
		{name: "missing cause", err: &provider.SSEStreamError{}, want: failure.CodeUnknown},
		{name: "canceled", err: &provider.SSEStreamError{Err: context.Canceled}, want: failure.CodeCanceled},
		{name: "deadline", err: &provider.SSEStreamError{Err: context.DeadlineExceeded}, want: failure.CodeDeadlineExceeded},
		{name: "oversized event", err: &provider.SSEStreamError{Stage: provider.StreamStageRead, Err: provider.ErrSSEEventTooLarge}, want: failure.CodeInternal},
		{name: "oversized stream", err: &provider.SSEStreamError{Stage: provider.StreamStageLimit, Err: provider.ErrSSEStreamTooLarge}, want: failure.CodeInternal},
		{name: "read", err: &provider.SSEStreamError{Stage: provider.StreamStageRead, Err: errors.New("read failed")}, want: failure.CodeUnavailable},
		{name: "decode", err: &provider.SSEStreamError{Stage: provider.StreamStageDecode, Err: errors.New("decode failed")}, want: failure.CodeInternal},
		{name: "limit", err: &provider.SSEStreamError{Stage: provider.StreamStageLimit, Err: errors.New("limit failed")}, want: failure.CodeInternal},
		{name: "unknown stage", err: &provider.SSEStreamError{Stage: "other", Err: errors.New("failed")}, want: failure.CodeUnknown},
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

	var nilError *provider.SSEStreamError
	if got := nilError.Error(); got != "SSE stream: unknown error" {
		t.Fatalf("nil Error() = %q", got)
	}
	if nilError.Unwrap() != nil || nilError.FailureCode() != failure.CodeUnknown {
		t.Fatal("nil stream error did not return the empty error contract")
	}
}
