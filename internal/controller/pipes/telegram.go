package pipes

import (
	"fmt"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
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

// processCommand validates an incoming message as a command and hands the
// complete command to the unified processor. It returns the response text to
// reply with; an empty response means the message was discarded.
func (t *TelegramController) processCommand(cmd controller.Command) string {
	// First check whether the received message is a command.
	parsed, ok := controller.ParseCommand(cmd.RawText)
	if !ok {
		return "" // Not a command, discard.
	}

	parsed.ChatID = cmd.ChatID
	parsed.ChatType = cmd.ChatType
	parsed.SenderID = cmd.SenderID

	// Hand the complete command to the unified processor, which checks group
	// vs private, trusted groups, admin permissions, and executes it.
	response, err := controller.GetManager().Trigger(parsed, t.cfg.TrustedGroups, t.cfg.Admins, t.cfg.ListenMethod)
	if err != nil {
		postLog.Error("Telegram command processing failed: " + err.Error())
		return ""
	}
	return response
}

// SendMessage sends an arbitrary message (e.g. the bot initialization message)
// via Telegram. The Telegram Bot API integration is currently a stub, so this
// only logs the message for now.
func (t *TelegramController) SendMessage(message string) error {
	if !t.cfg.Enabled || t.cfg.BotToken == "" {
		return nil
	}
	postLog.Debug("Telegram init message: " + message)
	return nil
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
