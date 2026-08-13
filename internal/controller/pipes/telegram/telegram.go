package telegram

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// TelegramController handles Telegram Bot interactions via long polling.
type TelegramController struct {
	cfg    config.TelegramConfig
	client *Client
	bot    *User // The bot's own user object from getMe.

	mu           sync.Mutex
	usernameToID map[string]int64 // resolved @username -> numeric user ID
}

// NewTelegramController creates a new Telegram controller.
func NewTelegramController(cfg config.TelegramConfig) *TelegramController {
	t := &TelegramController{
		cfg:          cfg,
		usernameToID: make(map[string]int64),
	}
	if cfg.Enabled {
		t.client = NewClient(cfg.BotToken, cfg.NetworkUseProxy)
	}
	return t
}

// Name returns the controller name.
func (t *TelegramController) Name() string {
	return "telegram"
}

// Start verifies the bot token via getMe and begins long polling.
func (t *TelegramController) Start() error {
	if !t.cfg.Enabled {
		return nil
	}
	if t.client == nil {
		postLog.Warning("Telegram controller enabled but client is nil")
		return nil
	}
	if t.cfg.BotToken == "" {
		postLog.Warning("Telegram controller enabled but no bot token configured")
		return nil
	}

	// The tutorial's first step: verify the token with getMe. The returned bot
	// identity is used for @mention mode and echo prevention.
	bot, err := t.client.GetMe()
	if err != nil {
		return fmt.Errorf("failed to verify Telegram bot token (getMe): %w", err)
	}
	t.bot = bot
	postLog.Info(fmt.Sprintf("Telegram bot authenticated: @%s (id %d)", bot.Username, bot.ID))

	postLog.Info("Telegram controller started (long polling)")
	go func() {
		defer func() {
			if r := recover(); r != nil {
				postLog.Error(fmt.Sprintf("Telegram poll loop panic recovered: %v", r))
			}
		}()
		t.client.Listen(t.handleUpdate)
	}()

	return nil
}

// Stop shuts down the Telegram controller and its poll loop.
func (t *TelegramController) Stop() {
	if t.client != nil {
		t.client.Stop()
	}
	postLog.Info("Telegram controller stopped")
}

// IsEnabled returns whether the controller is enabled.
func (t *TelegramController) IsEnabled() bool {
	return t.cfg.Enabled
}

// handleUpdate processes a single Telegram update received via long polling.
func (t *TelegramController) handleUpdate(update Update) {
	if update.Message == nil {
		return
	}
	msg := update.Message

	if config.GetConfig().System.DebugMode && config.GetConfig().Debug.ShowTelegramMsg {
		raw, _ := json.Marshal(update)
		postLog.Debug("Telegram update received: " + string(raw))
	}

	// Echo prevention: ignore messages from other bots and from this bot itself.
	if msg.From == nil || msg.From.IsBot {
		return
	}
	if t.bot != nil && msg.From.ID == t.bot.ID {
		return
	}

	chatType := telegramChatType(msg.Chat.Type)
	if chatType == "" {
		return // channel or other unsupported chat type.
	}

	// Record username -> ID so admins configured by username can be reached.
	t.recordUser(msg.From)

	// Only text messages can carry commands.
	if strings.TrimSpace(msg.Text) == "" {
		return
	}

	cmd := controller.Command{
		RawText:  msg.Text,
		ChatID:   msg.Chat.ID,
		ChatType: chatType,
		SenderID: msg.From.ID,
	}

	response := t.processCommand(cmd)
	if response == "" {
		return
	}

	if err := t.client.SendMessage(msg.Chat.ID, response); err != nil {
		postLog.Warning("Failed to send Telegram reply: " + err.Error())
	}
}

// processCommand validates an incoming message as a command and hands the
// complete command to the unified processor. It returns the response text to
// reply with; an empty response means the message was discarded.
func (t *TelegramController) processCommand(cmd controller.Command) string {
	text := cmd.RawText

	// In "at" listen mode, require a mention of the bot and strip it before
	// parsing, mirroring the QQ CQ-at behavior.
	if t.cfg.ListenMethod == "at" && t.bot != nil && t.bot.Username != "" {
		mention := "@" + t.bot.Username
		if !strings.Contains(text, mention) {
			return "" // Not mentioned, ignore.
		}
		text = strings.ReplaceAll(text, mention, "")
	}

	// Telegram appends @botname to commands sent in groups (/list@MyBot); strip
	// it so the command word parses.
	text = stripCommandBotSuffix(text)

	// First check whether the received message is a command.
	parsed, ok := controller.ParseCommand(text)
	if !ok {
		return "" // Not a command, discard.
	}

	parsed.ChatID = cmd.ChatID
	parsed.ChatType = cmd.ChatType
	parsed.SenderID = cmd.SenderID
	parsed.Source = "telegram"

	// Hand the complete command to the unified processor, which checks group
	// vs private, trusted groups, admin permissions, and executes it.
	response, err := controller.GetManager().Trigger(parsed, t.cfg.TrustedGroups, t.resolvedAdminList(), t.cfg.ListenMethod)
	if config.GetConfig().System.DebugMode && config.GetConfig().Debug.ShowTriggerCmdEcho {
		postLog.Debug(fmt.Sprintf("[telegram] triggered command: \"/%s\" with args: \"%s\" from chatID: %d and senderID: %d", parsed.Command, strings.Join(parsed.Args, ", "), cmd.ChatID, cmd.SenderID))
	}
	if err != nil {
		postLog.Error("Telegram command processing failed: " + err.Error())
		return ""
	}
	return response
}

