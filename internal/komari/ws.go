package komari

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/postLog"

	"github.com/gorilla/websocket"
)

// rpcMethodLatestStatus asks Komari for the live status of every node.
// It returns a map keyed by node UUID; each entry carries an `online` flag and
// the flattened latest report fields.
const rpcMethodLatestStatus = "common:getNodesLatestStatus"

// jsonRpcResponse mirrors Komari's JSON-RPC 2.0 response envelope.
type jsonRpcResponse struct {
	Version string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// nodeLatestStatus is one entry returned by common:getNodesLatestStatus.
// Komari flattens the live report into these fields.
type nodeLatestStatus struct {
	Time           string  `json:"time"`
	CPU            float64 `json:"cpu"`
	RAM            int64   `json:"ram"`
	RAMTotal       int64   `json:"ram_total"`
	Swap           int64   `json:"swap"`
	SwapTotal      int64   `json:"swap_total"`
	Load           float64 `json:"load"`
	Load5          float64 `json:"load5"`
	Load15         float64 `json:"load15"`
	Disk           int64   `json:"disk"`
	DiskTotal      int64   `json:"disk_total"`
	NetIn          int64   `json:"net_in"`
	NetOut         int64   `json:"net_out"`
	NetTotalUp     int64   `json:"net_total_up"`
	NetTotalDown   int64   `json:"net_total_down"`
	Process        int     `json:"process"`
	Connections    int     `json:"connections"`
	ConnectionsUDP int     `json:"connections_udp"`
	Online         bool    `json:"online"`
	Uptime         int64   `json:"uptime"`
}

// WSClient manages a WebSocket connection to Komari for real-time status updates.
type WSClient struct {
	komariURL       string
	conn            *websocket.Conn
	connMu          sync.Mutex
	reconnectCount  atomic.Int32
	maxRetries      int
	stopCh          chan struct{}
	running         atomic.Bool
	onReconnectFail func() // Callback when all reconnection attempts fail.
}

// NewWSClient creates a new WebSocket client for Komari.
func NewWSClient(komariURL string, maxRetries int) *WSClient {
	return &WSClient{
		komariURL:  komariURL,
		maxRetries: maxRetries,
		stopCh:     make(chan struct{}),
	}
}

// SetOnReconnectFail sets the callback invoked when all reconnection attempts fail.
func (w *WSClient) SetOnReconnectFail(cb func()) {
	w.onReconnectFail = cb
}

// Connect establishes the WebSocket connection and starts the read loop.
func (w *WSClient) Connect() error {
	postLog.Info("Connecting to Komari WebSocket at " + w.komariURL)

	conn, err := w.dial()
	if err != nil {
		return err
	}

	w.connMu.Lock()
	w.conn = conn
	w.connMu.Unlock()

	w.reconnectCount.Store(0)
	w.running.Store(true)

	postLog.Info("Connected to Komari WebSocket")
	go w.readLoop()

	return nil
}

// dial opens the WebSocket connection to Komari's JSON-RPC endpoint (/api/rpc2).
// Komari's CheckWebSocketOrigin rejects handshakes that carry no Origin header
// (HTTP 403 → "bad handshake"), and without the session_token cookie the
// connection is treated as an anonymous guest, so both are sent.
func (w *WSClient) dial() (*websocket.Conn, error) {
	wsURL, origin, err := w.buildWSURL()
	if err != nil {
		return nil, err
	}

	header := http.Header{}
	header.Set("Origin", origin)
	if client := GetClient(); client != nil {
		if token := client.GetSessionToken(); token != "" {
			header.Set("Cookie", "session_token="+token)
		}
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, header)
	return conn, err
}

// buildWSURL converts the configured HTTP(S) base URL into the WebSocket URL for
// /api/rpc2 and derives the Origin header value that matches its host.
func (w *WSClient) buildWSURL() (wsURL, origin string, err error) {
	base := strings.TrimRight(w.komariURL, "/")
	if base == "" {
		return "", "", fmt.Errorf("empty Komari URL")
	}

	origin = base
	switch {
	case strings.HasPrefix(base, "http://"):
		wsURL = "ws://" + strings.TrimPrefix(base, "http://")
	case strings.HasPrefix(base, "https://"):
		wsURL = "wss://" + strings.TrimPrefix(base, "https://")
	default:
		return "", "", fmt.Errorf("unsupported Komari URL scheme: %s", base)
	}

	wsURL += "/api/rpc2"

	// Clean the Origin to "scheme://host" (no path/trailing slash).
	if u, perr := url.Parse(origin); perr == nil {
		origin = u.Scheme + "://" + u.Host
	}
	return wsURL, origin, nil
}

// requestStatus asks Komari for a fresh snapshot of all node statuses.
func (w *WSClient) requestStatus(conn *websocket.Conn) error {
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  rpcMethodLatestStatus,
		"params":  map[string]any{},
		"id":      1,
	})
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// readLoop repeatedly pulls a fresh status snapshot and feeds the node tracker.
// Komari never pushes unsolicited messages on /api/rpc2, so the loop issues one
// request per refresh interval and reads exactly one response.
//
// It must never continue reading on the same connection after a read deadline
// timeout: gorilla/websocket marks a connection as corrupt once a read times
// out and returns the stored error on every subsequent read, which would make
// the loop spin until gorilla panics. A timed-out read therefore triggers a
// reconnect instead.
func (w *WSClient) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			postLog.Error(fmt.Sprintf("WebSocket read loop panic recovered: %v", r))
		}
		w.running.Store(false)
	}()

	const (
		refreshInterval = 5 * time.Second
		readTimeout     = refreshInterval + 10*time.Second
	)

	for {
		select {
		case <-w.stopCh:
			return
		default:
		}

		w.connMu.Lock()
		conn := w.conn
		w.connMu.Unlock()

		if conn == nil {
			return
		}

		// Pull a fresh snapshot, then wait for its single response.
		if err := w.requestStatus(conn); err != nil {
			postLog.Warning("Failed to request node status refresh: " + err.Error())
			w.tryReconnect()
			return
		}

		if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
			postLog.Warning("Komari WebSocket set read deadline error: " + err.Error())
			w.tryReconnect()
			return
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				postLog.Warning("Komari WebSocket read timed out waiting for node status; reconnecting")
			} else {
				postLog.Warning("Komari WebSocket read error: " + err.Error())
			}
			w.tryReconnect()
			return
		}

		var rpcResp jsonRpcResponse
		if err := json.Unmarshal(data, &rpcResp); err != nil {
			postLog.Warning("Failed to parse Komari WebSocket message: " + err.Error())
		} else if rpcResp.Error != nil {
			postLog.Warning("Komari WebSocket RPC error: " + rpcResp.Error.Message)
		} else if len(rpcResp.Result) > 0 {
			online, reports := parseLatestStatus(rpcResp.Result)
			if tracker := node.GetTracker(); tracker != nil {
				tracker.UpdateStatus(online, reports)
			}
		}

		// Wait for the next refresh cycle before pulling again.
		select {
		case <-w.stopCh:
			return
		case <-time.After(refreshInterval):
		}
	}
}

