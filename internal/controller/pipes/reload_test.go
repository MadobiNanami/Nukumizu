package pipes

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"

	gomail "gopkg.in/mail.v2"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
)

// writeConfigFile writes one config.json and returns its path. Reload tests
// need several of these up front, because publishing a configuration from
// inside a goroutine cannot use t.Fatalf.
func writeConfigFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	return path
}

// publishConfig writes body and makes it the configuration in effect, which is
// what a settings update does before the reload hooks run.
func publishConfig(t *testing.T, body string) *config.Config {
	t.Helper()
	cfg, err := config.LoadGlobalConfig(writeConfigFile(t, body))
	if err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	return cfg
}

func TestNtfyControllerReload(t *testing.T) {
	first := publishConfig(t, `{
    "controllerMethod": {
        "ntfy": { "enabled": false, "markdown": false, "server": "https://ntfy.sh", "topic": "old" }
    }
}`)
	ctrl := NewNtfyController(first.ControllerMethod.Ntfy)

	if ctrl.IsEnabled() {
		t.Fatal("controller should start disabled")
	}

	publishConfig(t, `{
    "controllerMethod": {
        "ntfy": { "enabled": true, "markdown": true, "server": "https://ntfy.example.com", "topic": "new" }
    }
}`)
	ctrl.Reload()

	if !ctrl.IsEnabled() {
		t.Error("Reload did not pick up enabled")
	}
	if !ctrl.IsMarkdown() {
		t.Error("Reload did not pick up markdown")
	}
	s := ctrl.settings()
	if s.Topic != "new" || s.Server != "https://ntfy.example.com" {
		t.Errorf("Reload did not pick up the new server/topic: %+v", s)
	}
}

// TestReloadKeepsDerivedClientsWhenNothingRelevantChanged pins the guard: a
// reload that does not touch the proxy flag must not throw away a perfectly
// good HTTP client.
func TestReloadKeepsDerivedClientsWhenNothingRelevantChanged(t *testing.T) {
	first := publishConfig(t, `{
    "controllerMethod": {
        "webhook": { "enabled": true, "url": "https://example.com/one", "networkUseProxy": false },
        "ntfy":    { "enabled": true, "topic": "t", "networkUseProxy": false }
    }
}`)

	webhook := NewWebhookController(first.ControllerMethod.Webhook)
	ntfy := NewNtfyController(first.ControllerMethod.Ntfy)
	webhookClient, ntfyClient := webhook.client(), ntfy.client()

	// The whole point of an unchanged reload: the settings object is equal, so
	// nothing is rebuilt.
	publishConfig(t, `{
    "controllerMethod": {
        "webhook": { "enabled": true, "url": "https://example.com/one", "networkUseProxy": false },
        "ntfy":    { "enabled": true, "topic": "t", "networkUseProxy": false }
    }
}`)
	webhook.Reload()
	ntfy.Reload()

	if webhook.client() != webhookClient {
		t.Error("webhook client was rebuilt although its settings did not change")
	}
	if ntfy.client() != ntfyClient {
		t.Error("ntfy client was rebuilt although its settings did not change")
	}

	// A change that leaves the proxy flag alone still keeps the client.
	publishConfig(t, `{
    "controllerMethod": {
        "webhook": { "enabled": true, "url": "https://example.com/two", "networkUseProxy": false },
        "ntfy":    { "enabled": true, "topic": "t", "networkUseProxy": false }
    }
}`)
	webhook.Reload()

	if webhook.client() != webhookClient {
		t.Error("webhook client was rebuilt for a change that did not touch the proxy setting")
	}
	if got := webhook.settings().URL; got != "https://example.com/two" {
		t.Errorf("Reload did not pick up the new URL: %q", got)
	}

	// Flipping the proxy flag is the one change that must rebuild it.
	publishConfig(t, `{
    "controllerMethod": {
        "webhook": { "enabled": true, "url": "https://example.com/two", "networkUseProxy": true },
        "ntfy":    { "enabled": true, "topic": "t", "networkUseProxy": false }
    }
}`)
	webhook.Reload()

	if webhook.client() == webhookClient {
		t.Error("webhook client was not rebuilt after the proxy setting changed")
	}
}

