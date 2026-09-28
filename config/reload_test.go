package config

import (
	"reflect"
	"testing"

	"nukumizu-backend/global"
)

// swapReloadHooks replaces the registered reload hooks for the duration of a
// test and restores the previous set afterwards, so the package-level registry
// cannot leak into another test.
func swapReloadHooks(t *testing.T, hooks ...func(*Config)) {
	t.Helper()

	reloadHooksMu.Lock()
	original := reloadHooks
	reloadHooks = hooks
	reloadHooksMu.Unlock()

	t.Cleanup(func() {
		reloadHooksMu.Lock()
		reloadHooks = original
		reloadHooksMu.Unlock()
	})
}

func TestReloadHookRunsOnlyForGlobalSettings(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{"system":{"debugMode":false}}`)
	writeTempConfig(t, &global.ConfigPath.BotUserConfig, `{}`)
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, `{}`)

	var seen []bool
	swapReloadHooks(t, func(cfg *Config) {
		seen = append(seen, cfg.System.DebugMode)
	})

	// A config.json update runs the hook, with the values just written.
	if err := UpdateSettings(SettingGlobal, map[string]interface{}{
		"system": map[string]interface{}{"debugMode": true},
	}); err != nil {
		t.Fatalf("UpdateSettings(global): %v", err)
	}

	if len(seen) != 1 {
		t.Fatalf("hook ran %d times for a config.json update, want 1", len(seen))
	}
	if !seen[0] {
		t.Error("hook received a configuration without the updated debugMode")
	}

	// The other two files reload a singleton that callers read at the point of
	// use, so they have nothing to notify.
	for _, settingsType := range []string{SettingBotUserConfig, SettingBotNodeConfig} {
		if err := UpdateSettings(settingsType, map[string]interface{}{
			"unused": map[string]interface{}{"event_reply": true},
		}); err != nil {
			t.Fatalf("UpdateSettings(%s): %v", settingsType, err)
		}
	}

	if len(seen) != 1 {
		t.Errorf("hook ran %d times after updates to the other settings files, want 1", len(seen))
	}
}

func TestReloadHookIsolatesPanic(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{"system":{"debugMode":false}}`)

	reached := false
	swapReloadHooks(t,
		func(*Config) { panic("hook under test") },
		func(*Config) { reached = true },
	)

	// The configuration is already on disk and published by the time hooks run,
	// so a broken hook must not turn a successful write into a failed request.
	if err := UpdateSettings(SettingGlobal, map[string]interface{}{
		"system": map[string]interface{}{"debugMode": true},
	}); err != nil {
		t.Fatalf("a panicking hook must not fail the settings write: %v", err)
	}
	if !reached {
		t.Error("a panicking hook stopped the hooks registered after it")
	}

	// The write itself must still have landed.
	cfg := Current()
	if cfg == nil || !cfg.System.DebugMode {
		t.Error("the settings write did not take effect")
	}
}

// TestUnrelatedUpdateLeavesControllerMethodAlone guards the trigger for a
// controller rebuild. Whether to rebuild is decided by comparing the whole
// controllerMethod section with the one the running controllers were built
// from, so reloading the file has to reproduce that section byte for byte. A
// default applied inconsistently — a nil recipient slice turned into an empty
// one on the second load, say — would make every settings edit look like a
// controller change and tear down every channel on each save.
func TestUnrelatedUpdateLeavesControllerMethodAlone(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{
    "controllerMethod": {
        "qq(napcat)": { "enabled": false },
        "email": { "enabled": false, "to": [] },
        "webhook": { "enabled": false, "headers": {} }
    }
}`)

	first, err := LoadGlobalConfig(global.ConfigPath.Global)
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	before := first.ControllerMethod

	if err := UpdateSettings(SettingGlobal, map[string]interface{}{
		"controllerMessage": map[string]interface{}{"BOT_STARTED": "hello"},
	}); err != nil {
		t.Fatalf("UpdateSettings: %v", err)
	}

	after := Current().ControllerMethod
	if !reflect.DeepEqual(before, after) {
		t.Errorf("an unrelated update changed the controllerMethod section:\nbefore: %+v\nafter:  %+v", before, after)
	}
}

// TestReloadHookSeesEveryWriterPath pins the invariant that each writer of
// config.json notifies, not just /api/settings/set: the webhook endpoint
// helpers go through the same channel.
func TestReloadHookSeesEveryWriterPath(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{
    "webhook": { "endpoints": {} }
}`)

	var seen int
	swapReloadHooks(t, func(*Config) { seen++ })

	if err := AddWebhookEndpoint("example", map[string]interface{}{
		"enabled":     true,
		"token":       "secret",
		"notifyPipes": []interface{}{"ntfy"},
	}); err != nil {
		t.Fatalf("AddWebhookEndpoint: %v", err)
	}
	if seen != 1 {
		t.Errorf("hook ran %d times after adding an endpoint, want 1", seen)
	}

	if err := ModifyWebhookEndpoint("example", map[string]interface{}{
		"enabled": false,
	}); err != nil {
		t.Fatalf("ModifyWebhookEndpoint: %v", err)
	}
	if seen != 2 {
		t.Errorf("hook ran %d times after modifying an endpoint, want 2", seen)
	}

	if err := DeleteWebhookEndpoint("example"); err != nil {
		t.Fatalf("DeleteWebhookEndpoint: %v", err)
	}
	if seen != 3 {
		t.Errorf("hook ran %d times after deleting an endpoint, want 3", seen)
	}

	// A rejected write changes nothing, so it must not notify either.
	if err := DeleteWebhookEndpoint("ghost"); err == nil {
		t.Error("deleting an unknown endpoint should fail")
	}
	if seen != 3 {
		t.Errorf("hook ran for a rejected write (%d notifications, want 3)", seen)
	}
}
