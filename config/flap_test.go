package config

import (
	"encoding/json"
	"testing"
	"time"

	"nukumizu-backend/global"
)

// TestNodeFlapTolerant covers the flap-inhibition window accessor against the
// per-node options in bot_node_config.json, including the hand-written values
// the comment on BotNodeOptions.FlapTolerant promises to accept.
func TestNodeFlapTolerant(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, `{
    "seconds": { "flap_tolerant": 30 },
    "fractional": { "flap_tolerant": 2.5 },
    "zero": { "flap_tolerant": 0 },
    "negative": { "flap_tolerant": -5 },
    "unset": { "enableStatusNotify": true },
    "absurd": { "flap_tolerant": 1e9 },
    "throughput": { "flap_tolerant": 8, "enableStatusNotify": false }
}`)
	if err := LoadBotNodeConfig(global.ConfigPath.BotNodeConfig); err != nil {
		t.Fatalf("LoadBotNodeConfig: %v", err)
	}

	cases := []struct {
		name string
		uuid string
		want time.Duration
	}{
		{"whole seconds", "seconds", 30 * time.Second},
		{"fractional seconds parse", "fractional", 2500 * time.Millisecond},
		{"zero disables", "zero", 0},
		{"negative disables", "negative", 0},
		{"absent field disables", "unset", 0},
		{"unknown node disables", "not-in-the-file", 0},
		{"absurd value is clamped", "absurd", maxFlapTolerantSeconds * time.Second},
		{"independent of enableStatusNotify", "throughput", 8 * time.Second},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NodeFlapTolerant(tc.uuid); got != tc.want {
				t.Errorf("NodeFlapTolerant(%q) = %v, want %v", tc.uuid, got, tc.want)
			}
		})
	}
}

// TestNodeFlapTolerantWithoutConfig pins the nil-safe default: before any
// bot_node_config.json has been loaded — and for a file that is missing or
// empty — no node has a window, so notifications keep their previous
// notify-immediately behavior.
func TestNodeFlapTolerantWithoutConfig(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, ``)
	if err := LoadBotNodeConfig(global.ConfigPath.BotNodeConfig); err != nil {
		t.Fatalf("LoadBotNodeConfig: %v", err)
	}

	if got := NodeFlapTolerant("anything"); got != 0 {
		t.Errorf("node flap tolerance from an empty file = %v, want 0", got)
	}
}

// nodeEntry reads one node's raw object back from the settings file, the way
// GetSettings serves it to the console.
func nodeEntry(t *testing.T, uuid string) map[string]interface{} {
	t.Helper()
	data, err := GetSettings(SettingBotNodeConfig)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	var all map[string]map[string]interface{}
	if err := json.Unmarshal(data, &all); err != nil {
		t.Fatalf("unmarshal settings: %v", err)
	}
	return all[uuid]
}

// TestFlapTolerantSettingsRoundTrip drives the payload the console's node card
// actually posts — { "<uuid>": { "flap_tolerant": <seconds> } } — through the
// settings API, so the frontend and the accessor are checked against each other
// and not just against hand-written files.
func TestFlapTolerantSettingsRoundTrip(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, `{
    "node-a": { "enableStatusNotify": false }
}`)
	if err := LoadBotNodeConfig(global.ConfigPath.BotNodeConfig); err != nil {
		t.Fatalf("LoadBotNodeConfig: %v", err)
	}

	patch := map[string]interface{}{
		"node-a": map[string]interface{}{"flap_tolerant": 30},
	}
	if err := UpdateSettings(SettingBotNodeConfig, patch); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	if got := NodeFlapTolerant("node-a"); got != 30*time.Second {
		t.Errorf("NodeFlapTolerant after save = %v, want 30s", got)
	}
	// The sibling option must survive: the card saves only the field it edits.
	entry := nodeEntry(t, "node-a")
	if entry["enableStatusNotify"] != false {
		t.Errorf("enableStatusNotify = %v, want it preserved as false", entry["enableStatusNotify"])
	}
	if entry["flap_tolerant"] != float64(30) {
		t.Errorf("flap_tolerant on disk = %v, want 30", entry["flap_tolerant"])
	}
}

// TestFlapTolerantSettingsRemoval: clearing the console's field sends a JSON
// null, which must remove the key rather than leave an explicit 0 behind.
func TestFlapTolerantSettingsRemoval(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, `{
    "node-a": { "enableStatusNotify": true, "flap_tolerant": 30 }
}`)
	if err := LoadBotNodeConfig(global.ConfigPath.BotNodeConfig); err != nil {
		t.Fatalf("LoadBotNodeConfig: %v", err)
	}

	patch := map[string]interface{}{
		"node-a": map[string]interface{}{"flap_tolerant": nil},
	}
	if err := UpdateSettings(SettingBotNodeConfig, patch); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	if got := NodeFlapTolerant("node-a"); got != 0 {
		t.Errorf("NodeFlapTolerant after removal = %v, want 0", got)
	}
	entry := nodeEntry(t, "node-a")
	if _, present := entry["flap_tolerant"]; present {
		t.Error("flap_tolerant is still in the file after being cleared")
	}
	if entry["enableStatusNotify"] != true {
		t.Errorf("enableStatusNotify = %v, want it preserved as true", entry["enableStatusNotify"])
	}
}
