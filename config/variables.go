package config

import (
	"sort"
	"sync/atomic"
)

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
	Markdown     	bool   `json:"markdown"`
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
	Markdown     	bool   `json:"markdown"`
	Enabled         bool   `json:"enabled"`
	NetworkUseProxy bool   `json:"networkUseProxy"`
	BotToken        string `json:"botToken"`
	ListenMethod    string `json:"listenMethod"`
}

// EmailConfig holds Email notification controller configuration.
type EmailConfig struct {
	Markdown     	bool   `json:"markdown"`
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
	Markdown     	bool   `json:"markdown"`
	Enabled         bool   `json:"enabled"`
	NetworkUseProxy bool   `json:"networkUseProxy"`
	Server          string `json:"server"`
	Topic           string `json:"topic"`
	Token           string `json:"token"`
	Priority        string `json:"priority"`
}

// WebhookConfig holds the outgoing Webhook notification controller
// configuration. It is the counterpart of WebhookReceiverConfig, which serves
// the incoming webhook API.
type WebhookConfig struct {
	Markdown     	bool  			  `json:"markdown"`
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

// WebhookEndpointConfig holds a single incoming webhook endpoint. Endpoints are
// keyed by name under webhook.endpoints; the name is the last path segment of
// the endpoint's URL, so an endpoint named "example" is served at
// POST /api/webhook/example. One endpoint per external application and target
// channel group keeps their tokens and recipients apart.
type WebhookEndpointConfig struct {
	// Enabled controls whether the endpoint accepts requests. A disabled
	// endpoint answers with 403.
	Enabled bool `json:"enabled"`

	// Token is the shared secret the caller must send in the request body. An
	// endpoint without a token is rejected: an empty token would make the
	// endpoint an open relay, so it is treated as a configuration error.
	Token string `json:"token"`

	// NotifyPipes lists the notification channels the alert is delivered to, by
	// controller name (e.g. "qq(napcat)", "telegram", "email", "ntfy",
	// "webhook").
	NotifyPipes []string `json:"notifyPipes"`
}

// WebhookReceiverConfig holds the incoming webhook API settings. The API is
// served on its own listener instead of the main one, so external applications
// can be given access to the webhook port without exposing the admin API. Only
// the endpoints map is re-read on a settings update; enabled, listenAddr and
// listenPort are applied at startup.
type WebhookReceiverConfig struct {
	Enabled    bool                             `json:"enabled"`
	ListenAddr string                           `json:"listenAddr"`
	ListenPort string                           `json:"listenPort"`
	Endpoints  map[string]WebhookEndpointConfig `json:"endpoints"`
}

// GetWebhookEndpoint returns the incoming webhook endpoint registered under the
// given name, and whether such an endpoint exists.
func GetWebhookEndpoint(name string) (WebhookEndpointConfig, bool) {
	cfg := Current()
	if cfg == nil {
		return WebhookEndpointConfig{}, false
	}
	endpoint, ok := cfg.Webhook.Endpoints[name]
	return endpoint, ok
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
	Webhook           WebhookReceiverConfig   `json:"webhook"`
	ControllerMethod  ControllerMethodConfig  `json:"controllerMethod"`
	ControllerMessage ControllerMessageConfig `json:"controllerMessage"`
	DataPath          string                  `json:"dataPath"`
	DBPath            string                  `json:"dbPath"`
}

// globalConfig holds the configuration currently in effect. It is replaced as a
// whole by LoadGlobalConfig — and therefore by every settings update — and never
// mutated in place, so a reader that loads the pointer always observes a fully
// initialized Config. Read it through Current rather than caching the result: a
// cached pointer stops tracking reloads.
var globalConfig atomic.Pointer[Config]

// Current returns the configuration currently in effect, or nil before the
// first successful LoadGlobalConfig. It is safe to call from any goroutine, and
// must be called on every use rather than stored, so the caller sees reloads.
func Current() *Config {
	return globalConfig.Load()
}

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

// botUserConfig mirrors bot_user_config.json the same way globalConfig mirrors
// config.json: replaced wholesale on reload and read through BotUsers.
var botUserConfig atomic.Pointer[BotUserConfig]

// BotUsers returns the bot user configuration currently in effect, or nil
// before the first successful LoadBotUserConfig. Like Current it must be called
// on every use rather than stored.
func BotUsers() *BotUserConfig {
	return botUserConfig.Load()
}

// BotNodeOptions holds per-node options stored in bot_node_config.json. The
// file is auto-populated by the node tracker for every node Komari reports;
// per-node options are edited by hand in the JSON file.
type BotNodeOptions struct {
	// EnableStatusNotify controls whether this node broadcasts status-change
	// notifications. It defaults to true: only an explicit false in the JSON
	// file disables a node's notifications. The pointer (rather than a plain
	// bool) lets an absent field be told apart from an explicit false, and
	// omitempty keeps untouched nodes stored as {}.
	EnableStatusNotify *bool `json:"enableStatusNotify,omitempty"`
}

// BotNodeMembers maps a node UUID (as reported by Komari) to its per-node
// options.
type BotNodeMembers map[string]BotNodeOptions

// botNodeConfig mirrors bot_node_config.json, populated by LoadBotNodeConfig.
// The map is rebuilt rather than mutated on every load, so the pointer can be
// swapped atomically; read it through BotNodes.
var botNodeConfig atomic.Pointer[BotNodeMembers]

// BotNodes returns the per-node options currently in effect, or nil before the
// first LoadBotNodeConfig. Like Current it must be called on every use rather
// than stored.
func BotNodes() BotNodeMembers {
	nodes := botNodeConfig.Load()
	if nodes == nil {
		return nil
	}
	return *nodes
}
