package pipes

import (
	"fmt"

	gomail "gopkg.in/mail.v2"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// EmailController handles email notifications via SMTP.
type EmailController struct {
	cfg config.EmailConfig
}

// NewEmailController creates a new Email controller.
func NewEmailController(cfg config.EmailConfig) *EmailController {
	return &EmailController{cfg: cfg}
}

// Name returns the controller name.
func (e *EmailController) Name() string {
	return "email"
}

// Start initializes the Email controller.
func (e *EmailController) Start() error {
	if !e.cfg.Enabled {
		postLog.Info("Email controller is disabled")
		return nil
	}
	postLog.Info("Email controller started")
	return nil
}

// Stop shuts down the Email controller.
func (e *EmailController) Stop() {
	postLog.Info("Email controller stopped")
}

// IsEnabled returns whether the controller is enabled.
func (e *EmailController) IsEnabled() bool {
	return e.cfg.Enabled
}

// HandleCommand is not supported for Email (status-only controller).
// This controller does not implement CommandController.

// SendStatusChange sends a status change notification via Email.
func (e *EmailController) SendStatusChange(change node.StatusChange) error {
	if !e.cfg.Enabled {
		return nil
	}
	if len(e.cfg.To) == 0 {
		postLog.Debug("Email controller has no recipients configured")
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromStatusChange(change)
	body := template.Render(cfg.ControllerMessage.ServerStatusChanged, params)

	subject := fmt.Sprintf("Server Status Change: %s - %s", change.Name, change.Event)
	return e.sendEmail(subject, body)
}

// SendServerList sends the server list via Email.
func (e *EmailController) SendServerList(onlineServers, offlineServers string) error {
	if !e.cfg.Enabled || len(e.cfg.To) == 0 {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromServerList()
	body := template.Render(cfg.ControllerMessage.ServerList, params)

	return e.sendEmail("Server List", body)
}

// SendExecuteResult sends a command execution result via Email.
func (e *EmailController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	if !e.cfg.Enabled || len(e.cfg.To) == 0 {
		return nil
	}

	cfg := config.GetConfig()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	body := template.Render(cfg.ControllerMessage.ServerExecuteResult, params)

	subject := fmt.Sprintf("Command Result: %s on %s", command, serverName)
	return e.sendEmail(subject, body)
}

func (e *EmailController) sendEmail(subject, body string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", e.cfg.From)
	m.SetHeader("To", e.cfg.To...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(e.cfg.SMTPHost, e.cfg.SMTPPort, e.cfg.Username, e.cfg.Password)

	if err := d.DialAndSend(m); err != nil {
		postLog.Warning("Failed to send email: " + err.Error())
		return err
	}

	postLog.Debug("Email sent successfully to " + fmt.Sprintf("%v", e.cfg.To))
	return nil
}
