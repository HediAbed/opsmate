package failure

import (
	"context"
	"errors"
)

type Code string

const (
	CodeOK                 Code = "ok"
	CodeUnknown            Code = "unknown"
	CodeCanceled           Code = "canceled"
	CodeDeadlineExceeded   Code = "deadline_exceeded"
	CodeInvalidArgument    Code = "invalid_argument"
	CodeFailedPrecondition Code = "failed_precondition"
	CodeNotFound           Code = "not_found"
	CodeAlreadyExists      Code = "already_exists"
	CodePermissionDenied   Code = "permission_denied"
	CodeUnauthenticated    Code = "unauthenticated"
	CodeConflict           Code = "conflict"
	CodeRateLimited        Code = "rate_limited"
	CodeUnavailable        Code = "unavailable"
	CodeInternal           Code = "internal"
)

type Coded interface {
	FailureCode() Code
}

func CodeOf(err error) Code {
	if err == nil {
		return CodeOK
	}
	var coded Coded
	if errors.As(err, &coded) {
		return normalize(coded.FailureCode())
	}
	if errors.Is(err, context.Canceled) {
		return CodeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return CodeDeadlineExceeded
	}
	return CodeUnknown
}

func normalize(code Code) Code {
	switch code {
	case CodeUnknown,
		CodeCanceled,
		CodeDeadlineExceeded,
		CodeInvalidArgument,
		CodeFailedPrecondition,
		CodeNotFound,
		CodeAlreadyExists,
		CodePermissionDenied,
		CodeUnauthenticated,
		CodeConflict,
		CodeRateLimited,
		CodeUnavailable,
		CodeInternal:
		return code
	case CodeOK:
		return CodeUnknown
	}
	return CodeUnknown
}
