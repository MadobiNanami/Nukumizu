package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// LoadConfig reads and parses the configuration file, applies defaults,
// and stores it as a global singleton.
func LoadConfig(configPath string) (*Config, error) {
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
	if cfg.ControllerMethod.QQ.Admins == nil {
		cfg.ControllerMethod.QQ.Admins = []string{}
	}
	if cfg.ControllerMethod.QQ.TrustedGroups == nil {
		cfg.ControllerMethod.QQ.TrustedGroups = []string{}
	}

	// Apply defaults for Telegram controller.
	if cfg.ControllerMethod.Telegram.ListenMethod == "" {
		cfg.ControllerMethod.Telegram.ListenMethod = "global"
	}
	if cfg.ControllerMethod.Telegram.Admins == nil {
		cfg.ControllerMethod.Telegram.Admins = []string{}
	}
	if cfg.ControllerMethod.Telegram.TrustedGroups == nil {
		cfg.ControllerMethod.Telegram.TrustedGroups = []string{}
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
		cfg.DBPath = "./logs.db"
	}

	// Apply default message templates if not specified.
	if cfg.ControllerMessage.ServerStatusChanged == "" {
		cfg.ControllerMessage.ServerStatusChanged = "Server Status Changed Alert\n{{ serverName }} - {{ upStatus }}\nEvent: {{ event }}\nServer Name: {{ serverName }}\nMessage: {{ message }}\nTime: {{ time }}"
	}
	if cfg.ControllerMessage.ServerList == "" {
		cfg.ControllerMessage.ServerList = "All server list:\nOnline:\n{{ list.onlineServers }}\nOffline:\n{{ list.offlineServers }}"
	}
	if cfg.ControllerMessage.ServerExecuteResult == "" {
		cfg.ControllerMessage.ServerExecuteResult = "Command execute result:\nServer Name: {{ serverName }}\nCommand: {{ command }}\n***Result***\n\n{{ result }}\n\n************\nTime: {{ time }}"
	}

	globalConfig = &cfg
	return &cfg, nil
}

// GetConfig returns the global configuration singleton.
func GetConfig() *Config {
	return globalConfig
}

// IsDebugMode returns whether debug mode is enabled.
func IsDebugMode() bool {
	if globalConfig == nil {
		return false
	}
	return globalConfig.System.DebugMode
}
