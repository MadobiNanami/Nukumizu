package komari

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/postLog"

	"github.com/gorilla/websocket"
)

// WSMessage is the structure of messages received from Komari WebSocket.
type WSMessage struct {
	Status string `json:"status"`
	Data   struct {
		Online []string                `json:"online"`
		Data   map[string]node.Report  `json:"data"`
	} `json:"data"`
}

// WSClient manages a WebSocket connection to Komari for real-time status updates.
type WSClient struct {
	komariURL      string
	conn           *websocket.Conn
	connMu         sync.Mutex
	reconnectCount atomic.Int32
	maxRetries     int
	stopCh         chan struct{}
	running        atomic.Bool
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
		postLog.Error("Failed to connect to Komari WebSocket: " + err.Error())
		return err
	}

	w.connMu.Lock()
	w.conn = conn
	w.connMu.Unlock()

	w.reconnectCount.Store(0)
	w.running.Store(true)

	// Request initial state.
	if err := conn.WriteMessage(websocket.TextMessage, []byte("get")); err != nil {
		postLog.Error("Failed to send 'get' message: " + err.Error())
		return err
	}

	postLog.Info("Connected to Komari WebSocket")
	go w.readLoop()

	return nil
}

func (w *WSClient) dial() (*websocket.Conn, error) {
	// Build the WebSocket URL from the Komari HTTP URL.
	wsURL := w.komariURL
	if len(wsURL) > 7 && wsURL[:7] == "http://" {
		wsURL = "ws://" + wsURL[7:]
	} else if len(wsURL) > 8 && wsURL[:8] == "https://" {
		wsURL = "wss://" + wsURL[8:]
	}
	wsURL = wsURL + "/api/clients"

	header := http.Header{}
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(wsURL, header)
	return conn, err
}

func (w *WSClient) readLoop() {
	defer func() {
		if r := recover(); r != nil {
			postLog.Error(fmt.Sprintf("WebSocket read loop panic recovered: %v", r))
		}
		w.running.Store(false)
	}()

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

		_, data, err := conn.ReadMessage()
		if err != nil {
			postLog.Warning("Komari WebSocket read error: " + err.Error())
			w.tryReconnect()
			return
		}

		var msg WSMessage
		if err := json.Unmarshal(data, &msg); err != nil {
			postLog.Warning("Failed to parse Komari WebSocket message: " + err.Error())
			continue
		}

		if msg.Status != "success" {
			postLog.Warning("Komari WebSocket message with non-success status")
			continue
		}

		// Update the node tracker with the received data.
		tracker := node.GetTracker()
		if tracker != nil {
			tracker.UpdateStatus(msg.Data.Online, msg.Data.Data)
		}
	}
}

func (w *WSClient) tryReconnect() {
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
	delay := time.Duration(1<<(count-1)) * time.Second
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}

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

	// Request full state after reconnect.
	if err := conn.WriteMessage(websocket.TextMessage, []byte("get")); err != nil {
		postLog.Error("Failed to send 'get' after reconnect: " + err.Error())
		return
	}

	// Re-fetch node list after reconnect.
	client := GetClient()
	if client != nil {
		nodes, err := client.FetchNodes()
		if err != nil {
			postLog.Error("Failed to refresh node list after reconnect: " + err.Error())
		} else {
			tracker := node.GetTracker()
			if tracker != nil {
				nodeNames := make(map[string]string, len(nodes))
				for _, n := range nodes {
					nodeNames[n.UUID] = n.Name
				}
				tracker.UpdateNodeList(nodeNames)
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
	cfg := config.GetConfig()
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
		nodeNames := make(map[string]string, len(nodes))
		for _, n := range nodes {
			nodeNames[n.UUID] = n.Name
		}
		tracker.UpdateNodeList(nodeNames)
	}

	return nil
}
