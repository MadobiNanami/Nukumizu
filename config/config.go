package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// LoadBotNodeConfig reads and parses bot_node_config.json and stores it as the
// global C_botNodeConfig singleton, mirroring LoadGlobalConfig. Unlike the
// other config files this one is auto-generated and optional: a missing or
// empty file yields an empty map, so every node falls back to its default
// enableStatusNotify value (true).
func LoadBotNodeConfig(configPath string) error {
	cfg := BotNodeMembers{}
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			C_botNodeConfig = cfg
			return nil
		}
		return fmt.Errorf("failed to read bot node config file: %w", err)
	}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &cfg); err != nil {
			return fmt.Errorf("failed to parse bot node config file: %w", err)
		}
	}
	C_botNodeConfig = cfg
	return nil
}

// NodeStatusNotifyEnabled reports whether the node identified by uuid should
// broadcast status-change notifications, per bot_node_config.json.
// enableStatusNotify defaults to true: a node notifies unless its entry
// explicitly sets the flag to false.
func NodeStatusNotifyEnabled(uuid string) bool {
	if C_botNodeConfig == nil {
		return true
	}
	opts, ok := C_botNodeConfig[uuid]
	if !ok || opts.EnableStatusNotify == nil {
		return true
	}
	return *opts.EnableStatusNotify
}

// LoadGlobalConfig reads and parses the configuration file, applies defaults,
// and stores it as a global singleton.
func LoadGlobalConfig(configPath string) (*Config, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Apply defaults for System.
	if cfg.System.ListenAddr == "" {
		cfg.System.ListenAddr = "0.0.0.0"
	}
	if cfg.System.ListenPort == "" {
		cfg.System.ListenPort = "8080"
	}

	// Apply defaults for QQ controller.
	if cfg.ControllerMethod.QQ.ListenMethod == "" {
		cfg.ControllerMethod.QQ.ListenMethod = "global"
	}
	if cfg.ControllerMethod.QQ.NapcatAddr == "" {
		cfg.ControllerMethod.QQ.NapcatAddr = "127.0.0.1"
	}
	if cfg.ControllerMethod.QQ.NapcatPort == "" {
		cfg.ControllerMethod.QQ.NapcatPort = "3000"
	}

	// Apply defaults for Telegram controller.
	if cfg.ControllerMethod.Telegram.ListenMethod == "" {
		cfg.ControllerMethod.Telegram.ListenMethod = "global"
	}

	// Apply defaults for Email controller.
	if cfg.ControllerMethod.Email.SMTPPort == 0 {
		cfg.ControllerMethod.Email.SMTPPort = 587
	}
	if cfg.ControllerMethod.Email.To == nil {
		cfg.ControllerMethod.Email.To = []string{}
	}

	// Apply defaults for Ntfy controller.
	if cfg.ControllerMethod.Ntfy.Server == "" {
		cfg.ControllerMethod.Ntfy.Server = "https://ntfy.sh"
	}
	if cfg.ControllerMethod.Ntfy.Priority == "" {
		cfg.ControllerMethod.Ntfy.Priority = "default"
	}

	// Apply defaults for Webhook controller.
	if cfg.ControllerMethod.Webhook.Method == "" {
		cfg.ControllerMethod.Webhook.Method = "POST"
	}
	if cfg.ControllerMethod.Webhook.Headers == nil {
		cfg.ControllerMethod.Webhook.Headers = map[string]string{}
	}

	// Apply defaults for paths.
	if cfg.DataPath == "" {
		cfg.DataPath = "./data"
	}
	if cfg.DBPath == "" {
		cfg.DBPath = "./db"
	}

	// Apply default message templates if not specified.
	if cfg.ControllerMessage.BotStarted == "" {
		cfg.ControllerMessage.BotStarted = "Nukumizu Alert Bot Started\nVersion: {{ softwareVersion }}.{{ softwareBuildVer }}.{{ softwareCommitHash }}.{{ softwareBuildType }}\nDeveloper: {{ softwareDeveloper }}\nTime: {{ time }}"
	}
	if cfg.ControllerMessage.BotHelp == "" {
		cfg.ControllerMessage.BotHelp = "Available commands:\n/help - Show this help message\n/list - List all servers\n/status <uuid> - Show status of a specific server\n/shutdown <uuid> - Shutdown a specific server\n/reboot <uuid> - Reboot a specific server\n/run <uuid> <command> - Run a command on a specific server\n/info - Show bot information"
	}
	if cfg.ControllerMessage.Tg_BotStart == "" {
		cfg.ControllerMessage.Tg_BotStart = "Welcome to use Nukumizu Alert Bot!\nUse /help to see available commands."
	}
	if cfg.ControllerMessage.ServerStatusChanged == "" {
		cfg.ControllerMessage.ServerStatusChanged = "Server Status Changed Alert\n{{ serverName }} - {{ upStatus }}\nEvent: {{ event }}\nServer Name: {{ serverName }}\nMessage: {{ message }}\nTime: {{ time }}"
	}
	if cfg.ControllerMessage.ServerList == "" {
		cfg.ControllerMessage.ServerList = "All server list:\nOnline:\n{{ list.onlineServers }}\nOffline:\n{{ list.offlineServers }}"
	}
	if cfg.ControllerMessage.ServerExecuteResult == "" {
		cfg.ControllerMessage.ServerExecuteResult = "Command execute result:\nServer Name: {{ serverName }}\nCommand: {{ command }}\n***Result***\n\n{{ result }}\n\n************\nTime: {{ time }}"
	}

	C_globalConfig = &cfg
	return &cfg, nil
}

