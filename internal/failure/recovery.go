package failure

type Recovery uint8

const (
	RecoveryNone Recovery = iota
	RecoveryPermanent
	RecoveryRetryable
)

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
