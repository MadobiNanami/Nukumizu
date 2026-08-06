package controller

import (
	"strings"

	"nukumizu-backend/config"
	"nukumizu-backend/postLog"
)

// Trigger is a placeholder for triggering commands across all controllers.
func (m *Manager) Trigger (command string, args []string) {
	if config.GetConfig().System.DebugMode {
		postLog.Debug("Received command: " + command + " with args: " + strings.Join(args, ", "))
	}
}

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
