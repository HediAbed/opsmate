package service

import "strings"

const commandResponseParts = 2

func parseCommandResponse(response string) (command, explanation string) {
	lines := strings.SplitN(response, "\n", commandResponseParts)
	command = strings.TrimSpace(lines[0])
	if len(lines) > 1 {
		explanation = strings.TrimSpace(lines[1])
	}
	return command, explanation
}
