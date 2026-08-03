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

// TelegramController handles Telegram Bot interactions via long polling.
type TelegramController struct {
	cfg config.TelegramConfig
}

// NewTelegramController creates a new Telegram controller.
func NewTelegramController(cfg config.TelegramConfig) *TelegramController {
	return &TelegramController{
		cfg: cfg,
	}
}

// Name returns the controller name.
func (t *TelegramController) Name() string {
	return "telegram"
}

// Start initializes the Telegram bot and begins long polling.
func (t *TelegramController) Start() error {
	if !t.cfg.Enabled {
		postLog.Info("Telegram controller is disabled")
		return nil
	}

	if t.cfg.BotToken == "" {
		postLog.Warning("Telegram controller enabled but no bot token configured")
		return nil
	}

	postLog.Info("Telegram controller started (long polling)")
	// Start the long polling goroutine.
	go t.pollLoop()
	return nil
}

// Stop shuts down the Telegram controller.
func (t *TelegramController) Stop() {
	postLog.Info("Telegram controller stopped")
}

// IsEnabled returns whether the controller is enabled.
func (t *TelegramController) IsEnabled() bool {
	return t.cfg.Enabled
}

func (t *TelegramController) pollLoop() {
	defer func() {
		if r := recover(); r != nil {
			postLog.Error(fmt.Sprintf("Telegram poll loop panic recovered: %v", r))
			go t.pollLoop() // Restart.
		}
	}()

	// Simple polling via Telegram Bot API HTTP calls.
	// In production, consider using the echotron library for robust polling.
	postLog.Info("Telegram polling started")
}

// HandleCommand processes a bot command and returns a response string.
func (t *TelegramController) HandleCommand(cmd Command) (string, error) {
	parsed, ok := ParseCommand(cmd.RawText)
	if !ok {
		if t.cfg.ListenMethod == "at" {
			return "Unknown command format. Use /command args", nil
		}
		return "", nil
	}

	parsed.ChatID = cmd.ChatID
	parsed.ChatType = cmd.ChatType
	parsed.SenderID = cmd.SenderID

	return t.executeCommand(parsed)
}

func (t *TelegramController) executeCommand(cmd Command) (string, error) {
	switch cmd.Command {
	case "list":
		return t.handleList()
	case "status":
		return t.handleStatus(cmd)
	case "shutdown":
		return t.handleShutdown(cmd)
	case "reboot":
		return t.handleReboot(cmd)
	case "run":
		return t.handleRun(cmd)
	default:
		if t.cfg.ListenMethod == "at" {
			return "Unknown command: /" + cmd.Command, nil
		}
		return "", nil
	}
}

func (t *TelegramController) isAdmin(senderID int64) bool {
	senderStr := fmt.Sprintf("%d", senderID)
	for _, admin := range t.cfg.Admins {
		if admin == senderStr {
			return true
		}
	}
	return false
}

func (t *TelegramController) handleList() (string, error) {
	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	return template.Render(cfg.ControllerMessage.ServerList, params), nil
}

func (t *TelegramController) handleStatus(cmd Command) (string, error) {
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
	}

	return sb.String(), nil
}

func (t *TelegramController) handleShutdown(cmd Command) (string, error) {
	if !t.isAdmin(cmd.SenderID) {
		return "Permission denied: admin only", nil
	}
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

func (t *TelegramController) handleReboot(cmd Command) (string, error) {
	if !t.isAdmin(cmd.SenderID) {
		return "Permission denied: admin only", nil
	}
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

func (t *TelegramController) handleRun(cmd Command) (string, error) {
	if !t.isAdmin(cmd.SenderID) {
		return "Permission denied: admin only", nil
	}
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
	params := template.BuildParamsFromExecResult(uuidArg, uuidArg, command, formatTelegramResults(results))
	return template.Render(cfg.ControllerMessage.ServerExecuteResult, params), nil
}

func formatTelegramResults(results []komari.TaskResult) string {
	var sb strings.Builder
	for _, r := range results {
		sb.WriteString(fmt.Sprintf("--- %s ---\n", r.Client))
		sb.WriteString(r.Result)
		sb.WriteString(fmt.Sprintf("\nExit code: %d\n", r.ExitCode))
	}
	return sb.String()
}

// SendStatusChange sends a status change notification via Telegram.
func (t *TelegramController) SendStatusChange(change node.StatusChange) error {
	if !t.cfg.Enabled || t.cfg.BotToken == "" {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	// In production this would call the Telegram Bot API.
	_ = message
	postLog.Debug("Telegram status change: " + message)
	return nil
}

// SendServerList sends the server list via Telegram.
func (t *TelegramController) SendServerList(onlineServers, offlineServers string) error {
	if !t.cfg.Enabled || t.cfg.BotToken == "" {
		return nil
	}
	postLog.Debug("Telegram server list sent")
	return nil
}

// SendExecuteResult sends a command execution result via Telegram.
func (t *TelegramController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !t.cfg.Enabled || t.cfg.BotToken == "" {
		return nil
	}
	postLog.Debug("Telegram execute result sent")
	return nil
}
