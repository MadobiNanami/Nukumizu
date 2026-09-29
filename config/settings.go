package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
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

// startupOnlySettings are the config.json keys that are read once before the
// program starts serving and never again: the listener addresses, the paths the
// databases are opened from, and the Komari dashboard its client is built
// against. Editing one writes the file and replaces the in-memory
// configuration, but the running program keeps the old value, so an update that
// touches one is reported back to the caller instead of being silently
// accepted.
//
// Keep this in step with main: these are exactly the settings main reads before
// the HTTP server comes up. Everything else — controllerMethod, networkProxy,
// the message templates, the debug switches — is picked up at runtime.
var startupOnlySettings = []string{
	"system.listenAddr",
	"system.listenPort",
	"webhook.enabled",
	"webhook.listenAddr",
	"webhook.listenPort",
	"komari.dashboardURL",
	"dataPath",
	"dbPath",
}

// IsRestartRequiredKeys lists the settings in patch that only take effect at
// startup, as dot-separated paths, in the order startupOnlySettings declares
// them. Only config.json carries such settings; an update to one of the other
// files always reports nothing.
//
// The write itself succeeds either way — this is advice for the user, not a
// rejection. The result is never nil, so a caller can put it straight into a
// JSON response and get [] rather than null.
func IsRestartRequiredKeys(settingsType string, patch map[string]interface{}) []string {
	keys := []string{}
	if settingsType != SettingGlobal {
		return keys
	}

	patched := patchPaths(patch)
	for _, watched := range startupOnlySettings {
		for _, path := range patched {
			if isPathsOverlap(path, watched) {
				keys = append(keys, watched)
				break
			}
		}
	}
	return keys
}

// patchPaths expands a nested settings patch into the dot-separated paths of its
// leaves. An object is descended into rather than reported, so a patch that only
// names sections still resolves to the keys it changes, and a JSON null is a
// leaf because it deletes the key it names.
func patchPaths(patch map[string]interface{}) []string {
	paths := []string{}
	var walk func(prefix string, node map[string]interface{})
	walk = func(prefix string, node map[string]interface{}) {
		for key, value := range node {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			if nested, ok := value.(map[string]interface{}); ok && nested != nil {
				walk(path, nested)
				continue
			}
			paths = append(paths, path)
		}
	}
	walk("", patch)
	return paths
}

// isPathsOverlap reports whether a patched path and a watched setting can affect
// each other: they are the same key, the patch names something inside the
// watched setting, or the patch names a section the watched setting lives in.
// The last case matters because a patch may replace a whole section, which
// changes every key under it.
func isPathsOverlap(patched, watched string) bool {
	return patched == watched ||
		strings.HasPrefix(patched, watched+".") ||
		strings.HasPrefix(watched, patched+".")
}

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
	return runSettingsUpdate(func() (*Config, error) {
		return updateSettingsLocked(settingsType, patch)
	})
}

// runSettingsUpdate runs fn under settingsLock and then, once the lock is
// released, runs the reload hooks with whatever configuration fn reports (nil
// when the update did not touch config.json).
//
// The hooks deliberately run outside settingsLock. A hook rebuilds controllers,
// which can wait on a network call, while settingsLock is also held by the node
// tracker's background save (SaveBotNodeConfig); holding it across a hook would
// stall node registration behind an unrelated settings edit.
func runSettingsUpdate(fn func() (*Config, error)) error {
	cfg, err := func() (*Config, error) {
		settingsLock.Lock()
		defer settingsLock.Unlock()
		return fn()
	}()
	if err != nil {
		return err
	}

	notifyReload(cfg)
	return nil
}

// updateSettingsLocked is UpdateSettings without the locking, for callers that
// need to inspect the loaded configuration and write in one critical section
// (see the incoming webhook endpoint helpers). Callers must hold settingsLock.
//
// It returns the freshly loaded global configuration, or nil when the settings
// type is one of the other files. The caller is responsible for handing that
// value to notifyReload once settingsLock is released — which runSettingsUpdate
// does for every writer.
func updateSettingsLocked(settingsType string, patch map[string]interface{}) (*Config, error) {
	path, err := settingsPath(settingsType)
	if err != nil {
		return nil, err
	}

	// Start from whatever is already on disk so nothing is dropped. A missing or
	// empty file is treated as an empty object.
	current := map[string]interface{}{}
	data, err := os.ReadFile(path)
	if err == nil {
		if len(bytes.TrimSpace(data)) > 0 {
			if err := json.Unmarshal(data, &current); err != nil {
				return nil, fmt.Errorf("failed to parse existing %s settings file %s: %w", settingsType, path, err)
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read existing %s settings file %s: %w", settingsType, path, err)
	}

	deepMergeSettings(current, patch)

	data, err = json.MarshalIndent(current, "", "    ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal %s settings: %w", settingsType, err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write %s settings file %s: %w", settingsType, path, err)
	}

	return reloadSettings(settingsType, path)
}

// deepMergeSettings recursively overlays src onto dst. Object values merge
// key-by-key so partial updates keep sibling keys untouched; arrays and scalars
// always replace the destination value. A JSON null in the payload removes that
// key from dst, giving clients a way to delete entries (members, nodes, map
// rows) through /api/settings/set.
func deepMergeSettings(dst, src map[string]interface{}) {
	for key, srcVal := range src {
		if srcVal == nil {
			delete(dst, key)
			continue
		}
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
// so the running program observes the values just persisted to disk. Only
// config.json has a hook-visible reload, so for SettingGlobal it returns the
// configuration now in effect and for the other types it returns nil.
func reloadSettings(settingsType, path string) (*Config, error) {
	switch settingsType {
	case SettingGlobal:
		return LoadGlobalConfig(path)
	case SettingBotUserConfig:
		_, err := LoadBotUserConfig(path)
		return nil, err
	case SettingBotNodeConfig:
		return nil, LoadBotNodeConfig(path)
	default:
		return nil, ErrUnsupportedSettingsType
	}
}
