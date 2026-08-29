package failure

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type codedError struct {
	code Code
	err  error
}

func (codedError) Error() string {
	return "coded failure"
}

func (e codedError) Unwrap() error {
	return e.err
}

func (e codedError) FailureCode() Code {
	return e.code
}

func TestCodeOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Code
	}{
		{name: "nil", want: CodeOK},
		{name: "plain", err: errors.New("plain failure"), want: CodeUnknown},
		{name: "coded", err: codedError{code: CodeConflict}, want: CodeConflict},
		{name: "wrapped coded", err: fmt.Errorf("wrapped: %w", codedError{code: CodeUnavailable}), want: CodeUnavailable},
		{name: "canceled", err: context.Canceled, want: CodeCanceled},
		{name: "wrapped canceled", err: fmt.Errorf("wrapped: %w", context.Canceled), want: CodeCanceled},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: CodeDeadlineExceeded},
		{name: "wrapped deadline exceeded", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), want: CodeDeadlineExceeded},
		{name: "coded wrapping canceled", err: codedError{code: CodeInternal, err: context.Canceled}, want: CodeInternal},
		{name: "coded wrapping deadline exceeded", err: codedError{code: CodeNotFound, err: context.DeadlineExceeded}, want: CodeNotFound},
		{name: "coded ok normalizes", err: codedError{code: CodeOK}, want: CodeUnknown},
		{name: "coded empty normalizes", err: codedError{code: Code("")}, want: CodeUnknown},
		{name: "coded undefined normalizes", err: codedError{code: Code("made_up")}, want: CodeUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CodeOf(test.err); got != test.want {
				t.Fatalf("CodeOf(%v) = %q, want %q", test.err, got, test.want)
			}
		})
	}
}
