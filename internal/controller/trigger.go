package controller

import (
	"fmt"
	"strings"
)

// Trigger validates an incoming command against the pipe's authorization
// config (trusted groups / admins) and, if authorized, processes it through
// RouteCommand. It returns the response text to reply with; an empty response means
// the command was discarded.
//
// All command handling across pipes is unified here: pipes only decide whether
// a received message is a command, then hand the complete command over.
func (m *Manager) Trigger(cmd Command, trustedGroups, admins []string, listenMethod string) (string, error) {
	// Check whether the command arrived in a group or a private chat.
	if cmd.ChatType == "group" {
		// Group commands are only honored from trusted groups, otherwise discard.
		if !IsTrustedGroup(cmd.ChatID, trustedGroups) {
			return "", nil
		}
	}

	// Commands that require special permission must be sent by an admin.
	if IsAdminCommand(cmd.Command) {
		if !IsAdmin(cmd.SenderID, admins) {
			return "", nil
		}
	}

	response, err := m.RouteCommand(cmd)
	if err != nil {
		return "", err
	}

	// In "at" listen mode, give feedback for unknown commands.
	if response == "" && listenMethod == "at" {
		return "Unknown command: /" + cmd.Command, nil
	}
	return response, nil
}

// RouteCommand processes a parsed bot command and returns the response text.
// The actual command execution for every pipe is unified here.
func (m *Manager) RouteCommand(cmd Command) (string, error) {
	if cmd.Source == "telegram"{
		switch cmd.Command {
		case "start":
			return telegram_handleStart()
		}
	}
	switch cmd.Command {
	case "help":
		return handleHelp(cmd)
	case "list":
		return handleList(cmd)
	case "status":
		return handleStatus(cmd)
	case "shutdown":
		return handleShutdown(cmd)
	case "reboot":
		return handleReboot(cmd)
	case "run":
		return handleRun(cmd)
	case "info":
		return handleInfo(cmd)
	case "getip":
		return handleGetIP(cmd)
	default:
		return "", nil
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