// dialerIsDefault reports whether gomail still dials directly. Function values
// are not comparable in Go, so the code pointers are compared.
func dialerIsDefault() bool {
	return reflect.ValueOf(gomail.NetDialTimeout).Pointer() ==
		reflect.ValueOf(net.DialTimeout).Pointer()
}

// TestEmailControllerReloadResetsDialer covers the case the constructor used to
// get wrong: gomail's dial hook is a package-level variable with no "unset", so
// turning networkUseProxy off has to actively restore the default dialer rather
// than leave SMTP tunnelled through a proxy nobody asked for.
func TestEmailControllerReloadResetsDialer(t *testing.T) {
	t.Cleanup(func() { gomail.NetDialTimeout = net.DialTimeout })

	first := publishConfig(t, `{
    "system": { "networkProxy": "http://127.0.0.1:7890" },
    "controllerMethod": {
        "email": { "enabled": true, "networkUseProxy": true, "smtpHost": "smtp.example.com", "to": ["a@example.com"] }
    }
}`)
	ctrl := NewEmailController(first.ControllerMethod.Email)

	if dialerIsDefault() {
		t.Fatal("the proxy dialer was not installed for networkUseProxy: true")
	}

	publishConfig(t, `{
    "system": { "networkProxy": "http://127.0.0.1:7890" },
    "controllerMethod": {
        "email": { "enabled": true, "networkUseProxy": false, "smtpHost": "smtp.example.com", "to": ["a@example.com"] }
    }
}`)
	ctrl.Reload()

	if !dialerIsDefault() {
		t.Error("turning networkUseProxy off left gomail's proxy dialer in place")
	}
}

// TestWebhookReloadDuringSend drives Reload while notifications are being
// delivered. Run with -race: the settings and the HTTP client are swapped on
// the settings-update goroutine while the send path reads them on others, which
// is exactly the race the atomic pointers exist to prevent.
func TestWebhookReloadDuringSend(t *testing.T) {
	var delivered int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&delivered, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	body := func(proxy bool) string {
		return fmt.Sprintf(`{
    "controllerMethod": {
        "webhook": { "enabled": true, "url": %q, "method": "POST", "networkUseProxy": %t }
    },
    "controllerMessage": { "SERVER_STATUS_CHANGED": "{{ serverName }} {{ event }}" }
}`, srv.URL, proxy)
	}
	direct := writeConfigFile(t, body(false))
	proxied := writeConfigFile(t, body(true))

	if _, err := config.LoadGlobalConfig(direct); err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
	ctrl := NewWebhookController(config.Current().ControllerMethod.Webhook)

	change := node.StatusChange{Event: "Online", UUID: "u1", Name: "alpha"}

	var readers, writers sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 3; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := ctrl.SendStatusChange(change); err != nil {
					// The test server never fails; a transport error here means
					// the swap lost a field mid-flight.
					t.Errorf("SendStatusChange: %v", err)
					return
				}
				_ = ctrl.IsEnabled()
				_ = ctrl.IsMarkdown()
			}
		}()
	}

	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 30; i++ {
			path := direct
			if i%2 == 0 {
				path = proxied
			}
			if _, err := config.LoadGlobalConfig(path); err != nil {
				t.Errorf("LoadGlobalConfig: %v", err)
				return
			}
			ctrl.Reload()
		}
	}()

	// Let the writer finish, then release the readers: they only return once
	// stop is closed.
	writers.Wait()
	close(stop)
	readers.Wait()

	if atomic.LoadInt64(&delivered) == 0 {
		t.Error("no notification reached the test server")
	}
}
