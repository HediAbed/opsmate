// Package failure defines the shared failure contract: stable failure
// codes, the operation vocabulary, and recovery classification.
package failure

import (
	"context"
	"errors"
)

// Code identifies a stable failure category shared across modules.
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

// Coded reports a module-specific failure code.
type Coded interface {
	FailureCode() Code
}

// CodeOf classifies an error chain. A Coded error is authoritative even
// when it wraps a context cancellation or deadline error, but its code is
// normalized: CodeOK is returned only for a nil error, and any code outside
// the defined non-OK constants becomes CodeUnknown.
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

// normalize keeps a reported failure code inside the stable non-OK
// vocabulary. CodeOK, the empty code, and undefined codes become
// CodeUnknown because the reporting error is non-nil.
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
