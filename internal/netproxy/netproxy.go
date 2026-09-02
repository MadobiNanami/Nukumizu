// Package netproxy builds network clients that route traffic through the HTTP
// network proxy configured in the system config. Each caller decides whether
// to use the proxy by passing its own useProxy flag (the per-channel
// networkUseProxy setting), so proxying is opt-in per channel.
package netproxy

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"nukumizu-backend/config"
)

// proxyURL returns the system-wide network proxy URL, or nil when none is
// configured. A missing scheme is normalized to http:// for convenience.
func proxyURL() *url.URL {
	raw := config.C_globalConfig.System.NetworkProxy
	if raw == "" {
		return nil
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	return u
}

// ProxyFunc returns a transport proxy function that routes requests through
// the configured network proxy when enabled. It returns nil when the caller
// opts out or no proxy is configured, meaning direct connection. The returned
// function is compatible with both http.Transport.Proxy and
// websocket.Dialer.Proxy.
func ProxyFunc(useProxy bool) func(*http.Request) (*url.URL, error) {
	if !useProxy {
		return nil
	}
	u := proxyURL()
	if u == nil {
		return nil
	}
	return http.ProxyURL(u)
}

// HTTPClient builds an http.Client that sends traffic through the configured
// network proxy when enabled, falling back to a direct connection otherwise.
// The default transport's timeouts and connection pooling are preserved.
func HTTPClient(useProxy bool, timeout time.Duration) *http.Client {
	client := &http.Client{Timeout: timeout}
	if p := ProxyFunc(useProxy); p != nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = p
		client.Transport = transport
	}
	return client
}

// DialWithTimeout returns a dial function that connects directly, or tunnels
// through the configured HTTP CONNECT proxy when enabled. Its signature
// matches net.DialTimeout so it can be plugged into gomail's NetDialTimeout
// to send SMTP over the proxy.
func DialWithTimeout(useProxy bool) func(network, addr string, timeout time.Duration) (net.Conn, error) {
	u := proxyURL()
	return func(network, addr string, timeout time.Duration) (net.Conn, error) {
		if !useProxy || u == nil {
			return net.DialTimeout(network, addr, timeout)
		}
		return dialViaProxy(u, addr, timeout)
	}
}

// bufferedConn reads any bytes buffered while parsing the CONNECT response
// before falling back to the underlying connection. Without this, tunneled
// bytes (e.g. an SMTP greeting) that arrived in the same read as the proxy
// response headers would be lost.
type bufferedConn struct {
	net.Conn
	r *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if c.r != nil {
		n, err := c.r.Read(p)
		if n > 0 {
			return n, err
		}
		if err != nil {
			return 0, err
		}
		// Buffer exhausted; read directly from the tunnel from now on.
		c.r = nil
	}
	return c.Conn.Read(p)
}

// dialViaProxy opens a TCP connection to the proxy and issues an HTTP CONNECT
// request to establish a tunnel to the target address. The returned conn is a
// raw bidirectional tunnel to target.
func dialViaProxy(proxy *url.URL, target string, timeout time.Duration) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", proxy.Host, timeout)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy %s: %w", proxy.Host, err)
	}
	if timeout > 0 {
		conn.SetDeadline(time.Now().Add(timeout))
	}

	// Build the CONNECT request manually; http.Request.Write omits the
	// CONNECT authority when the URL has a non-empty Path, so Opaque is used.
	req := &http.Request{
		Method: http.MethodConnect,
		URL:    &url.URL{Opaque: target},
		Host:   target,
		Header: make(http.Header),
	}
	if proxy.User != nil {
		pw, _ := proxy.User.Password()
		creds := proxy.User.Username() + ":" + pw
		req.Header.Set("Proxy-Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(creds)))
	}
	if err := req.Write(conn); err != nil {
		conn.Close()
		return nil, fmt.Errorf("write CONNECT request: %w", err)
	}

	// Read the status line, then headers up to the blank line. The response has
	// no body, so this must be parsed manually rather than with
	// http.ReadResponse, which would treat tunneled bytes as a response body.
	br := bufio.NewReader(conn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("read proxy response: %w", err)
	}
	parts := strings.SplitN(strings.TrimSpace(statusLine), " ", 3)
	if len(parts) < 2 || !strings.HasPrefix(parts[0], "HTTP/") {
		conn.Close()
		return nil, fmt.Errorf("invalid proxy response: %q", strings.TrimSpace(statusLine))
	}
	var statusCode int
	if _, err := fmt.Sscanf(parts[1], "%d", &statusCode); err != nil {
		conn.Close()
		return nil, fmt.Errorf("invalid proxy status code: %q", parts[1])
	}
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("read proxy response headers: %w", err)
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	conn.SetDeadline(time.Time{})
	if statusCode != http.StatusOK {
		conn.Close()
		return nil, fmt.Errorf("proxy CONNECT to %s failed: %s", target, strings.TrimSpace(statusLine))
	}
	return &bufferedConn{Conn: conn, r: br}, nil
}
