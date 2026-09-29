package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
            "100000001": {
                "event_status_notify": true,
                "event_bot_started": true
            }
        },
        "trustedGroups": {
            "200000002": {
                "event_status_notify": true,
                "event_bot_started": false
            }
        }
    }
}`)

	patch := map[string]interface{}{
		"qq(napcat)": map[string]interface{}{
			"admins": map[string]interface{}{
				"100000001": map[string]interface{}{
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
		`"event_status_notify": true`, // sibling under trustedGroups 200000002 kept
		`"12345"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("merged config missing %q:\n%s", want, got)
		}
	}

	// The in-memory singleton must reflect the merged file too.
	users := BotUsers()
	if users == nil {
		t.Fatal("bot user config not reloaded")
	}
	if users.QQ.Admins["100000001"].EventStatusNotify {
		t.Error("expected reloaded admin event_status_notify = false")
	}
	if !users.QQ.Admins["100000001"].EventReply {
		t.Error("expected reloaded admin event_reply = true")
	}
	if !users.QQ.TrustedGroups["12345"].EventBotStarted {
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

func TestRestartRequiredKeys(t *testing.T) {
	cases := []struct {
		name         string
		settingsType string
		patch        map[string]interface{}
		want         []string
	}{
		{
			name:         "a runtime switch needs no restart",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"system": map[string]interface{}{"debugMode": true}},
			want:         []string{},
		},
		{
			name:         "the listen port does",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"system": map[string]interface{}{"listenPort": "9090"}},
			want:         []string{"system.listenPort"},
		},
		{
			name:         "only the startup key of a mixed patch is reported",
			settingsType: SettingGlobal,
			patch: map[string]interface{}{
				"system": map[string]interface{}{"listenPort": "9090", "debugMode": true},
			},
			want: []string{"system.listenPort"},
		},
		{
			name:         "a top-level path is reported",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"dataPath": "/srv/data"},
			want:         []string{"dataPath"},
		},
		{
			name:         "results follow the declared order, not the patch order",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"dbPath": "/srv/db", "dataPath": "/srv/data"},
			want:         []string{"dataPath", "dbPath"},
		},
		{
			name:         "deleting a startup key with null is reported",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"system": map[string]interface{}{"listenPort": nil}},
			want:         []string{"system.listenPort"},
		},
		{
			name:         "replacing a whole section reports the startup keys inside it",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"webhook": map[string]interface{}{"listenAddr": "127.0.0.1"}},
			want:         []string{"webhook.listenAddr"},
		},
		{
			// Deleting the section resets the URL to its built-in default.
			name:         "deleting a section the startup key lives in reports it",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"komari": nil},
			want:         []string{"komari.dashboardURL"},
		},
		{
			name:         "deleting a section reports every startup key inside it",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"webhook": nil},
			want:         []string{"webhook.enabled", "webhook.listenAddr", "webhook.listenPort"},
		},
		{
			// An empty object merges nothing, so it changes no key and needs no
			// restart — surprising enough to pin.
			name:         "an empty object changes nothing",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{"komari": map[string]interface{}{}},
			want:         []string{},
		},
		{
			// webhook.endpoints must not be mistaken for webhook.enabled.
			name:         "a sibling subtree is not mistaken for the startup key",
			settingsType: SettingGlobal,
			patch: map[string]interface{}{
				"webhook": map[string]interface{}{
					"endpoints": map[string]interface{}{"example": map[string]interface{}{"enabled": true}},
				},
			},
			want: []string{},
		},
		{
			// The Komari credentials are re-read on the next login, so only the
			// dashboard URL is startup-only.
			name:         "komari credentials are not startup-only",
			settingsType: SettingGlobal,
			patch: map[string]interface{}{
				"komari": map[string]interface{}{
					"account": map[string]interface{}{"username": "admin", "password": "x"},
				},
			},
			want: []string{},
		},
		{
			name:         "controller settings are not startup-only",
			settingsType: SettingGlobal,
			patch: map[string]interface{}{
				"controllerMethod": map[string]interface{}{
					"telegram": map[string]interface{}{"enabled": true, "botToken": "t"},
				},
			},
			want: []string{},
		},
		{
			name:         "an empty patch reports nothing",
			settingsType: SettingGlobal,
			patch:        map[string]interface{}{},
			want:         []string{},
		},
		{
			// Only config.json has settings that are read once at startup.
			name:         "the other settings files never need a restart",
			settingsType: SettingBotUserConfig,
			patch:        map[string]interface{}{"dataPath": "/srv/data"},
			want:         []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsRestartRequiredKeys(tc.settingsType, tc.patch)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("RestartRequiredKeys() = %v, want %v", got, tc.want)
			}
			if got == nil {
				t.Error("the result must never be nil, so it serializes as [] rather than null")
			}
		})
	}
}

// TestRestartRequiredKeysIsAdvisory pins that reporting a startup-only key does
// not stop the write: the caller is told, the file is still updated.
func TestRestartRequiredKeysIsAdvisory(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{"system":{"listenPort":"8080"}}`)

	keys := IsRestartRequiredKeys(SettingGlobal, map[string]interface{}{
		"system": map[string]interface{}{"listenPort": "9090"},
	})
	if len(keys) != 1 {
		t.Fatalf("expected the listen port to be reported, got %v", keys)
	}

	if err := UpdateSettings(SettingGlobal, map[string]interface{}{
		"system": map[string]interface{}{"listenPort": "9090"},
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	if got := Current().System.ListenPort; got != "9090" {
		t.Errorf("the update was not applied: listenPort = %q", got)
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
            "100000001": { "event_status_notify": true },
            "200000002": { "event_status_notify": false }
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
				"100000001": nil,
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
	if strings.Contains(got, "100000001") {
		t.Errorf("deleted member still present:\n%s", got)
	}
	for _, want := range []string{"200000002", `"trustedGroups"`, `"telegram"`} {
		if !strings.Contains(got, want) {
			t.Errorf("unrelated content missing %q:\n%s", want, got)
		}
	}
}