// LoadBotUserConfig reads and parses bot_user_config.json and stores it as the
// global C_botUserConfig singleton, mirroring LoadGlobalConfig.
func LoadBotUserConfig(configPath string) (*BotUserConfig, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read bot user config file: %w", err)
	}

	var cfg BotUserConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse bot user config file: %w", err)
	}
	C_botUserConfig = &cfg
	return &cfg, nil
}

// SaveBotNodeConfig persists the given node UUIDs to the bot node config file,
// which is a plain registry of the nodes Komari reports (read back at startup
// by LoadBotNodeConfig). Entries already present are always preserved: a UUID
// Komari no longer reports on a given fetch is kept rather than deleted, and
// any enableStatusNotify a user set by hand is left untouched. UUIDs seen for
// the first time are added as an empty object, i.e. no parameter is written, so
// they inherit the enableStatusNotify default (true). The resulting JSON has
// its object keys emitted in sorted order by encoding/json, keeping the file
// deterministic across writes. The path is supplied by the caller (typically
// global.ConfigPath.BotNodeConfig).
func SaveBotNodeConfig(configPath string, uuids []string) error {
	// Serialize against admin edits of the same file via UpdateSettings so the
	// two read-modify-write paths cannot drop each other's changes.
	settingsLock.Lock()
	defer settingsLock.Unlock()

	// Start from whatever is already on disk so nothing is dropped. An empty or
	// missing file is treated as an empty map.
	members := make(BotNodeMembers)
	if data, err := os.ReadFile(configPath); err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &members); err != nil {
			return fmt.Errorf("failed to parse existing bot node config %s: %w", configPath, err)
		}
	}

	// Add every currently-fetched UUID, but never overwrite an entry that is
	// already configured.
	for _, uuid := range uuids {
		if _, ok := members[uuid]; !ok {
			members[uuid] = BotNodeOptions{}
		}
	}

	data, err := json.MarshalIndent(members, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal bot node config: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to write bot node config %s: %w", configPath, err)
	}
	return nil
}

// IsDebugMode returns whether debug mode is enabled.
func IsDebugMode() bool {
	if C_globalConfig == nil {
		return false
	}
	return C_globalConfig.System.DebugMode
}
