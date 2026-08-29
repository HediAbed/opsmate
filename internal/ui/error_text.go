package ui

import "fmt"

const shellNamespaceRequiredMessage = "shell: namespace required (select an explicit namespace first)"

func operationErrorText(action string, err error) string {
	detail := "unknown error"
	if err != nil {
		detail = err.Error()
	}
	return sanitizeTerminalLine(action + ": " + detail)
}

func batchAllNamespacesErrorText(action string) string {
	return "batch " + action + " is not supported in all-namespaces mode; pick one namespace first"
}

func shellPodPhaseErrorText(name, status string) string {
	return sanitizeTerminalLine(fmt.Sprintf("shell: pod %q is in phase %q; can only shell into Running pods", name, status))
}

func analysisErrorText(err error) string {
	return "Analysis error: " + sanitizeTerminalLine(err.Error())
}
