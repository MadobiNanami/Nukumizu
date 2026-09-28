package config

import (
	"sync"
	"testing"

	"nukumizu-backend/global"
)

// TestConcurrentReloadAndRead drives every configuration accessor from reader
// goroutines while UpdateSettings and SaveBotNodeConfig replace the in-memory
// configurations underneath them. Run with -race to check that the swap is
// race-free: before the singletons were published through atomic pointers this
// pattern was an unsynchronized read of a variable written by LoadGlobalConfig
// and friends, which the race detector reports.
//
// The readers call the accessors the way production code does — take the value
// and use it immediately, never store it — because that is what keeps a reader
// pinned to one complete version of the configuration.
func TestConcurrentReloadAndRead(t *testing.T) {
	writeTempConfig(t, &global.ConfigPath.Global, `{
    "system": {
        "debugMode": true,
        "listenPort": "8080"
    },
    "webhook": {
        "endpoints": {
            "example": { "enabled": true, "token": "t", "notifyPipes": ["ntfy"] }
        }
    },
    "controllerMessage": {
        "BOT_STARTED": "hello"
    }
}`)
	writeTempConfig(t, &global.ConfigPath.BotUserConfig, `{
    "qq(napcat)": {
        "admins": { "1": { "event_reply": true } },
        "trustedGroups": { "2": { "event_status_notify": true } }
    }
}`)
	writeTempConfig(t, &global.ConfigPath.BotNodeConfig, `{
    "node-1": { "enableStatusNotify": true }
}`)

	// Seed every singleton so the readers start from a loaded configuration
	// rather than racing the first store.
	if _, err := LoadGlobalConfig(global.ConfigPath.Global); err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	if _, err := LoadBotUserConfig(global.ConfigPath.BotUserConfig); err != nil {
		t.Fatalf("LoadBotUserConfig: %v", err)
	}
	if err := LoadBotNodeConfig(global.ConfigPath.BotNodeConfig); err != nil {
		t.Fatalf("LoadBotNodeConfig: %v", err)
	}

	const readers = 4
	const rounds = 40

	var readersWg, writersWg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < readers; i++ {
		readersWg.Add(1)
		go func() {
			defer readersWg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if cfg := Current(); cfg != nil {
					_ = cfg.System.DebugMode
					_ = cfg.System.ListenPort
					_ = cfg.ControllerMessage.BotStarted
					_ = cfg.Webhook.Endpoints
				}
				if users := BotUsers(); users != nil {
					_ = users.QQ.Admins.IDs()
					_ = users.QQ.TrustedGroups.IDs()
				}
				_ = BotNodes()
				_ = IsDebugMode()
				_ = NodeStatusNotifyEnabled("node-1")
				_ = WebhookEndpoints()
				_, _ = GetWebhookEndpoint("example")
			}
		}()
	}

	// Writer: reloads the global and bot user configurations, and rewrites the
	// node registry through UpdateSettings so its reload runs too.
	writersWg.Add(1)
	go func() {
		defer writersWg.Done()
		for i := 0; i < rounds; i++ {
			enabled := i%2 == 0
			patch := map[string]interface{}{
				"system": map[string]interface{}{"debugMode": enabled},
				"controllerMessage": map[string]interface{}{
					"BOT_STARTED": "hello",
				},
			}
			if err := UpdateSettings(SettingGlobal, patch); err != nil {
				t.Errorf("UpdateSettings(global): %v", err)
				return
			}
			if err := UpdateSettings(SettingBotUserConfig, map[string]interface{}{
				"qq(napcat)": map[string]interface{}{
					"admins": map[string]interface{}{
						"1": map[string]interface{}{"event_reply": enabled},
					},
				},
			}); err != nil {
				t.Errorf("UpdateSettings(bot_user_config): %v", err)
				return
			}
			if err := UpdateSettings(SettingBotNodeConfig, map[string]interface{}{
				"node-2": map[string]interface{}{"enableStatusNotify": enabled},
			}); err != nil {
				t.Errorf("UpdateSettings(bot_node_config): %v", err)
				return
			}
		}
	}()

	// Second writer: the node tracker's background save, which shares the same
	// read-modify-write lock as the admin edits above.
	writersWg.Add(1)
	go func() {
		defer writersWg.Done()
		for i := 0; i < rounds; i++ {
			if err := SaveBotNodeConfig(global.ConfigPath.BotNodeConfig, []string{"node-1", "node-2"}); err != nil {
				t.Errorf("SaveBotNodeConfig: %v", err)
				return
			}
		}
	}()

	// Let the writers finish, then release the readers. Waiting on the readers
	// first would deadlock: they only return once stop is closed.
	writersWg.Wait()
	close(stop)
	readersWg.Wait()

	// The last write must be visible: the accessors are not allowed to serve a
	// stale configuration once UpdateSettings has returned.
	if err := UpdateSettings(SettingGlobal, map[string]interface{}{
		"system": map[string]interface{}{"debugMode": true},
	}); err != nil {
		t.Fatalf("final UpdateSettings(global): %v", err)
	}
	cfg := Current()
	if cfg == nil {
		t.Fatal("Current() is nil after a successful reload")
	}
	if !cfg.System.DebugMode {
		t.Error("Current() did not observe the reloaded debugMode")
	}
	if cfg.System.ListenPort != "8080" {
		t.Errorf("reload dropped an untouched sibling: listenPort = %q", cfg.System.ListenPort)
	}
}
