package qq_napcat

import (
	"encoding/json"
	"fmt"
	"strings"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// oneBotEvent mirrors a OneBot 11 event pushed over the NapCat WebSocket.
// Only "message" events are handled; notice/request/meta_event are ignored.
type oneBotEvent struct {
	PostType    string          `json:"post_type"`    // Identifies "message", "notice", "request", or "meta_event"
	MessageType string          `json:"message_type"` // Identifies "private" or "group"
	GroupID     int64           `json:"group_id"`     // Only present for group messages
	UserID      int64           `json:"user_id"`      // The sender's QQ ID
	RawMessage  string          `json:"raw_message"`  // The raw message text
	Message     json.RawMessage `json:"message"`      // The message content, which may include CQ codes
	Sender      struct {        // The sender's information
		UserID   int64  `json:"user_id"`  // The sender's QQ ID
		Nickname string `json:"nickname"` // The sender's nickname
	} `json:"sender"`
	SelfID  int64  `json:"self_id"`  // The bot's QQ ID
	SubType string `json:"sub_type"` // The subtype of the message, e.g., "normal", "anonymous", etc.
}

// QQController handles QQ Bot interactions by connecting directly to NapCat.
type QQController struct {
	cfg          config.QQConfig
	napcatClient *Client
}

// NewQQController creates a new QQ (Napcat) controller.
func NewQQController(cfg config.QQConfig) *QQController {
	q := &QQController{cfg: cfg}
	if cfg.Enabled {
		q.napcatClient = NewClient(cfg.NapcatAddr, cfg.NapcatPort, cfg.NapcatToken, cfg.NetworkUseProxy)
	}
	return q
}

// Name returns the controller name.
func (q *QQController) Name() string {
	return "qq(napcat)"
}

// Start initializes the QQ controller and starts the NapCat WebSocket listener.
func (q *QQController) Start() error {
	if !q.cfg.Enabled {
		// postLog.Info("QQ (Napcat) controller is disabled")
		return nil
	}
	if q.napcatClient == nil {
		postLog.Warning("QQ (Napcat) controller enabled but NapCat client is nil")
		return nil
	}

	postLog.Info("QQ (Napcat) controller started, connecting to NapCat WebSocket...")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Napcat WS listener panic recovered: %v", r))
			}
		}()
		q.napcatClient.Listen(q.handleNapcatEvent)
	}()

	return nil
}

// Stop shuts down the QQ controller and its WebSocket listener.
func (q *QQController) Stop() {
	if q.napcatClient != nil {
		q.napcatClient.Stop()
	}
	postLog.Info("QQ (Napcat) controller stopped")
}

// IsEnabled returns whether the controller is enabled.
func (q *QQController) IsEnabled() bool {
	return q.cfg.Enabled
}

// handleNapcatEvent processes a raw OneBot event received from the NapCat WebSocket.
func (q *QQController) handleNapcatEvent(raw []byte) {
	var ev oneBotEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		postLog.Debug("Failed to parse NapCat WS event: " + err.Error())
		return
	}

	if config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.ShowNapcatMsg {
		postLog.Debug("Napcat WS event received: " + string(raw))
	}

	// Only handle message events; ignore notice/request/meta_event.
	if ev.PostType != "message" {
		if config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.ShowNapcatAction {
			postLog.Debug("Ignoring Napcat WS event: " + string(raw))
		}
		return
	}

	// Ignore messages the bot itself sent (echo prevention).
	if q.isSelfMessage(ev) && !config.C_globalConfig.System.DebugMode {
		return
	}
	if q.isSelfMessage(ev) && config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.NapcatIgnoreSelfMsg {
		if config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.ShowNapcatAction {
			postLog.Debug("Ignoring Napcat WS self message: " + string(raw))
		}
		return
	}

	// Determine chat information.
	chatID := ev.GroupID
	if ev.MessageType == "private" {
		chatID = ev.UserID
	}

	cmd := controller.Command{
		RawText:  ev.RawMessage,
		ChatID:   chatID,
		ChatType: ev.MessageType,
		SenderID: ev.UserID,
	}

	response := q.processCommand(cmd)
	if response == "" {
		if config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.ShowNapcatAction {
			postLog.Debug("Napcat WS command discarded: " + string(raw))
		}
		return
	}

	// Reply in the same chat.
	if ev.MessageType == "private" {
		if _, err := q.napcatClient.SendMsg("private", ev.UserID, response, false, 0); err != nil {
			postLog.Warning("Failed to send NapCat private reply: " + err.Error())
		}
	} else {
		if _, err := q.napcatClient.SendMsg("group", ev.GroupID, response, false, 0); err != nil {
			postLog.Warning("Failed to send NapCat group reply: " + err.Error())
		}
	}
}

