package controller

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// Command represents a parsed bot command.
type Command struct {
	Source   string   // Name of the pipe the command arrived on (see Controller.Name)
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
	// MessageTypeAlert marks an alert submitted by an external application
	// through the incoming webhook API. Not member-controllable: an alert is
	// always delivered to the channel's recipients.
	MessageTypeAlert = "alert"
)

// Alert is a free-form notification submitted by an external application
// through the incoming webhook API. Its target channels are chosen per webhook
// endpoint in config.json, not per alert.
type Alert struct {
	Subject string // Short one-line title of the alert
	Source  string // Name of the webhook endpoint the alert was submitted to
	Content string // Free-form alert body
	Time    string // Submission time
}

// Render renders the alert body for a channel, wrapping the source and content
// in Markdown when that channel has markdown enabled (see template.RenderAlert).
func (a Alert) Render(markdown bool) string {
	return template.RenderAlert(template.AlertParams{
		Subject: a.Subject,
		Source:  a.Source,
		Content: a.Content,
		Time:    a.Time,
	}, markdown)
}

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
	// IsMarkdown reports whether the channel renders Markdown, per its own
	// "markdown" setting in config.json.
	IsMarkdown() bool
	SendStatusChange(change node.StatusChange) error
	SendServerList(onlineServers, offlineServers string) error
	SendExecuteResult(serverName, serverUUID, command, result string) error
	// SendAlert delivers a free-form alert submitted through the incoming
	// webhook API to the channel's own recipients.
	SendAlert(alert Alert) error
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
// EventBotStarted opt-out. It is rendered once per controller because the
// Markdown formatting depends on each channel's own markdown setting.
func (m *Manager) ShowBotInitMessage() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := config.C_globalConfig
	params := template.BuildBotInitializationMsgParams()

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
			Content: template.Render(cfg.ControllerMessage.BotStarted, params, ctrl.IsMarkdown()),
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
// receive it, and rendered once per controller so each channel's markdown
// setting is honored.
func (m *Manager) ShowBotServerList() {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := config.C_globalConfig
	params := template.BuildParamsFromServerList()

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
			Content: template.Render(cfg.ControllerMessage.ServerList, params, ctrl.IsMarkdown()),
			Type:    MessageTypeBotStarted,
		}
		if err := bot.SendMessage(message); err != nil {
			postLog.Warning(fmt.Sprintf("Controller %s failed to send server list: %v", bot.Name(), err))
		}
	}
}

// NotifyStatusChange sends a status change notification to all enabled
// controllers. It honors the per-node allow-list in bot_node_config.json: a
// node whose enableStatusNotify is not true is skipped entirely, so no
// controller (chat bots or notification pipes) broadcasts its change.
func (m *Manager) NotifyStatusChange(change node.StatusChange) {
	if !config.NodeStatusNotifyEnabled(change.UUID) {
		postLog.Debug(fmt.Sprintf("Status change for node %s skipped: enableStatusNotify is not enabled", change.UUID))
		return
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, ctrl := range m.controllers {
		if !ctrl.IsEnabled() {
			continue
		}
		if err := ctrl.SendStatusChange(change); err != nil {
			postLog.Warning(fmt.Sprintf("Controller %s failed to send status change: %v", ctrl.Name(), err))
		}
	}
}

// NotifyAlert delivers an alert to the named pipes only, and returns the names
// of the pipes it was handed to. A pipe that is unknown, disabled or fails to
// send is reported through the returned error instead of stopping the delivery
// to the remaining pipes; if no pipe accepted the alert, the error describes
// every failure.
func (m *Manager) NotifyAlert(pipes []string, alert Alert) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var delivered, failures []string
	for _, name := range pipes {
		ctrl, ok := m.controllers[name]
		if !ok {
			failures = append(failures, fmt.Sprintf("%s: no such channel", name))
			continue
		}
		if !ctrl.IsEnabled() {
			failures = append(failures, fmt.Sprintf("%s: channel is disabled", name))
			continue
		}
		if err := ctrl.SendAlert(alert); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", name, err))
			continue
		}
		delivered = append(delivered, name)
	}

	if len(failures) > 0 {
		postLog.Warning(fmt.Sprintf("Alert %q from %s not delivered by: %s", alert.Subject, alert.Source, strings.Join(failures, "; ")))
	}
	if len(delivered) == 0 {
		if len(failures) == 0 {
			return nil, errors.New("no notify channel configured")
		}
		return nil, errors.New(strings.Join(failures, "; "))
	}
	return delivered, nil
}

// IsMarkdown reports whether the pipe with the given name renders Markdown, per
// its channel's "markdown" setting in config.json. An unknown pipe renders
// plain text.
func (m *Manager) IsMarkdown(pipeName string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ctrl, ok := m.controllers[pipeName]
	if !ok {
		return false
	}
	return ctrl.IsMarkdown()
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
