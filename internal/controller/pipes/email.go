package pipes

import (
	"fmt"
	"net"
	"reflect"
	"sync/atomic"

	gomail "gopkg.in/mail.v2"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/controller"
	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
)

// EmailController handles email notifications via SMTP.
//
// The configuration is held behind an atomic pointer rather than in a plain
// field: Reload replaces it on the settings-update goroutine while the send
// methods read it on the status-change and incoming-webhook goroutines.
type EmailController struct {
	cfg atomic.Pointer[config.EmailConfig]
}

// NewEmailController creates a new Email controller.
func NewEmailController(cfg config.EmailConfig) *EmailController {
	e := &EmailController{}
	e.cfg.Store(&cfg)
	applyEmailProxy(cfg.NetworkUseProxy)
	return e
}

// settings returns the configuration currently in effect. The value it points
// at is never mutated after being stored, so a caller can hold the pointer for
// one whole send without a concurrent Reload disturbing it.
func (e *EmailController) settings() *config.EmailConfig {
	return e.cfg.Load()
}

// applyEmailProxy routes SMTP through the HTTP CONNECT proxy, or restores a
// direct dial. gomail exposes the dial path as the package-level
// NetDialTimeout, whose own default is net.DialTimeout, so turning the proxy
// off has to put that back rather than leave the hook in place. There is a
// single global email channel, so setting a package-level hook here is
// unambiguous.
func applyEmailProxy(useProxy bool) {
	if useProxy {
		gomail.NetDialTimeout = netproxy.DialWithTimeout(true)
		return
	}
	gomail.NetDialTimeout = net.DialTimeout
}

// Name returns the controller name.
func (e *EmailController) Name() string {
	return "email"
}

// Start initializes the Email controller.
func (e *EmailController) Start() error {
	if !e.settings().Enabled {
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

// Reload applies the current configuration. Swapping in the new settings is
// enough for everything read at the point of use; the proxy setting is the
// exception, because it is baked into gomail's package-level dialer when the
// controller is built rather than consulted per send.
func (e *EmailController) Reload() {
	global := config.Current()
	if global == nil {
		return
	}

	current := e.settings()
	updated := global.ControllerMethod.Email
	// EmailConfig carries a recipient slice, so it is not comparable with ==.
	if reflect.DeepEqual(*current, updated) {
		return
	}

	if current.NetworkUseProxy != updated.NetworkUseProxy {
		applyEmailProxy(updated.NetworkUseProxy)
	}
	e.cfg.Store(&updated)
	postLog.Debug("Email controller reloaded")
}

// IsEnabled returns whether the controller is enabled.
func (e *EmailController) IsEnabled() bool {
	return e.settings().Enabled
}

// IsMarkdown returns whether the channel renders Markdown, per its markdown
// setting in config.json.
func (e *EmailController) IsMarkdown() bool {
	return e.settings().Markdown
}

// SendStatusChange sends a status change notification via Email.
func (e *EmailController) SendStatusChange(change node.StatusChange) error {
	s := e.settings()
	if !s.Enabled {
		return nil
	}
	if len(s.To) == 0 {
		postLog.Debug("Email controller has no recipients configured")
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromStatusChange(change)
	body := template.Render(cfg.ControllerMessage.ServerStatusChanged, params, s.Markdown)

	subject := fmt.Sprintf("Server Status Change: %s - %s", change.Name, change.Event)
	return e.sendEmail(s, subject, body)
}

// SendServerList sends the server list via Email.
func (e *EmailController) SendServerList(onlineServers, offlineServers string) error {
	s := e.settings()
	if !s.Enabled || len(s.To) == 0 {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromServerList()
	body := template.Render(cfg.ControllerMessage.ServerList, params, s.Markdown)

	return e.sendEmail(s, "Server List", body)
}

// SendExecuteResult sends a command execution result via Email.
func (e *EmailController) SendExecuteResult(serverName, serverUUID, command, result string) error {
	s := e.settings()
	if !s.Enabled || len(s.To) == 0 {
		return nil
	}

	cfg := config.Current()
	params := template.BuildParamsFromExecResult(serverName, serverUUID, command, result)
	body := template.Render(cfg.ControllerMessage.ServerExecuteResult, params, s.Markdown)

	subject := fmt.Sprintf("Command Result: %s on %s", command, serverName)
	return e.sendEmail(s, subject, body)
}

// SendAlert sends an alert submitted through the incoming webhook API to the
// configured recipients.
func (e *EmailController) SendAlert(alert controller.Alert) error {
	s := e.settings()
	if !s.Enabled {
		return nil
	}
	if len(s.To) == 0 {
		postLog.Debug("Email controller has no recipients configured")
		return nil
	}

	return e.sendEmail(s, alert.Subject, alert.Render(s.Markdown))
}

// sendEmail delivers one message using the settings the caller already
// snapshotted, so the recipients the guard approved are the recipients that
// receive it even if a reload lands mid-send.
func (e *EmailController) sendEmail(s *config.EmailConfig, subject, body string) error {
	m := gomail.NewMessage()
	m.SetHeader("From", s.From)
	m.SetHeader("To", s.To...)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(s.SMTPHost, s.SMTPPort, s.Username, s.Password)

	if err := d.DialAndSend(m); err != nil {
		postLog.Warning("Failed to send email: " + err.Error())
		return err
	}

	postLog.Debug("Email sent successfully to " + fmt.Sprintf("%v", s.To))
	return nil
}
