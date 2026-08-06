package controller

import (
	"fmt"
	"strings"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/komari"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// RouteCommand validates an incoming command against the pipe's authorization
// config (trusted groups / admins) and, if authorized, processes it through
// Trigger. It returns the response text to reply with; an empty response means
// the command was discarded.
//
// All command handling across pipes is unified here: pipes only decide whether
// a received message is a command, then hand the complete command over.
func (m *Manager) RouteCommand(cmd Command, trustedGroups, admins []string, listenMethod string) (string, error) {
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

	response, err := m.Trigger(cmd)
	if err != nil {
		return "", err
	}

	// In "at" listen mode, give feedback for unknown commands.
	if response == "" && listenMethod == "at" {
		return "Unknown command: /" + cmd.Command, nil
	}
	return response, nil
}

// Trigger processes a parsed bot command and returns the response text.
// The actual command execution for every pipe is unified here.
func (m *Manager) Trigger(cmd Command) (string, error) {
	cfg := config.GetConfig()
	if cfg.System.DebugMode && cfg.Debug.ShowTriggerCmdEcho {
		postLog.Debug("Trigger received command: /" + cmd.Command + " with args: " + strings.Join(cmd.Args, ", "))
	}

	switch cmd.Command {
	case "list":
		return handleList()
	case "status":
		return handleStatus(cmd)
	case "shutdown":
		return handleShutdown(cmd)
	case "reboot":
		return handleReboot(cmd)
	case "run":
		return handleRun(cmd)
	default:
		return "", nil
	}
}

func handleList() (string, error) {
	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	return template.Render(cfg.ControllerMessage.ServerList, params), nil
}

func handleStatus(cmd Command) (string, error) {
	if len(cmd.Args) < 1 {
		return "Usage: /status <uuid>", nil
	}
	uuid := cmd.Args[0]
	tracker := node.GetTracker()
	n, exists := tracker.GetNode(uuid)
	if !exists {
		return fmt.Sprintf("Server with UUID %s not found", uuid), nil
	}

	statusStr := "Offline"
	if n.Online {
		statusStr = "Online"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Server: %s (%s)\n", n.Name, n.UUID))
	sb.WriteString(fmt.Sprintf("Status: %s\n", statusStr))

	if n.LatestReport != nil {
		r := n.LatestReport
		sb.WriteString(fmt.Sprintf("CPU: %.2f%%\n", r.CPU.Usage))
		sb.WriteString(fmt.Sprintf("RAM: %d / %d\n", r.RAM.Used, r.RAM.Total))
		sb.WriteString(fmt.Sprintf("Disk: %d / %d\n", r.Disk.Used, r.Disk.Total))
		sb.WriteString(fmt.Sprintf("Network: ↑%d ↓%d\n", r.Network.Up, r.Network.Down))
		sb.WriteString(fmt.Sprintf("Uptime: %d seconds\n", r.Uptime))
		sb.WriteString(fmt.Sprintf("Processes: %d\n", r.Process))
		if r.Message != "" {
			sb.WriteString(fmt.Sprintf("Message: %s\n", r.Message))
		}
	}

	return sb.String(), nil
}

func handleShutdown(cmd Command) (string, error) {
	if len(cmd.Args) < 1 {
		return "Usage: /shutdown <uuid>", nil
	}

	uuid := cmd.Args[0]
	client := komari.GetClient()
	if client == nil {
		return "Error: Komari client not initialized", nil
	}

	_, err := client.ExecTask([]string{uuid}, "shutdown")
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	return fmt.Sprintf("Shutdown command sent to server %s", uuid), nil
}

func handleReboot(cmd Command) (string, error) {
	if len(cmd.Args) < 1 {
		return "Usage: /reboot <uuid>", nil
	}

	uuid := cmd.Args[0]
	client := komari.GetClient()
	if client == nil {
		return "Error: Komari client not initialized", nil
	}

	_, err := client.ExecTask([]string{uuid}, "reboot")
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}

	return fmt.Sprintf("Reboot command sent to server %s", uuid), nil
}

func handleRun(cmd Command) (string, error) {
	if len(cmd.Args) < 2 {
		return "Usage: /run <uuid|all> <command>", nil
	}

	uuidArg := cmd.Args[0]
	command := cmd.Args[1]
	client := komari.GetClient()
	if client == nil {
		return "Error: Komari client not initialized", nil
	}

	var uuids []string
	if uuidArg == "all" {
		tracker := node.GetTracker()
		for _, n := range tracker.GetAllNodes() {
			uuids = append(uuids, n.UUID)
		}
	} else {
		uuids = []string{uuidArg}
	}

	taskID, err := client.ExecTask(uuids, command)
	if err != nil {
		return fmt.Sprintf("Error executing command: %v", err), nil
	}

	results, err := client.PollTaskResult(taskID)
	if err != nil {
		return fmt.Sprintf("Error getting results: %v", err), nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromExecResult(uuidArg, uuidArg, command, formatTaskResults(results))
	return template.Render(cfg.ControllerMessage.ServerExecuteResult, params), nil
}

func formatTaskResults(results []komari.TaskResult) string {
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("--- %s ---\n", r.Client))
		sb.WriteString(r.Result)
		sb.WriteString(fmt.Sprintf("\nExit code: %d\n", r.ExitCode))
	}
	return sb.String()
}