// parseLatestStatus converts a common:getNodesLatestStatus result into the
// online UUID list and per-node report map consumed by the node tracker.
func parseLatestStatus(raw json.RawMessage) ([]string, map[string]node.Report) {
	var statuses map[string]nodeLatestStatus
	if err := json.Unmarshal(raw, &statuses); err != nil {
		postLog.Warning("Failed to parse node latest status: " + err.Error())
		return nil, map[string]node.Report{}
	}

	online := make([]string, 0, len(statuses))
	reports := make(map[string]node.Report, len(statuses))
	for uuid, s := range statuses {
		if s.Online {
			online = append(online, uuid)
		}

		var r node.Report
		r.CPU.Usage = s.CPU
		r.RAM.Total = s.RAMTotal
		r.RAM.Used = s.RAM
		r.Swap.Total = s.SwapTotal
		r.Swap.Used = s.Swap
		r.Load.Load1 = s.Load
		r.Load.Load5 = s.Load5
		r.Load.Load15 = s.Load15
		r.Disk.Total = s.DiskTotal
		r.Disk.Used = s.Disk
		r.Network.Up = s.NetOut
		r.Network.Down = s.NetIn
		r.Network.TotalUp = s.NetTotalUp
		r.Network.TotalDown = s.NetTotalDown
		r.Connections.TCP = s.Connections - s.ConnectionsUDP
		r.Connections.UDP = s.ConnectionsUDP
		r.Uptime = int(s.Uptime)
		r.Process = s.Process
		r.UpdatedAt = s.Time
		reports[uuid] = r
	}
	return online, reports
}

