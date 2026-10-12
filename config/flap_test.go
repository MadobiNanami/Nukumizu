package config

import (
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
