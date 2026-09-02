package pipes

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// NtfyController handles notifications via ntfy.sh or a self-hosted ntfy server.
type NtfyController struct {
	cfg        config.NtfyConfig
	httpClient *http.Client
}

// NewNtfyController creates a new Ntfy controller.
func NewNtfyController(cfg config.NtfyConfig) *NtfyController {
	return &NtfyController{
		cfg:        cfg,
		httpClient: netproxy.HTTPClient(cfg.NetworkUseProxy, 10*time.Second),
	}
}

// Name returns the controller name.
func (n *NtfyController) Name() string {
	return "ntfy"
}

// Start initializes the Ntfy controller.
func (n *NtfyController) Start() error {
	if !n.cfg.Enabled {
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

// IsEnabled returns whether the controller is enabled.
func (n *NtfyController) IsEnabled() bool {
	return n.cfg.Enabled
}

// SendStatusChange sends a status change notification via Ntfy.
func (n *NtfyController) SendStatusChange(change node.StatusChange) error {
	if !n.cfg.Enabled {
		return nil
	}

	cfg := config.C_globalConfig
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	title := fmt.Sprintf("Server %s: %s", change.Name, change.Event)
	return n.publish(title, message)
}

// SendServerList sends the server list via Ntfy.
func (n *NtfyController) SendServerList(onlineServers, offlineServers string) error {
	if !n.cfg.Enabled {
		return nil
	}

	cfg := config.C_globalConfig
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params)

	return n.publish("Server List", message)
}

// SendExecuteResult sends a command execution result via Ntfy.
func (n *NtfyController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !n.cfg.Enabled {
		return nil
	}

	cfg := config.C_globalConfig
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params)

	title := fmt.Sprintf("Command Result: %s on %s", command, serverName)
	return n.publish(title, message)
}

func (n *NtfyController) publish(title, message string) error {
	serverURL := n.cfg.Server
	if serverURL == "" {
		serverURL = "https://ntfy.sh"
	}
	serverURL = strings.TrimRight(serverURL, "/")

	publishURL := fmt.Sprintf("%s/%s", serverURL, n.cfg.Topic)

	req, err := http.NewRequest("POST", publishURL, strings.NewReader(message))
	if err != nil {
		return fmt.Errorf("failed to create ntfy request: %w", err)
	}

	req.Header.Set("Title", title)
	if n.cfg.Priority != "" && n.cfg.Priority != "default" {
		req.Header.Set("Priority", n.cfg.Priority)
	}
	if n.cfg.Token != "" {
		req.Header.Set("Authorization", "Bearer "+n.cfg.Token)
	}

	resp, err := n.httpClient.Do(req)
	if err != nil {
		postLog.Warning("Failed to publish to ntfy: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		postLog.Warning(fmt.Sprintf("Ntfy publish returned status %d", resp.StatusCode))
	}

	postLog.Debug("Ntfy notification sent to topic: " + n.cfg.Topic)
	return nil
}
