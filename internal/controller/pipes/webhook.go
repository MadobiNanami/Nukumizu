package pipes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// WebhookController handles notifications via generic HTTP webhooks.
type WebhookController struct {
	cfg        config.WebhookConfig
	httpClient *http.Client
}

// NewWebhookController creates a new Webhook controller.
func NewWebhookController(cfg config.WebhookConfig) *WebhookController {
	return &WebhookController{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Name returns the controller name.
func (w *WebhookController) Name() string {
	return "webhook"
}

// Start initializes the Webhook controller.
func (w *WebhookController) Start() error {
	if !w.cfg.Enabled {
		postLog.Info("Webhook controller is disabled")
		return nil
	}
	postLog.Info("Webhook controller started")
	return nil
}

// Stop shuts down the Webhook controller.
func (w *WebhookController) Stop() {
	postLog.Info("Webhook controller stopped")
}

// IsEnabled returns whether the controller is enabled.
func (w *WebhookController) IsEnabled() bool {
	return w.cfg.Enabled
}

// SendStatusChange sends a status change notification via Webhook.
func (w *WebhookController) SendStatusChange(change node.StatusChange) error {
	if !w.cfg.Enabled {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	payload := map[string]interface{}{
		"event":      change.Event,
		"serverName": change.Name,
		"serverUUID": change.UUID,
		"message":    message,
		"time":       params.Time,
	}

	return w.send(payload)
}

// SendServerList sends the server list via Webhook.
func (w *WebhookController) SendServerList(onlineServers, offlineServers string) error {
	if !w.cfg.Enabled {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params)

	payload := map[string]interface{}{
		"type":            "serverList",
		"onlineServers":   params.OnlineServers,
		"offlineServers":  params.OfflineServers,
		"message":         message,
		"time":            params.Time,
	}

	return w.send(payload)
}

// SendExecuteResult sends a command execution result via Webhook.
func (w *WebhookController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !w.cfg.Enabled {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params)

	payload := map[string]interface{}{
		"type":       "executeResult",
		"serverName": serverName,
		"serverUUID": serverUUID,
		"command":    command,
		"result":     params.Result,
		"message":    message,
		"time":       params.Time,
	}

	return w.send(payload)
}

func (w *WebhookController) send(payload map[string]interface{}) error {
	method := w.cfg.Method
	if method == "" {
		method = "POST"
	}

	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(method, w.cfg.URL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range w.cfg.Headers {
		req.Header.Set(key, value)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		postLog.Warning("Failed to send webhook: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		postLog.Warning(fmt.Sprintf("Webhook returned status %d", resp.StatusCode))
	}

	postLog.Debug("Webhook notification sent to " + w.cfg.URL)
	return nil
}