// SendMessage sends an arbitrary message (e.g. the bot initialization message)
// to all Telegram trusted groups and admins.
func (t *TelegramController) SendMessage(message string) error {
	if !t.cfg.Enabled || t.client == nil {
		return nil
	}
	t.sendToGroupsAndAdmins(message)
	return nil
}

// SendStatusChange sends a status change notification via Telegram.
func (t *TelegramController) SendStatusChange(change node.StatusChange) error {
	if !t.cfg.Enabled || t.client == nil {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	t.sendToGroupsAndAdmins(message)
	return nil
}

// SendServerList sends the server list via Telegram.
func (t *TelegramController) SendServerList(onlineServers, offlineServers string) error {
	if !t.cfg.Enabled || t.client == nil {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params)

	t.sendToGroups(message)
	return nil
}

// SendExecuteResult sends a command execution result via Telegram.
func (t *TelegramController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !t.cfg.Enabled || t.client == nil {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params)

	t.sendToGroups(message)
	return nil
}

// telegramChatType maps a Telegram chat type to the unified ChatType value used
// by the controller package. Empty means the chat type is unsupported.
func telegramChatType(chatType string) string {
	switch chatType {
	case "private":
		return "private"
	case "group", "supergroup":
		return "group"
	default:
		return ""
	}
}

// stripCommandBotSuffix removes a "@botname" suffix from the command word,
// turning "/list@MyBot" into "/list".
func stripCommandBotSuffix(text string) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}

	// The command word is everything before the first space.
	spaceIdx := strings.Index(text, " ")
	if spaceIdx < 0 {
		spaceIdx = len(text)
	}

	command, _, found := strings.Cut(text[1:spaceIdx], "@")
	if !found {
		return text
	}
	return "/" + command + text[spaceIdx:]
}

// recordUser stores the numeric ID for a sender's username so admins configured
// by username (rather than numeric ID) can be resolved later.
func (t *TelegramController) recordUser(u *User) {
	if u == nil || u.Username == "" {
		return
	}
	t.mu.Lock()
	t.usernameToID[strings.ToLower(u.Username)] = u.ID
	t.mu.Unlock()
}

// resolveUsername resolves a Telegram username to its numeric ID, or returns
// ok == false when the user has not messaged the bot yet.
func (t *TelegramController) resolveUsername(username string) (int64, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	id, ok := t.usernameToID[strings.ToLower(strings.TrimPrefix(username, "@"))]
	return id, ok
}

// resolvedAdminList maps username-based admin entries to numeric IDs so the
// unified processor's numeric IsAdmin check works. Unresolvable entries (the
// user has not messaged the bot yet) pass through unchanged and simply never
// match.
func (t *TelegramController) resolvedAdminList() []string {
	result := make([]string, 0, len(t.cfg.Admins))
	for _, admin := range t.cfg.Admins {
		if id, ok := t.resolveUsername(admin); ok {
			result = append(result, strconv.FormatInt(id, 10))
		} else {
			result = append(result, admin)
		}
	}
	return result
}

// sendToGroupsAndAdmins sends a message to all trusted groups and admins.
func (t *TelegramController) sendToGroupsAndAdmins(message string) {
	t.sendToGroups(message)
	for _, admin := range t.cfg.Admins {
		t.sendAdminMessage(admin, message)
	}
}

// sendToGroups sends a message to all trusted groups.
func (t *TelegramController) sendToGroups(message string) {
	for _, groupID := range t.cfg.TrustedGroups {
		t.sendGroupMessage(groupID, message)
	}
}

// sendGroupMessage sends a message to a single group chat.
func (t *TelegramController) sendGroupMessage(groupID string, message string) {
	chatID, err := strconv.ParseInt(groupID, 10, 64)
	if err != nil || chatID == 0 {
		postLog.Warning(fmt.Sprintf("Invalid Telegram group chat ID: %s", groupID))
		return
	}
	t.sendToChat(chatID, message)
}

// sendAdminMessage sends a message to an admin, resolving a username-based
// entry to a numeric ID. Telegram bots cannot start conversations, so an admin
// configured by username is only reachable after they have messaged the bot.
func (t *TelegramController) sendAdminMessage(admin string, message string) {
	chatID, err := strconv.ParseInt(admin, 10, 64)
	if err != nil || chatID == 0 {
		// Not a numeric ID: try to resolve a username.
		if id, ok := t.resolveUsername(admin); ok {
			t.sendToChat(id, message)
			return
		}
		postLog.Warning(fmt.Sprintf("Cannot reach Telegram admin %q: user has not messaged the bot yet (bots cannot start conversations)", admin))
		return
	}
	t.sendToChat(chatID, message)
}

// sendToChat sends a message to a chat ID, logging failures.
func (t *TelegramController) sendToChat(chatID int64, message string) {
	if err := t.client.SendMessage(chatID, message); err != nil {
		postLog.Warning(fmt.Sprintf("Failed to send Telegram message to %d: %v", chatID, err))
	}
}
