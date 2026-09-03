package config

import "sort"

// SystemConfig holds system-level configuration.
type SystemConfig struct {
	DebugMode    bool   `json:"debugMode"`
	ListenAddr   string `json:"listenAddr"`
	ListenPort   string `json:"listenPort"`
	NetworkProxy string `json:"networkProxy"`
}

// DebugConfig holds debug-level configuration.
type DebugConfig struct {
	ShowNapcatMsg       bool `json:"showNapcatMsg"`
	ShowNapcatAction    bool `json:"showNapcatAction"`
	ShowTelegramMsg     bool `json:"showTelegramMsg"`
	ShowTriggerCmdEcho  bool `json:"showTriggerCmdEcho"`
	ShowKomariTaskEcho  bool `json:"showKomariTaskEcho"`
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
	Enabled         bool   `json:"enabled"`
	NetworkUseProxy bool   `json:"networkUseProxy"`
	NapcatAddr      string `json:"napcatAddr"`
	NapcatPort      string `json:"napcatPort"`
	NapcatToken     string `json:"napcatToken"`
	BotQQID         int64  `json:"botQQID"`
	ListenMethod    string `json:"listenMethod"`
}

// TelegramConfig holds Telegram Bot controller configuration.
type TelegramConfig struct {
	Enabled         bool   `json:"enabled"`
	NetworkUseProxy bool   `json:"networkUseProxy"`
	BotToken        string `json:"botToken"`
	ListenMethod    string `json:"listenMethod"`
}

// EmailConfig holds Email notification controller configuration.
type EmailConfig struct {
	Enabled         bool     `json:"enabled"`
	NetworkUseProxy bool     `json:"networkUseProxy"`
	SMTPHost        string   `json:"smtpHost"`
	SMTPPort        int      `json:"smtpPort"`
	Username        string   `json:"username"`
	Password        string   `json:"password"`
	From            string   `json:"from"`
	To              []string `json:"to"`
	UseTLS          bool     `json:"useTLS"`
}

// NtfyConfig holds Ntfy notification controller configuration.
type NtfyConfig struct {
	Enabled         bool   `json:"enabled"`
	NetworkUseProxy bool   `json:"networkUseProxy"`
	Server          string `json:"server"`
	Topic           string `json:"topic"`
	Token           string `json:"token"`
	Priority        string `json:"priority"`
}

// WebhookConfig holds Webhook notification controller configuration.
type WebhookConfig struct {
	Enabled         bool              `json:"enabled"`
	NetworkUseProxy bool              `json:"networkUseProxy"`
	URL             string            `json:"url"`
	Method          string            `json:"method"`
	Headers         map[string]string `json:"headers"`
	Template        string            `json:"template"`
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
	BotStarted          string `json:"BOT_STARTED"`
	BotHelp             string `json:"BOT_HELP"`
	Tg_BotStart         string `json:"TG_BOT_START"`
	ServerStatusChanged string `json:"SERVER_STATUS_CHANGED"`
	ServerList          string `json:"SERVER_LIST"`
	ServerExecuteResult string `json:"SERVER_EXECUTE_RESULT"`
}

// Config is the top-level application configuration.
type Config struct {
	System            SystemConfig            `json:"system"`
	Debug             DebugConfig             `json:"debug"`
	Komari            KomariConfig            `json:"komari"`
	ControllerMethod  ControllerMethodConfig  `json:"controllerMethod"`
	ControllerMessage ControllerMessageConfig `json:"controllerMessage"`
	DataPath          string                  `json:"dataPath"`
	DBPath            string                  `json:"dbPath"`
}

var C_globalConfig *Config

// BotUserOptions holds per-member options stored in bot_user_config.json.
type BotUserOptions struct {
	// EventStatusNotify indicates whether this member is subscribed to node
	// status change notifications.
	EventStatusNotify bool `json:"event_status_notify"`

	// EventBotStarted indicates whether this member receives the automatic
	// messages the bot pushes on startup (welcome message and startup server
	// list).
	EventBotStarted bool `json:"event_bot_started"`

	// EventReply indicates whether this member receives automatic replies to
	// their commands (e.g. /status, /list). If false, the bot will not send any
	// reply to this member's commands.
	EventReply bool `json:"event_reply"`
}

// BotUserMembers maps a member ID (QQ number, Telegram @username or numeric
// user ID, or chat/group ID) to its per-member options.
type BotUserMembers map[string]BotUserOptions

// IDs returns the member IDs as a sorted slice. JSON object keys have no
// stable iteration order once decoded into a map, so the result is sorted to
// keep notifications and authorization checks deterministic.
func (m BotUserMembers) IDs() []string {
	ids := make([]string, 0, len(m))
	for id := range m {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// BotUser_QQConfig holds the admins and trusted groups for the QQ (Napcat) bot.
type BotUser_QQConfig struct {
	Admins        BotUserMembers `json:"admins"`
	TrustedGroups BotUserMembers `json:"trustedGroups"`
}

// BotUser_TelegramConfig holds the admins and trusted groups for the Telegram bot.
type BotUser_TelegramConfig struct {
	Admins        BotUserMembers `json:"admins"`
	TrustedGroups BotUserMembers `json:"trustedGroups"`
}

// BotUserConfig is the schema of bot_user_config.json, which lists the admins
// and trusted groups (chat targets) for each bot channel separately from the
// main config.json.
type BotUserConfig struct {
	QQ       BotUser_QQConfig       `json:"qq(napcat)"`
	Telegram BotUser_TelegramConfig `json:"telegram"`
}

var C_botUserConfig *BotUserConfig
