package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"

	"nukumizu-backend/global"
)

// Settings types accepted by the /api/settings/get and /api/settings/set
// endpoints. Each maps 1:1 to a JSON configuration file on disk.
const (
	SettingGlobal        = "global"
	SettingBotUserConfig = "bot_user_config"
	SettingBotNodeConfig = "bot_node_config"
)

// ErrUnsupportedSettingsType is returned when a settings type is not one of the
// accepted constants above.
var ErrUnsupportedSettingsType = errors.New("unsupported settings type")

// settingsLock serializes read-modify-write access to the on-disk configuration
// files so concurrent admin edits (UpdateSettings) and the node tracker's
// background save (SaveBotNodeConfig) cannot lose each other's updates.
var settingsLock sync.RWMutex

// settingsPath resolves a settings type to its JSON configuration file path.
func settingsPath(settingsType string) (string, error) {
	switch settingsType {
	case SettingGlobal:
		return global.ConfigPath.Global, nil
	case SettingBotUserConfig:
		return global.ConfigPath.BotUserConfig, nil
	case SettingBotNodeConfig:
		return global.ConfigPath.BotNodeConfig, nil
	default:
		return "", ErrUnsupportedSettingsType
	}
}

// IsValidSettingsType reports whether the given string is one of the accepted
// settings types.
func IsValidSettingsType(settingsType string) bool {
	_, err := settingsPath(settingsType)
	return err == nil
}

// GetSettings returns the raw JSON of the file backing the given settings type,
// byte-for-byte the same content as the source file. bot_node_config.json is
// optional and auto-generated, so a missing or empty file yields an empty
// object.
func GetSettings(settingsType string) ([]byte, error) {
	path, err := settingsPath(settingsType)
	if err != nil {
		return nil, err
	}

	settingsLock.RLock()
	defer settingsLock.RUnlock()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) && settingsType == SettingBotNodeConfig {
			return []byte("{}"), nil
		}
		return nil, fmt.Errorf("failed to read %s settings file: %w", settingsType, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return []byte("{}"), nil
	}
	return data, nil
}

// UpdateSettings merges the given partial update into the JSON file backing the
// given settings type and persists the result back to disk. Because the merge is
// recursive, a payload such as {"system":{"debugMode":true}} only touches the
// nested keys it names and leaves every sibling key untouched. After the file is
// written the matching in-memory singleton is reloaded so runtime code observes
// the new values.
func UpdateSettings(settingsType string, patch map[string]interface{}) error {
	path, err := settingsPath(settingsType)
	if err != nil {
		return err
	}

	settingsLock.Lock()
	defer settingsLock.Unlock()

	// Start from whatever is already on disk so nothing is dropped. A missing or
	// empty file is treated as an empty object.
	current := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &current); err != nil {
				return fmt.Errorf("failed to parse existing %s settings file %s: %w", settingsType, path, err)
			}
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing %s settings file %s: %w", settingsType, path, err)
	}

	deepMergeSettings(current, patch)

	data, err = json.MarshalIndent(current, "", "    ")
	if err != nil {
		return fmt.Errorf("failed to marshal %s settings: %w", settingsType, err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write %s settings file %s: %w", settingsType, path, err)
	}

	return reloadSettings(settingsType, path)
}

// deepMergeSettings recursively overlays src onto dst. Object values merge
// key-by-key so partial updates keep sibling keys untouched; arrays and scalars
// always replace the destination value.
func deepMergeSettings(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		srcObj, srcIsObj := srcVal.(map[string]interface{})
		if srcIsObj {
			if dstObj, ok := dst[key].(map[string]interface{}); ok {
				deepMergeSettings(dstObj, srcObj)
			} else {
				dst[key] = srcVal
			}
			continue
		}
		dst[key] = srcVal
	}
}

// reloadSettings refreshes the in-memory singleton for the given settings type
// so the running program observes the values just persisted to disk.
func reloadSettings(settingsType, path string) error {
	switch settingsType {
	case SettingGlobal:
		_, err := LoadGlobalConfig(path)
		return err
	case SettingBotUserConfig:
		_, err := LoadBotUserConfig(path)
		return err
	case SettingBotNodeConfig:
		return LoadBotNodeConfig(path)
	default:
		return ErrUnsupportedSettingsType
	}
}
