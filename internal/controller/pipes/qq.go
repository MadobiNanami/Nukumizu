package pipes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/komari"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
	"nukumizu-backend/internal/controller"
)

// QQController handles QQ Bot interactions via napcat-bridge.
type QQController struct {
	cfg        config.QQConfig
	httpClient *http.Client
}

// NewQQController creates a new QQ (Napcat) controller.
func NewQQController(cfg config.QQConfig) *QQController {
	return &QQController{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Name returns the controller name.
func (q *QQController) Name() string {
	return "qq(napcat)"
}

// Start initializes the QQ controller.
func (q *QQController) Start() error {
	if !q.cfg.Enabled {
		postLog.Info("QQ (Napcat) controller is disabled")
		return nil
	}
	postLog.Info("QQ (Napcat) controller started")
	return nil
}

// Stop shuts down the QQ controller.
func (q *QQController) Stop() {
	postLog.Info("QQ (Napcat) controller stopped")
}

// IsEnabled returns whether the controller is enabled.
func (q *QQController) IsEnabled() bool {
	return q.cfg.Enabled
}

// HandleCommand processes a bot command and returns a response string.
func (q *QQController) HandleCommand(cmd controller.Command) (string, error) {
	parsed, ok := controller.ParseCommand(cmd.RawText)
	if !ok {
		// Not a command. In "global" mode, silently ignore.
		// In "at" mode, this would be an error - but at detection happens at the message level.
		return "", nil
	}

	// Check listen method.
	if q.cfg.ListenMethod == "at" {
		// At detection: check if message contains an @mention for our bot.
		atMention := fmt.Sprintf("[CQ:at,qq=%d]", q.cfg.BotQQID)
		if !strings.Contains(cmd.RawText, atMention) {
			return "", nil // Not mentioned, ignore.
		}
	}

	parsed.ChatID = cmd.ChatID
	parsed.ChatType = cmd.ChatType
	parsed.SenderID = cmd.SenderID

	return q.executeCommand(parsed)
}

func (q *QQController) executeCommand(cmd controller.Command) (string, error) {
	// Validate against supported commands.
	switch cmd.Command {
	case "list":
		return q.handleList()
	case "status":
		return q.handleStatus(cmd)
	case "shutdown":
		return q.handleShutdown(cmd)
	case "reboot":
		return q.handleReboot(cmd)
	case "run":
		return q.handleRun(cmd)
	default:
		if q.cfg.ListenMethod == "at" {
			return "Unknown command: /" + cmd.Command, nil
		}
		return "", nil // Global mode: silently ignore unknown commands.
	}
}

func (q *QQController) isAdmin(senderID int64) bool {
	senderStr := fmt.Sprintf("%d", senderID)
	for _, admin := range q.cfg.Admins {
		if admin == senderStr {
			return true
		}
	}
	return false
}

func (q *QQController) handleList() (string, error) {
	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	return template.Render(cfg.ControllerMessage.ServerList, params), nil
}

func (q *QQController) handleStatus(cmd controller.Command) (string, error) {
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

func (q *QQController) handleShutdown(cmd controller.Command) (string, error) {
	if !q.isAdmin(cmd.SenderID) {
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

func (q *QQController) handleReboot(cmd controller.Command) (string, error) {
	if !q.isAdmin(cmd.SenderID) {
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

func (q *QQController) handleRun(cmd controller.Command) (string, error) {
	if !q.isAdmin(cmd.SenderID) {
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

// SendStatusChange sends a status change notification via QQ.
func (q *QQController) SendStatusChange(change node.StatusChange) error {
	if !q.cfg.Enabled {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	// Send to trusted groups.
	for _, groupID := range q.cfg.TrustedGroups {
		q.sendGroupMessage(groupID, message)
	}

	// Send to admins via private message.
	for _, adminID := range q.cfg.Admins {
		q.sendPrivateMessage(adminID, message)
	}

	return nil
}

// SendServerList sends the server list via QQ.
func (q *QQController) SendServerList(onlineServers, offlineServers string) error {
	if !q.cfg.Enabled {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params)

	for _, groupID := range q.cfg.TrustedGroups {
		q.sendGroupMessage(groupID, message)
	}
	return nil
}

// SendExecuteResult sends a command execution result via QQ.
func (q *QQController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !q.cfg.Enabled {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params)

	for _, groupID := range q.cfg.TrustedGroups {
		q.sendGroupMessage(groupID, message)
	}
	return nil
}

func (q *QQController) sendGroupMessage(groupID string, message string) {
	var groupIDInt int64
	fmt.Sscanf(groupID, "%d", &groupIDInt)

	body := map[string]interface{}{
		"targetType": "group",
		"targetID":   groupIDInt,
		"message":    message,
	}
	bodyJSON, _ := json.Marshal(body)

	resp, err := q.httpClient.Post(
		q.cfg.URL+"/api/msg/send",
		"application/json",
		bytes.NewReader(bodyJSON),
	)
	if err != nil {
		postLog.Warning(fmt.Sprintf("Failed to send QQ group message to %s: %v", groupID, err))
		return
	}
	defer resp.Body.Close()
}

func (q *QQController) sendPrivateMessage(userID string, message string) {
	var userIDInt int64
	fmt.Sscanf(userID, "%d", &userIDInt)

	body := map[string]interface{}{
		"targetType": "private",
		"targetID":   userIDInt,
		"message":    message,
	}
	bodyJSON, _ := json.Marshal(body)

	resp, err := q.httpClient.Post(
		q.cfg.URL+"/api/msg/send",
		"application/json",
		bytes.NewReader(bodyJSON),
	)
	if err != nil {
		postLog.Warning(fmt.Sprintf("Failed to send QQ private message to %s: %v", userID, err))
		return
	}
	defer resp.Body.Close()
}
