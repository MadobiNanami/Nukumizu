package pipes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"sync/atomic"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// WebhookController handles notifications via generic HTTP webhooks.
//
// Both fields are held behind atomic pointers rather than in plain fields:
// Reload replaces them on the settings-update goroutine while the send methods
// read them on the status-change and incoming-webhook goroutines.
type WebhookController struct {
	cfg        atomic.Pointer[config.WebhookConfig]
	httpClient atomic.Pointer[http.Client]
}

// NewWebhookController creates a new Webhook controller.
func NewWebhookController(cfg config.WebhookConfig) *WebhookController {
	w := &WebhookController{}
	w.cfg.Store(&cfg)
	w.httpClient.Store(netproxy.HTTPClient(cfg.NetworkUseProxy, 10*time.Second))
	return w
}

// settings returns the configuration currently in effect. The value it points
// at is never mutated after being stored, so a caller can hold the pointer for
// one whole send without a concurrent Reload disturbing it.
func (w *WebhookController) settings() *config.WebhookConfig {
	return w.cfg.Load()
}

// client returns the HTTP client built for the current proxy setting.
func (w *WebhookController) client() *http.Client {
	return w.httpClient.Load()
}

// Name returns the controller name.
func (w *WebhookController) Name() string {
	return "webhook"
}

// Start initializes the Webhook controller.
func (w *WebhookController) Start() error {
	if !w.settings().Enabled {
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

// Reload applies the current configuration. Everything the sender reads is
// taken from the settings at send time, so only a change to the proxy flag
// needs more than the swap: that one is baked into the HTTP client's transport
// when the client is built.
func (w *WebhookController) Reload() {
	global := config.Current()
	if global == nil {
		return
	}

	current := w.settings()
	updated := global.ControllerMethod.Webhook
	// WebhookConfig carries a header map, so it is not comparable with ==.
	if reflect.DeepEqual(*current, updated) {
		return
	}

	if current.NetworkUseProxy != updated.NetworkUseProxy {
		w.httpClient.Store(netproxy.HTTPClient(updated.NetworkUseProxy, 10*time.Second))
	}
	w.cfg.Store(&updated)
	postLog.Debug("Webhook controller reloaded")
}

// IsEnabled returns whether the controller is enabled.
func (w *WebhookController) IsEnabled() bool {
	return w.settings().Enabled
}

// IsMarkdown returns whether the channel renders Markdown, per its markdown
// setting in config.json.
func (w *WebhookController) IsMarkdown() bool {
	return w.settings().Markdown
}

// SendStatusChange sends a status change notification via Webhook.
func (w *WebhookController) SendStatusChange(change node.StatusChange) error {
	s := w.settings()
	if !s.Enabled {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromStatusChange(change)
	message := template.Render(cfg.ControllerMessage.ServerStatusChanged, params, s.Markdown)

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
	s := w.settings()
	if !s.Enabled {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromServerList()
	message := template.Render(cfg.ControllerMessage.ServerList, params, s.Markdown)

	payload := map[string]interface{}{
		"type":           "serverList",
		"onlineServers":  params.OnlineServers,
		"offlineServers": params.OfflineServers,
		"message":        message,
		"time":           params.Time,
	}

	return w.send(payload)
}

// SendExecuteResult sends a command execution result via Webhook.
func (w *WebhookController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	s := w.settings()
	if !s.Enabled {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	message := template.Render(cfg.ControllerMessage.ServerExecuteResult, params, s.Markdown)

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

// SendAlert sends an alert submitted through the incoming webhook API to the
// configured URL.
func (w *WebhookController) SendAlert(alert controller.Alert) error {
	s := w.settings()
	if !s.Enabled {
		return nil
	}

	payload := map[string]interface{}{
		"type":    "alert",
		"subject": alert.Subject,
		"source":  alert.Source,
		"content": alert.Content,
		"message": alert.Render(s.Markdown),
		"time":    alert.Time,
	}

	return w.send(payload)
}

// send posts one payload using the settings in effect at the moment it is
// called, so the URL, method and headers all come from the same configuration
// even if a reload lands mid-send.
func (w *WebhookController) send(payload map[string]interface{}) error {
	s := w.settings()

	method := s.Method
	if method == "" {
		method = "POST"
	}

	bodyJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook payload: %w", err)
	}

	req, err := http.NewRequest(method, s.URL, bytes.NewReader(bodyJSON))
	if err != nil {
		return fmt.Errorf("failed to create webhook request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	for key, value := range s.Headers {
		req.Header.Set(key, value)
	}

	resp, err := w.client().Do(req)
	if err != nil {
		postLog.Warning("Failed to send webhook: " + err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		postLog.Warning(fmt.Sprintf("Webhook returned status %d", resp.StatusCode))
	}

	postLog.Debug("Webhook notification sent to " + s.URL)
	return nil
}
