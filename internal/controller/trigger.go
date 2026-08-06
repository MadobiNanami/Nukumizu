package controller

import (
	"fmt"
	"strings"
)

// ParseCommand parses a raw message text into a Command.
// Format: /{{command}} {{args...}}
func ParseCommand(rawText string) (cmd Command, ok bool) {
	rawText = strings.TrimSpace(rawText)
	if !strings.HasPrefix(rawText, "/") {
		return Command{}, false
	}

	// Remove the leading slash and split.
	parts := strings.SplitN(rawText[1:], " ", 2)
	cmd.Command = strings.ToLower(parts[0])
	cmd.RawText = rawText

	if len(parts) > 1 {
		argStr := strings.TrimSpace(parts[1])

		// Special handling for /run: first arg is uuid, rest is command.
		if cmd.Command == "run" {
			spaceIdx := strings.Index(argStr, " ")
			if spaceIdx > 0 {
				cmd.Args = []string{argStr[:spaceIdx], strings.TrimSpace(argStr[spaceIdx+1:])}
			} else {
				cmd.Args = []string{argStr}
			}
		} else {
			cmd.Args = strings.Fields(argStr)
		}
	}

	return cmd, true
}

// IsAdminCommand returns whether the given command requires admin privileges.
func IsAdminCommand(command string) bool {
	switch command {
	case "shutdown", "reboot", "run":
		return true
	default:
		return false
	}
}

// IsAdmin returns whether the sender is present in the given admins list.
func IsAdmin(senderID int64, admins []string) bool {
	senderStr := fmt.Sprintf("%d", senderID)
	for _, admin := range admins {
		if admin == senderStr {
			return true
		}
	}
	return false
}

// IsTrustedGroup returns whether the chat ID is present in the given trusted groups list.
func IsTrustedGroup(chatID int64, trustedGroups []string) bool {
	chatStr := fmt.Sprintf("%d", chatID)
	for _, group := range trustedGroups {
		if group == chatStr {
			return true
		}
	}
	return false
}
