package handler

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/postLog"
	"nukumizu-backend/utils"
)

// maxWebhookBodyBytes caps the size of an incoming webhook request body. The
// endpoint is reachable without a session token, so the body is bounded before
// it is read.
const maxWebhookBodyBytes = 1 << 20 // 1 MiB

// webhookRequest is the JSON body accepted by the incoming webhook API.
type webhookRequest struct {
	Token   string `json:"token"`
	Subject string `json:"subject"`
	Content string `json:"content"`
}

// WebhookHandler handles POST /api/webhook/{name}, the incoming webhook API
// served on its own listener (see webhook in config.json). The path segment
// selects the endpoint, which carries the token to present and the notification
// channels to deliver to:
//
//	POST /api/webhook/example
//	{"token": "...", "subject": "...", "content": "..."}
//
// The alert is rendered per channel and sent through every channel the endpoint
// lists in notifyPipes. This route is not part of the token-authenticated API:
// it authenticates with the endpoint's own shared token.
func WebhookHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		utils.SendErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	name := r.PathValue("name")
	endpoint, exists := config.GetWebhookEndpoint(name)
	if !exists {
		utils.SendErrorResponse(w, http.StatusNotFound, "unknown webhook endpoint: "+name)
		return
	}
	if !endpoint.Enabled {
		utils.SendErrorResponse(w, http.StatusForbidden, "webhook endpoint is disabled: "+name)
		return
	}
	// An endpoint without a token would accept requests from anyone, so it is
	// treated as a misconfiguration rather than as an open endpoint.
	if endpoint.Token == "" {
		postLog.Error("Webhook endpoint " + name + " has no token configured, rejecting request")
		utils.SendErrorResponse(w, http.StatusInternalServerError, "webhook endpoint is not configured with a token: "+name)
		return
	}

	var req webhookRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxWebhookBodyBytes)).Decode(&req); err != nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid request body: expected a JSON object with token, subject and content")
		return
	}

	if subtle.ConstantTimeCompare([]byte(req.Token), []byte(endpoint.Token)) != 1 {
		postLog.Warning("Webhook request rejected for endpoint " + name + ": invalid token")
		utils.SendErrorResponse(w, http.StatusUnauthorized, "invalid token")
		return
	}

	if req.Subject == "" || req.Content == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing required parameter: subject and content must not be empty")
		return
	}

	manager := controller.GetManager()
	if manager == nil {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "controller manager not initialized")
		return
	}

	alert := controller.Alert{
		Subject: req.Subject,
		Source:  name,
		Content: req.Content,
		Time:    time.Now().Format("2006-01-02T15:04:05.000000000-07:00"),
	}

	delivered, err := manager.NotifyAlert(endpoint.NotifyPipes, alert)
	if err != nil {
		postLog.Error("Failed to deliver webhook alert for endpoint " + name + ": " + err.Error())
		utils.SendErrorResponse(w, http.StatusBadGateway, "failed to send alert: "+err.Error())
		return
	}

	postLog.Info("Webhook alert delivered for endpoint " + name)
	utils.SendSuccessResponse(w, "alert sent", map[string]interface{}{
		"endpoint": name,
		"channels": delivered,
	})
}
