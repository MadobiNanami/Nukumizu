package controller

import (
	"fmt"
	"sync"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// Command represents a parsed bot command.
type Command struct {
	Source   string   // The source pipe (e.g., "telegram", "qq", "napcat")
	RawText  string   // The raw text of the command message
	Command  string   // The command word (e.g., "list", "status")
	Args     []string // Command arguments
	ChatID   int64    // Chat/group ID where the command was issued
	ChatType string   // "group" or "private"
	SenderID int64    // User ID of the sender
}

// Message represents a message to be sent by a controller.
type Message struct {
	Source  string // The source pipe (e.g., "telegram", "qq", "napcat")
	Content string // The message content
	ChatID  int64  // Chat/group ID where the message should be sent
	Type    string // Message type (see MessageType*), used for per-member opt-outs
}

// Message type labels. They let bot pipes apply per-member opt-out options from
// bot_user_config.json (see MemberReceives) to automatic messages.
const (
	// MessageTypeBotStarted marks the automatic welcome/server-list messages the
	// bot pushes on startup. Gated by BotUserOptions.EventBotStarted.
	MessageTypeBotStarted = "event_bot_started"
	// MessageTypeReply marks a direct reply to a user command. Reserved for the
	// BotUserOptions.EventReply opt-out.
	MessageTypeReply = "event_reply"
)

// MemberReceives reports whether a member whose bot_user_config.json options are
// opts receives an automatic message of the given type. Only member-controllable
// types are gated; anything else is always delivered.
func MemberReceives(opts config.BotUserOptions, messageType string) bool {
	switch messageType {
	case MessageTypeBotStarted:
		return opts.EventBotStarted
	default:
		return true
	}
}

// Controller defines the interface for all notification/bot controllers.
type Controller interface {
	Name() string
	Start() error
	Stop()
	IsEnabled() bool
	SendStatusChange(change node.StatusChange) error
	SendServerList(onlineServers, offlineServers string) error
	SendExecuteResult(serverName, serverUUID, command, result string) error
}

// BotController is implemented by controllers that act as chat bots and can
// deliver arbitrary messages, such as the bot initialization message on startup.
// Only bot-type pipes (QQ/NapCat, Telegram) implement it; notification-only
// pipes (email, ntfy, webhook) do not.
type BotController interface {
	Controller
	SendMessage(message Message) error
}

// Manager manages all controller instances and routes events.
type Manager struct {
	mu          sync.RWMutex
	controllers map[string]Controller
}

var globalManager *Manager

// InitManager initializes the global controller manager.
func InitManager() {
	globalManager = &Manager{
		controllers: make(map[string]Controller),
	}
	postLog.Info("Controller manager initialized")
}

// GetManager returns the global controller manager.
func GetManager() *Manager {
	return globalManager
}

// Register adds a controller to the manager.
func (m *Manager) Register(c Controller) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.controllers[c.Name()] = c
	postLog.Info("Controller registered: " + c.Name())
}

// ShowBotInitMessage sends the bot initialization message to all enabled
// bot controllers (QQ/NapCat and Telegram). Notification-only pipes that do
// not implement BotController are skipped. The message is typed
// MessageTypeBotStarted so each controller can honor its members' per-recipient
// EventBotStarted opt-out.
func (m *Manager) ShowBotInitMessage() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := config.C_globalConfig
	params := template.BuildBotInitializationMsgParams()
	content := template.Render(cfg.ControllerMessage.BotStarted, params)

	for _, ctrl := range m.controllers {
		if !ctrl.IsEnabled() {
			continue
		}
		bot, ok := ctrl.(BotController)
		if !ok {
			continue // Notification-only pipe (email/ntfy/webhook), not a bot.
		}
		message := Message{
			Source:  bot.Name(),
			Content: content,
			Type:    MessageTypeBotStarted,
		}
		if err := bot.SendMessage(message); err != nil {
			postLog.Warning(fmt.Sprintf("Controller %s failed to send init message: %v", bot.Name(), err))
		}
	}
}

// ShowBotServerList sends the startup server list to all enabled bot
// controllers. The message content is identical to the /list command (same
// template and parameters). Like the init message it is typed
// MessageTypeBotStarted so members who opted out of bot-started pushes do not
// receive it.
func (m *Manager) ShowBotServerList() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := config.C_globalConfig
	params := template.BuildParamsFromServerList()
	content := template.Render(cfg.ControllerMessage.ServerList, params)

	for _, ctrl := range m.controllers {
		if !ctrl.IsEnabled() {
			continue
		}
		bot, ok := ctrl.(BotController)
		if !ok {
			continue // Notification-only pipe (email/ntfy/webhook), not a bot.
		}
		message := Message{
			Source:  bot.Name(),
			Content: content,
			Type:    MessageTypeBotStarted,
		}
		if err := bot.SendMessage(message); err != nil {
			postLog.Warning(fmt.Sprintf("Controller %s failed to send server list: %v", bot.Name(), err))
		}
	}
}

// NotifyStatusChange sends a status change notification to all enabled controllers.
func (m *Manager) NotifyStatusChange(change node.StatusChange) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := config.C_globalConfig
	templateStr := cfg.ControllerMessage.ServerStatusChanged
	params := template.BuildParamsFromStatusChange(change)

	for _, ctrl := range m.controllers {
		if !ctrl.IsEnabled() {
			continue
		}

		// Get the names of controllers that support commands for the message.
		if err := ctrl.SendStatusChange(change); err != nil {
			postLog.Warning(fmt.Sprintf("Controller %s failed to send status change: %v", ctrl.Name(), err))
		}
		_ = templateStr
		_ = params
	}
}

// StopAll stops all registered controllers.
func (m *Manager) StopAll() {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ctrl := range m.controllers {
		ctrl.Stop()
	}
}

// NotifyAllAdmins sends an emergency message to all enabled controllers.
func (m *Manager) NotifyAllAdmins(message string) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ctrl := range m.controllers {
		if !ctrl.IsEnabled() {
			continue
		}
		postLog.Info(fmt.Sprintf("Notifying via %s: %s", ctrl.Name(), message))
	}
}
