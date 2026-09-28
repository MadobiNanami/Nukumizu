package pipes

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// NtfyController handles notifications via ntfy.sh or a self-hosted ntfy server.
//
// Both fields are held behind atomic pointers rather than in plain fields:
// Reload replaces them on the settings-update goroutine while the send methods
// read them on the status-change and incoming-webhook goroutines.
type NtfyController struct {
	cfg        atomic.Pointer[config.NtfyConfig]
	httpClient atomic.Pointer[http.Client]
}

// NewNtfyController creates a new Ntfy controller.
func NewNtfyController(cfg config.NtfyConfig) *NtfyController {
	n := &NtfyController{}
	n.cfg.Store(&cfg)
	n.httpClient.Store(netproxy.HTTPClient(cfg.NetworkUseProxy, 10*time.Second))
	return n
}

// settings returns the configuration currently in effect. The value it points
// at is never mutated after being stored, so a caller can hold the pointer for
// one whole send without a concurrent Reload disturbing it.
func (n *NtfyController) settings() *config.NtfyConfig {
	return n.cfg.Load()
}

// client returns the HTTP client built for the current proxy setting.
func (n *NtfyController) client() *http.Client {
	return n.httpClient.Load()
}

// Name returns the controller name.
func (n *NtfyController) Name() string {
	return "ntfy"
}

// Start initializes the Ntfy controller.
func (n *NtfyController) Start() error {
	if !n.settings().Enabled {
		postLog.Info("Ntfy controller is disabled")
		return nil
	}
	postLog.Info("Ntfy controller started")
	return nil
}

// Stop shuts down the Ntfy controller.
func (n *NtfyController) Stop() {
	postLog.Info("Ntfy controller stopped")
}

// Reload applies the current configuration. Everything the publisher reads is
// taken from the settings at send time, so only a change to the proxy flag
// needs more than the swap: that one is baked into the HTTP client's transport
// when the client is built.
func (n *NtfyController) Reload() {
	global := config.Current()
	if global == nil {
		return
	}

	current := n.settings()
	updated := global.ControllerMethod.Ntfy
	if *current == updated {
		return
	}

	if current.NetworkUseProxy != updated.NetworkUseProxy {
		n.httpClient.Store(netproxy.HTTPClient(updated.NetworkUseProxy, 10*time.Second))
	}
	n.cfg.Store(&updated)
	postLog.Debug("Ntfy controller reloaded")
}

// IsEnabled returns whether the controller is enabled.
func (n *NtfyController) IsEnabled() bool {
	return n.settings().Enabled
}

// IsMarkdown returns whether the channel renders Markdown, per its markdown
// setting in config.json.
func (n *NtfyController) IsMarkdown() bool {
	return n.settings().Markdown
}

// SendStatusChange sends a status change notification via Ntfy.
func (n *NtfyController) SendStatusChange(change node.StatusChange) error {
	s := n.settings()
	if !s.Enabled {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params, s.Markdown)

	title := fmt.Sprintf("Server %s: %s", change.Name, change.Event)
	return n.publish(title, message)
}

// SendServerList sends the server list via Ntfy.
func (n *NtfyController) SendServerList(onlineServers, offlineServers string) error {
	s := n.settings()
	if !s.Enabled {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params, s.Markdown)

	return n.publish("Server List", message)
}

// SendExecuteResult sends a command execution result via Ntfy.
func (n *NtfyController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	s := n.settings()
	if !s.Enabled {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params, s.Markdown)

	title := fmt.Sprintf("Command Result: %s on %s", command, serverName)
	return n.publish(title, message)
}

// SendAlert sends an alert submitted through the incoming webhook API to the
// configured topic.
func (n *NtfyController) SendAlert(alert controller.Alert) error {
	s := n.settings()
	if !s.Enabled {
		return nil
	}

	return n.publish(alert.Subject, alert.Render(s.Markdown))
}

func (n *NtfyController) publish(title, message string) error {
	s := n.settings()

	serverURL := s.Server
	if serverURL == "" {
		serverURL = "https://ntfy.sh"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	publishURL := fmt.Sprintf("%s/%s", serverURL, s.Topic)

	req, err := http.NewRequest("POST", publishURL, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("failed to create ntfy request: %w", err)
	}

	req.Header.Set("Title", title)
	if s.Priority != "" && s.Priority != "default" {
		req.Header.Set("Priority", s.Priority)
	}
	if s.Token != "" {
		req.Header.Set("Authorization", "Bearer "+s.Token)
	}

	resp, err := n.client().Do(req)
	if err != nil {
		postLog.Warning("Failed to publish to ntfy: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		postLog.Warning(fmt.Sprintf("Ntfy publish returned status %d", resp.StatusCode))
	}

	postLog.Debug("Ntfy notification sent to topic: " + s.Topic)
	return nil
}
