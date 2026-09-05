package screen

import (
	"errors"

	"github.com/HediAbed/opsmate/internal/failure"
	"github.com/HediAbed/opsmate/internal/kube"
)

const (
	accessNoticePrefix   = "Not permitted: "
	accessNoticeFallback = "Not permitted for this account"
)

func AccessDenied(err error) bool {
	var kubeErr *kube.Error
	return errors.As(err, &kubeErr) && failure.IsPermissionDenied(err)
}

func AccessNotice(err error) string {
	var kubeErr *kube.Error
	if !errors.As(err, &kubeErr) || kubeErr.Subject == "" {
		return accessNoticeFallback
	}
	notice := accessNoticePrefix + accessVerb(kubeErr.Operation) + " " + string(kubeErr.Subject)
	if kubeErr.Identifier != "" {
		notice += " " + kubeErr.Identifier
	}
	return notice
}

func accessVerb(operation kube.Operation) string {
	if operation == kube.OperationObserve {
		return string(kube.OperationList)
	}
	return string(operation)
}
