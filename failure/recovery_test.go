package failure

import (
	"errors"
	"fmt"
	"testing"
)

func TestRecoveryOfEveryCode(t *testing.T) {
	tests := []struct {
		code Code
		want Recovery
	}{
		{code: CodeOK, want: RecoveryPermanent},
		{code: Code(""), want: RecoveryPermanent},
		{code: Code("made_up"), want: RecoveryPermanent},
		{code: CodeUnknown, want: RecoveryPermanent},
		{code: CodeCanceled, want: RecoveryPermanent},
		{code: CodeDeadlineExceeded, want: RecoveryRetryable},
		{code: CodeInvalidArgument, want: RecoveryPermanent},
		{code: CodeFailedPrecondition, want: RecoveryPermanent},
		{code: CodeNotFound, want: RecoveryPermanent},
		{code: CodeAlreadyExists, want: RecoveryPermanent},
		{code: CodePermissionDenied, want: RecoveryPermanent},
		{code: CodeUnauthenticated, want: RecoveryPermanent},
		{code: CodeConflict, want: RecoveryRetryable},
		{code: CodeRateLimited, want: RecoveryRetryable},
		{code: CodeUnavailable, want: RecoveryRetryable},
		{code: CodeInternal, want: RecoveryPermanent},
	}
	for _, test := range tests {
		t.Run(string(test.code), func(t *testing.T) {
			if got := RecoveryOf(codedError{code: test.code}); got != test.want {
				t.Fatalf("RecoveryOf(code %q) = %d, want %d", test.code, got, test.want)
			}
		})
	}
}

func TestRecoveryOfErrorChains(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Recovery
	}{
		{name: "nil", want: RecoveryNone},
		{name: "plain", err: errors.New("plain failure"), want: RecoveryPermanent},
		{name: "wrapped coded", err: fmt.Errorf("wrapped: %w", codedError{code: CodeRateLimited}), want: RecoveryRetryable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := RecoveryOf(test.err); got != test.want {
				t.Fatalf("RecoveryOf(%v) = %d, want %d", test.err, got, test.want)
			}
		})
	}
}
