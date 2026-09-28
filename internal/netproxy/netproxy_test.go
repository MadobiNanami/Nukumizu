package netproxy

import (
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"nukumizu-backend/config"
)

// publishConfig writes a config.json and makes it the configuration in effect,
// which is what a settings update does.
func publishConfig(t *testing.T, body string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	if _, err := config.LoadGlobalConfig(path); err != nil {
		t.Fatalf("LoadGlobalConfig: %v", err)
	}
}

// resolve runs a proxy function and returns the URL it chose, or "" when it
// chose a direct connection.
func resolve(t *testing.T, proxy func(*http.Request) (*url.URL, error)) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	u, err := proxy(req)
	if err != nil {
		t.Fatalf("proxy function: %v", err)
	}
	if u == nil {
		return ""
	}
	return u.String()
}

// TestProxyFuncResolvesPerCall is the property that makes system.networkProxy
// hot-reloadable: the function handed to a transport keeps reading the live
// configuration instead of the address that was configured when it was built.
func TestProxyFuncResolvesPerCall(t *testing.T) {
	publishConfig(t, `{"system":{"networkProxy":"http://127.0.0.1:7890"}}`)

	proxy := ProxyFunc(true)
	if proxy == nil {
		t.Fatal("ProxyFunc(true) returned nil, so the channel would never proxy")
	}
	if got := resolve(t, proxy); got != "http://127.0.0.1:7890" {
		t.Errorf("first resolution = %q", got)
	}

	// The same function must follow a settings update.
	publishConfig(t, `{"system":{"networkProxy":"http://127.0.0.1:8888"}}`)
	if got := resolve(t, proxy); got != "http://127.0.0.1:8888" {
		t.Errorf("after a settings update the same function resolved %q", got)
	}

	// Clearing the proxy falls back to a direct connection.
	publishConfig(t, `{"system":{"networkProxy":""}}`)
	if got := resolve(t, proxy); got != "" {
		t.Errorf("a cleared proxy still resolved %q", got)
	}
}

func TestProxyFuncOptOutReturnsNil(t *testing.T) {
	publishConfig(t, `{"system":{"networkProxy":"http://127.0.0.1:7890"}}`)

	// A channel with networkUseProxy off must not be handed a function at all,
	// so its transport keeps the default direct dialing.
	if ProxyFunc(false) != nil {
		t.Error("ProxyFunc(false) must return nil")
	}
}

func TestProxyFuncNormalizesMissingScheme(t *testing.T) {
	publishConfig(t, `{"system":{"networkProxy":"127.0.0.1:7890"}}`)

	if got := resolve(t, ProxyFunc(true)); got != "http://127.0.0.1:7890" {
		t.Errorf("resolved %q, want the http:// prefix added", got)
	}
}

func TestProxyFuncIgnoresUnusableProxy(t *testing.T) {
	// A value that cannot be parsed must leave the client dialing directly
	// rather than failing every request.
	publishConfig(t, `{"system":{"networkProxy":"://missing-scheme"}}`)

	if got := resolve(t, ProxyFunc(true)); got != "" {
		t.Errorf("an unparseable proxy resolved %q, want a direct connection", got)
	}
}

// TestDialWithTimeoutDialsDirectlyWithoutProxy covers the path a cleared
// system.networkProxy takes: the dialer was built while a proxy was configured,
// and must fall back to a direct dial once there is none.
func TestDialWithTimeoutDialsDirectlyWithoutProxy(t *testing.T) {
	publishConfig(t, `{"system":{"networkProxy":"http://127.0.0.1:7890"}}`)

	dial := DialWithTimeout(true)
	if dial == nil {
		t.Fatal("DialWithTimeout(true) returned nil")
	}

	// No proxy is listening on that address, so a dial attempted now would
	// fail; clearing the setting is what makes the direct path reachable.
	publishConfig(t, `{"system":{"networkProxy":""}}`)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()

	conn, err := dial("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial through a cleared proxy: %v", err)
	}
	conn.Close()
}