func (w *WSClient) tryReconnect() {
	select {
	case <-w.stopCh:
		return
	default:
	}

	count := w.reconnectCount.Add(1)
	if int(count) > w.maxRetries {
		postLog.Error(fmt.Sprintf("Komari WebSocket reconnection failed after %d attempts", w.maxRetries))
		if w.onReconnectFail != nil {
			w.onReconnectFail()
		}
		// Reset count and start a long-delay retry.
		w.reconnectCount.Store(0)
		time.Sleep(30 * time.Second)
		go w.tryReconnect()
		return
	}

	// Exponential backoff.
	delay := min(time.Duration(1<<(count-1))*time.Second, 30*time.Second)

	postLog.Info(fmt.Sprintf("Attempting Komari WebSocket reconnect %d/%d in %v...",
		count, w.maxRetries, delay))

	time.Sleep(delay)

	conn, err := w.dial()
	if err != nil {
		postLog.Warning(fmt.Sprintf("Komari WebSocket reconnect attempt %d failed: %v", count, err))
		go w.tryReconnect()
		return
	}

	w.connMu.Lock()
	if w.conn != nil {
		w.conn.Close()
	}
	w.conn = conn
	w.connMu.Unlock()

	w.reconnectCount.Store(0)

	// Re-fetch node list after reconnect.
	client := GetClient()
	if client != nil {
		nodes, err := client.FetchNodes()
		if err != nil {
			postLog.Error("Failed to refresh node list after reconnect: " + err.Error())
		} else {
			tracker := node.GetTracker()
			if tracker != nil {
				tracker.UpdateNodeList(BuildNodeListData(nodes))
			}
		}
	}

	postLog.Info("Komari WebSocket reconnected successfully")
	go w.readLoop()
}

// Stop closes the WebSocket connection and stops the read loop.
func (w *WSClient) Stop() {
	w.running.Store(false)
	close(w.stopCh)

	w.connMu.Lock()
	defer w.connMu.Unlock()
	if w.conn != nil {
		w.conn.Close()
		w.conn = nil
	}
}

// IsRunning returns whether the WebSocket client is actively running.
func (w *WSClient) IsRunning() bool {
	return w.running.Load()
}

// --- Global Komari client management ---

var globalClient *Client
var globalWSClient *WSClient
var globalClientMu sync.Mutex

// InitClient initializes the global Komari HTTP client.
func InitClient(komariURL string) {
	globalClientMu.Lock()
	defer globalClientMu.Unlock()
	globalClient = NewClient(komariURL)
}

// GetClient returns the global Komari HTTP client.
func GetClient() *Client {
	return globalClient
}

// InitWSClient initializes the global Komari WebSocket client.
func InitWSClient(komariURL string, maxRetries int) {
	globalClientMu.Lock()
	defer globalClientMu.Unlock()
	globalWSClient = NewWSClient(komariURL, maxRetries)
}

// GetWSClient returns the global Komari WebSocket client.
func GetWSClient() *WSClient {
	return globalWSClient
}

// LoginAndStart performs the Komari login and returns an error if it fails.
func LoginAndStart() error {
	cfg := config.GetGlobalConfig()
	client := GetClient()
	if client == nil {
		return fmt.Errorf("komari client not initialized")
	}

	if err := client.Login(cfg.Komari.Account.Username, cfg.Komari.Account.Password); err != nil {
		return fmt.Errorf("komari login failed: %w", err)
	}

	// Fetch initial node list.
	nodes, err := client.FetchNodes()
	if err != nil {
		return fmt.Errorf("failed to fetch nodes from komari: %w", err)
	}

	tracker := node.GetTracker()
	if tracker != nil {
		tracker.UpdateNodeList(BuildNodeListData(nodes))
	}

	return nil
}
