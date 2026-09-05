package browser

import (
	"fmt"

	"github.com/HediAbed/opsmate/internal/terminal"
	"github.com/HediAbed/opsmate/internal/ui/screen"
	"github.com/HediAbed/opsmate/internal/ui/theme"
)

const shellNamespaceRequiredMessage = "shell: namespace required (select an explicit namespace first)"

func operationErrorText(action string, err error) string {
	detail := "unknown error"
	if err != nil {
		detail = err.Error()
	}
	return terminal.SanitizeLine(action + ": " + detail)
}

func accessNoticeStatus(err error) string {
	return theme.Notice.Render(screen.AccessNotice(err))
}

func (m *BrowserModel) reportOperationError(action string, err error) {
	if screen.AccessDenied(err) {
		m.statusMsg = accessNoticeStatus(err)
		return
	}
	m.errBanner = operationErrorText(action, err)
}

func (m *BrowserModel) applyListError(err error) {
	m.loading = false
	m.err = err
	if screen.AccessDenied(err) {
		m.errBanner = ""
		return
	}
	m.errBanner = terminal.SanitizeLine(err.Error())
}

func (m BrowserModel) listDenied() bool {
	return screen.AccessDenied(m.err)
}

func batchAllNamespacesErrorText(action string) string {
	return "batch " + action + " is not supported in all-namespaces mode; pick one namespace first"
}

func shellPodPhaseErrorText(name, status string) string {
	return terminal.SanitizeLine(fmt.Sprintf("shell: pod %q is in phase %q; can only shell into Running pods", name, status))
}

func analysisErrorText(err error) string {
	return "Analysis error: " + terminal.SanitizeLine(err.Error())
}
