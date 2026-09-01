package browser

import (
	"fmt"

	"github.com/HediAbed/opsmate/internal/terminal"
)

const shellNamespaceRequiredMessage = "shell: namespace required (select an explicit namespace first)"

func operationErrorText(action string, err error) string {
	detail := "unknown error"
	if err != nil {
		detail = err.Error()
	}
	return terminal.SanitizeLine(action + ": " + detail)
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
