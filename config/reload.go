package config

import (
	"fmt"
	"sync"

	"nukumizu-backend/postLog"
)

// reloadHooks are the callbacks run after the global configuration has been
// reloaded, i.e. after every settings update that touches config.json.
//
// They exist so that packages which already depend on config — the controller
// manager, the logger — can react to an update without config importing them,
// which would be an import cycle. Everything a hook needs is passed in.
var (
	reloadHooksMu sync.Mutex
	reloadHooks   []func(*Config)
)

// OnReload registers a hook to run after every reload of the global
// configuration, receiving the configuration now in effect. Hooks run in
// registration order.
//
// Register once, at startup, before the first settings update can arrive: a
// hook registered later has already missed the updates that came before it, and
// the configuration it would have seen is not replayed.
//
// A hook runs on the goroutine serving /api/settings/set, so it must not block
// for long. It may be called concurrently by two overlapping updates.
func OnReload(hook func(*Config)) {
	reloadHooksMu.Lock()
	defer reloadHooksMu.Unlock()
	reloadHooks = append(reloadHooks, hook)
}

// notifyReload runs every registered hook with cfg. A nil cfg is the signal
// that the update touched one of the other settings files, which have no
// hook-visible reload, and it is ignored.
//
// A panicking hook is logged and skipped rather than allowed to unwind through
// UpdateSettings: by the time hooks run the new configuration is already on
// disk and published in memory, so reporting the write as failed would be a
// lie, and the hooks registered after the broken one must still run.
func notifyReload(cfg *Config) {
	if cfg == nil {
		return
	}

	// Copy under the lock, then run outside it: a hook is free to register
	// another hook without deadlocking.
	reloadHooksMu.Lock()
	hooks := make([]func(*Config), len(reloadHooks))
	copy(hooks, reloadHooks)
	reloadHooksMu.Unlock()

	for _, hook := range hooks {
		runReloadHook(hook, cfg)
	}
}

// runReloadHook runs one hook, isolating a panic to that hook. Recovering in a
// separate function rather than inline is deliberate: a deferred recover placed
// in the loop body would not run until notifyReload itself returned, which
// would abandon the remaining hooks.
func runReloadHook(hook func(*Config), cfg *Config) {
	defer func() {
		if r := recover(); r != nil {
			postLog.Error(fmt.Sprintf("Configuration reload hook panicked: %v", r))
		}
	}()
	hook(cfg)
}
