package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nukumizu-backend/global"
)

// writeTempConfig writes content to a fresh temp file and points the matching
// global.ConfigPath field at it, returning a cleanup that restores the original.
func writeTempConfig(t *testing.T, field *string, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	original := *field
	*field = path
	t.Cleanup(func() { *field = original })
}

func TestUpdateSettingsDeepMerge(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.BotUserConfig, `{
    "qq(napcat)": {
        "admins": {
            "3526453517": {
                "event_status_notify": true,
                "event_bot_started": true
            }
        },
        "trustedGroups": {
            "740724778": {
                "event_status_notify": true,
                "event_bot_started": false
            }
        }
    }
}`)

	patch := map[string]interface{}{
		"qq(napcat)": map[string]interface{}{
			"admins": map[string]interface{}{
				"3526453517": map[string]interface{}{
					"event_status_notify": false, // toggle an existing nested flag
					"event_reply":         true,  // add a key that is not in the file
				},
			},
			"trustedGroups": map[string]interface{}{
				"12345": map[string]interface{}{ // add a whole new member
					"event_bot_started": true,
				},
			},
		},
	}
	if err := UpdateSettings(SettingBotUserConfig, patch); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	data, err := GetSettings(SettingBotUserConfig)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`"event_status_notify": false`,
		`"event_reply": true`,
		`"event_bot_started": true`,
		`"event_status_notify": true`, // sibling under trustedGroups 740724778 kept
		`"12345"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("merged config missing %q:\n%s", want, got)
		}
	}

	// The in-memory singleton must reflect the merged file too.
	if C_botUserConfig == nil {
		t.Fatal("C_botUserConfig not reloaded")
	}
	if C_botUserConfig.QQ.Admins["3526453517"].EventStatusNotify {
		t.Error("expected reloaded admin event_status_notify = false")
	}
	if !C_botUserConfig.QQ.Admins["3526453517"].EventReply {
		t.Error("expected reloaded admin event_reply = true")
	}
	if !C_botUserConfig.QQ.TrustedGroups["12345"].EventBotStarted {
		t.Error("expected new trusted group event_bot_started = true")
	}
}

func TestUpdateSettingsReplacesArraysAndKeepsNumbers(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{
    "system": {
        "debugMode": true,
        "listenPort": "8080"
    },
    "controllerMethod": {
        "email": {
            "enabled": false,
            "smtpHost": "smtp.example.com",
            "smtpPort": 587,
            "to": ["old@example.com"]
        }
    }
}`)

	patch := map[string]interface{}{
		"system": map[string]interface{}{
			"debugMode": false, // partial: listenPort must survive
		},
		"controllerMethod": map[string]interface{}{
			"email": map[string]interface{}{
				"enabled":  true,
				"smtpPort": json.Number("465"),
				"to":       []interface{}{"new@example.com"}, // arrays replace, not merge
			},
		},
	}
	if err := UpdateSettings(SettingGlobal, patch); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	data, err := GetSettings(SettingGlobal)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`"debugMode": false`,
		`"listenPort": "8080"`,           // sibling untouched
		`"smtpHost": "smtp.example.com"`, // sibling untouched
		`"smtpPort": 465`,                // number kept verbatim, not 465.0
		`"new@example.com"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("merged config missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "old@example.com") {
		t.Errorf("array was merged instead of replaced:\n%s", got)
	}
}

func TestSettingsTypeValidation(t *testing.T) {
	for _, valid := range []string{SettingGlobal, SettingBotUserConfig, SettingBotNodeConfig} {
		if !IsValidSettingsType(valid) {
			t.Errorf("expected %q to be a valid settings type", valid)
		}
	}
	for _, invalid := range []string{"", "system", "node", "bot"} {
		if IsValidSettingsType(invalid) {
			t.Errorf("expected %q to be an invalid settings type", invalid)
		}
		if _, err := GetSettings(invalid); err != ErrUnsupportedSettingsType {
			t.Errorf("GetSettings(%q) error = %v, want ErrUnsupportedSettingsType", invalid, err)
		}
	}
}

func TestGetSettingsBotNodeMissingFile(t *testing.T) {
	// Point at a temp path that does not exist yet.
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, "")
	os.Remove(global.ConfigPath.BotNodeConfig)

	data, err := GetSettings(SettingBotNodeConfig)
	if err != nil {
		t.Fatalf("GetSettings on missing bot_node_config: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("expected empty object for missing bot_node_config, got %s", data)
	}
}

func TestUpdateSettingsRemovesKeysWithNull(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.BotUserConfig, `{
    "qq(napcat)": {
        "admins": {
            "3526453517": { "event_status_notify": true },
            "740724778": { "event_status_notify": false }
        },
        "trustedGroups": {
            "999": { "event_bot_started": true }
        }
    },
    "telegram": {
        "admins": {}
    }
}`)

	// null removes a nested member and keeps its siblings; an empty section stays.
	patch := map[string]interface{}{
		"qq(napcat)": map[string]interface{}{
			"admins": map[string]interface{}{
				"3526453517": nil,
			},
		},
	}
	if err := UpdateSettings(SettingBotUserConfig, patch); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	data, err := GetSettings(SettingBotUserConfig)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	got := string(data)
	if strings.Contains(got, "3526453517") {
		t.Errorf("deleted member still present:\n%s", got)
	}
	for _, want := range []string{"740724778", `"trustedGroups"`, `"telegram"`} {
		if !strings.Contains(got, want) {
			t.Errorf("unrelated content missing %q:\n%s", want, got)
		}
	}
}
