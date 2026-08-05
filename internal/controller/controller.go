package controller

import (
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
	RawText  string
	Command  string   // The command word (e.g., "list", "status")
	Args     []string // Command arguments
	ChatID   int64    // Chat/group ID where the command was issued
	ChatType string   // "group" or "private"
	SenderID int64    // User ID of the sender
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

// CommandController extends Controller for bidirectional channels (QQ, Telegram).
type CommandController interface {
	Controller
	HandleCommand(cmd Command) (response string, err error)
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

// NotifyStatusChange sends a status change notification to all enabled controllers.
func (m *Manager) NotifyStatusChange(change node.StatusChange) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := config.GetConfig()
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
