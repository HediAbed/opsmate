package failure

// Recovery states whether a failure is worth retrying.
type Recovery uint8

const (
	// RecoveryNone marks the absence of a failure.
	RecoveryNone Recovery = iota
	// RecoveryPermanent marks failures that retrying cannot fix.
	RecoveryPermanent
	// RecoveryRetryable marks transient failures.
	RecoveryRetryable
)

// RecoveryOf classifies an error chain by its failure code. RecoveryNone
// is possible only for a nil error because CodeOf never returns CodeOK
// for a non-nil error.
func RecoveryOf(err error) Recovery {
	code := CodeOf(err)
	if code == CodeOK {
		return RecoveryNone
	}
	if code == CodeDeadlineExceeded ||
		code == CodeConflict ||
		code == CodeRateLimited ||
		code == CodeUnavailable {
		return RecoveryRetryable
	}
	return RecoveryPermanent
}