// processCommand validates an incoming message as a command and hands the
// complete command to the unified processor. It returns the response text to
// reply with; an empty response means the message was discarded.
func (q *QQController) processCommand(cmd controller.Command) string {
	text := cmd.RawText

	// In "at" listen mode, require an @mention of the bot and strip it before
	// parsing, otherwise raw_message like "[CQ:at,qq=123] /list" fails the
	// "/" prefix check.
	if q.cfg.ListenMethod == "at" {
		atMention := fmt.Sprintf("[CQ:at,qq=%d]", q.cfg.BotQQID)
		if !strings.Contains(text, atMention) {
			if config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.ShowNapcatAction {
				postLog.Debug("Napcat WS message ignored (no @mention): " + text)
			}
			return "" // Not mentioned, ignore.
		}
		text = strings.ReplaceAll(text, atMention, "")
	}

	// First check whether the received message is a command.
	parsed, ok := controller.ParseCommand(text)
	if !ok {
		return "" // Not a command, discard.
	}

	parsed.ChatID = cmd.ChatID
	parsed.ChatType = cmd.ChatType
	parsed.SenderID = cmd.SenderID
	parsed.Source = "qq_napcat"

	// Hand the complete command to the unified processor, which checks group
	// vs private, trusted groups, admin permissions, and executes it.
	response, err := controller.GetManager().Trigger(parsed, q.trustedGroupIDs(), q.adminIDs(), q.cfg.ListenMethod)
	if config.C_globalConfig.System.DebugMode && config.C_globalConfig.Debug.ShowTriggerCmdEcho {
		postLog.Debug(fmt.Sprintf("[qq_napcat] triggered command: \"/%s\" with args: \"%s\" from chatID: %d and senderID: %d", parsed.Command, strings.Join(parsed.Args, ", "), cmd.ChatID, cmd.SenderID))
	}
	if err != nil {
		postLog.Error("Napcat command processing failed: " + err.Error())
		return ""
	}
	return response
}

// isSelfMessage returns true when the event was generated by the bot itself.
func (q *QQController) isSelfMessage(ev oneBotEvent) bool {
	if q.cfg.BotQQID > 0 && ev.SelfID == q.cfg.BotQQID {
		return true
	}
	if ev.UserID == ev.SelfID {
		return true
	}
	return false
}

// adminIDs returns the QQ admin IDs from bot_user_config.json.
func (q *QQController) adminIDs() []string {
	if c := config.C_botUserConfig; c != nil {
		return c.QQ.Admins.IDs()
	}
	return nil
}

// trustedGroupIDs returns the QQ trusted group IDs from bot_user_config.json.
func (q *QQController) trustedGroupIDs() []string {
	if c := config.C_botUserConfig; c != nil {
		return c.QQ.TrustedGroups.IDs()
	}
	return nil
}

// SendMessage sends an arbitrary message (e.g. the bot initialization message)
// to all QQ trusted groups and admins.
func (q *QQController) SendMessage(message string) error {
	if !q.cfg.Enabled {
		return nil
	}

	for _, groupID := range q.trustedGroupIDs() {
		q.sendGroupMessage(groupID, message)
	}
	for _, adminID := range q.adminIDs() {
		q.sendPrivateMessage(adminID, message)
	}
	return nil
}

// SendStatusChange sends a status change notification via QQ.
func (q *QQController) SendStatusChange(change node.StatusChange) error {
	if !q.cfg.Enabled {
		return nil
	}

	cfg := config.C_globalConfig
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	// Send to trusted groups.
	for _, groupID := range q.trustedGroupIDs() {
		q.sendGroupMessage(groupID, message)
	}

	// Send to admins via private message.
	for _, adminID := range q.adminIDs() {
		q.sendPrivateMessage(adminID, message)
	}

	return nil
}

// SendServerList sends the server list via QQ.
func (q *QQController) SendServerList(onlineServers, offlineServers string) error {
	if !q.cfg.Enabled {
		return nil
	}

	cfg := config.C_globalConfig
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params)

	for _, groupID := range q.trustedGroupIDs() {
		q.sendGroupMessage(groupID, message)
	}
	return nil
}

// SendExecuteResult sends a command execution result via QQ.
func (q *QQController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !q.cfg.Enabled {
		return nil
	}

	cfg := config.C_globalConfig
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params)

	for _, groupID := range q.trustedGroupIDs() {
		q.sendGroupMessage(groupID, message)
	}
	return nil
}

func (q *QQController) sendGroupMessage(groupID string, message string) {
	if q.napcatClient == nil {
		postLog.Warning("Cannot send QQ group message: NapCat client not initialized")
		return
	}

	var groupIDInt int64
	if _, err := fmt.Sscanf(groupID, "%d", &groupIDInt); err != nil || groupIDInt == 0 {
		postLog.Warning(fmt.Sprintf("Invalid QQ group ID: %s", groupID))
		return
	}

	if _, err := q.napcatClient.SendMsg("group", groupIDInt, message, false, 0); err != nil {
		postLog.Warning(fmt.Sprintf("Failed to send QQ group message to %s: %v", groupID, err))
	}
}

func (q *QQController) sendPrivateMessage(userID string, message string) {
	if q.napcatClient == nil {
		postLog.Warning("Cannot send QQ private message: NapCat client not initialized")
		return
	}

	var userIDInt int64
	if _, err := fmt.Sscanf(userID, "%d", &userIDInt); err != nil || userIDInt == 0 {
		postLog.Warning(fmt.Sprintf("Invalid QQ user ID: %s", userID))
		return
	}

	if _, err := q.napcatClient.SendMsg("private", userIDInt, message, false, 0); err != nil {
		postLog.Warning(fmt.Sprintf("Failed to send QQ private message to %s: %v", userID, err))
	}
}
