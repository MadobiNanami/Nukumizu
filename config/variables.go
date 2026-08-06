package config

// SystemConfig holds system-level configuration.
type SystemConfig struct {
	DebugMode  bool   `json:"debugMode"`
	ListenAddr string `json:"listenAddr"`
	ListenPort string `json:"listenPort"`
}

// DebugConfig holds debug-level configuration.
type DebugConfig struct {
	ShowNapcatMsg  bool `json:"showNapcatMsg"`
	ShowNapcatAction bool `json:"showNapcatAction"`
	ShowTelegramMsg bool `json:"showTelegramMsg"`
	ShowTriggerCmdEcho bool `json:"showTriggerCmdEcho"`
	NapcatIgnoreSelfMsg bool `json:"napcatIgnoreSelfMsg"`
}

// KomariAccount holds Komari login credentials.
type KomariAccount struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// KomariConfig holds Komari Dashboard connection settings.
type KomariConfig struct {
	DashboardURL string        `json:"dashboardURL"`
	Account      KomariAccount `json:"account"`
}

// QQConfig holds QQ (Napcat) Bot controller configuration.
type QQConfig struct {
	Enabled       bool     `json:"enabled"`
	NapcatAddr    string   `json:"napcatAddr"`
	NapcatPort    string   `json:"napcatPort"`
	NapcatToken   string   `json:"napcatToken"`
	BotQQID       int64    `json:"botQQID"`
	ListenMethod  string   `json:"listenMethod"`
	Admins        []string `json:"admins"`
	TrustedGroups []string `json:"trustedGroups"`
}

// TelegramConfig holds Telegram Bot controller configuration.
type TelegramConfig struct {
	Enabled       bool     `json:"enabled"`
	BotToken      string   `json:"botToken"`
	ListenMethod  string   `json:"listenMethod"`
	Admins        []string `json:"admins"`
	TrustedGroups []string `json:"trustedGroups"`
}

// EmailConfig holds Email notification controller configuration.
type EmailConfig struct {
	Enabled  bool     `json:"enabled"`
	SMTPHost string   `json:"smtpHost"`
	SMTPPort int      `json:"smtpPort"`
	Username string   `json:"username"`
	Password string   `json:"password"`
	From     string   `json:"from"`
	To       []string `json:"to"`
	UseTLS   bool     `json:"useTLS"`
}

// NtfyConfig holds Ntfy notification controller configuration.
type NtfyConfig struct {
	Enabled  bool   `json:"enabled"`
	Server   string `json:"server"`
	Topic    string `json:"topic"`
	Token    string `json:"token"`
	Priority string `json:"priority"`
}

// WebhookConfig holds Webhook notification controller configuration.
type WebhookConfig struct {
	Enabled  bool              `json:"enabled"`
	URL      string            `json:"url"`
	Method   string            `json:"method"`
	Headers  map[string]string `json:"headers"`
	Template string            `json:"template"`
}

// ControllerMethodConfig holds all controller method configurations.
type ControllerMethodConfig struct {
	QQ       QQConfig       `json:"qq(napcat)"`
	Telegram TelegramConfig `json:"telegram"`
	Email    EmailConfig    `json:"email"`
	Ntfy     NtfyConfig     `json:"ntfy"`
	Webhook  WebhookConfig  `json:"webhook"`
}

// ControllerMessageConfig holds message templates for controller responses.
type ControllerMessageConfig struct {
	ServerStatusChanged string `json:"SERVER_STATUS_CHANGED"`
	ServerList          string `json:"SERVER_LIST"`
	ServerExecuteResult string `json:"SERVER_EXECUTE_RESULT"`
}

// Config is the top-level application configuration.
type Config struct {
	System            SystemConfig            `json:"system"`
	Debug  		      DebugConfig             `json:"debug"`
	Komari            KomariConfig            `json:"komari"`
	ControllerMethod  ControllerMethodConfig  `json:"controllerMethod"`
	ControllerMessage ControllerMessageConfig `json:"controllerMessage"`
	DataPath          string                  `json:"dataPath"`
	DBPath            string                  `json:"dbPath"`
}

var globalConfig *Config