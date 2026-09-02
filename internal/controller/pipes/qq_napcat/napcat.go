package qq_napcat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/postLog"
)

// APIResponse mirrors NapCat's HTTP API response envelope.
type APIResponse struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// Client is the HTTP + WebSocket client for a single NapCat instance.
// It both listens for incoming OneBot events over WebSocket and issues
// outbound NapCat API calls over HTTP.
type Client struct {
	addr       string
	port       string
	token      string
	useProxy   bool
	httpClient *http.Client

	connMu   sync.Mutex
	conn     *websocket.Conn
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewClient creates a NapCat client for the given host/port/token. When
// useProxy is set, HTTP API calls and the WebSocket connection are routed
// through the system-wide network proxy.
func NewClient(addr, port, token string, useProxy bool) *Client {
	return &Client{
		addr:       addr,
		port:       port,
		token:      token,
		useProxy:   useProxy,
		httpClient: netproxy.HTTPClient(useProxy, 30*time.Second),
		stopCh:     make(chan struct{}),
	}
}

func (c *Client) baseURL() string {
	return fmt.Sprintf("http://%s:%s", c.addr, c.port)
}

// sendRequest performs an HTTP call to NapCat, injecting access_token into POST
// bodies AND setting Authorization: Bearer (compatible with HTTP Server adapters
// created in the NapCat Web UI).
func (c *Client) sendRequest(method, endpoint string, body []byte) ([]byte, error) {
	url := c.baseURL() + endpoint

	// For POST requests, inject access_token into the JSON body before sending.
	var finalBody []byte
	if body != nil && c.token != "" {
		var bodyMap map[string]interface{}
		if err := json.Unmarshal(body, &bodyMap); err == nil {
			bodyMap["access_token"] = c.token
			finalBody, _ = json.Marshal(bodyMap)
		}
	}
	if finalBody == nil {
		finalBody = body
	}

	var req *http.Request
	var err error
	if finalBody != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(finalBody))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to napcat: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read napcat response: %w", err)
	}

	return respBody, nil
}

// parseResponse unmarshals NapCat's raw response body into an APIResponse.
func (c *Client) parseResponse(respBody []byte, caller string) (*APIResponse, error) {
	var napcatResp APIResponse
	if err := json.Unmarshal(respBody, &napcatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal napcat response (%s): %w\n%s", caller, err, string(respBody))
	}
	return &napcatResp, nil
}

// SendMsg sends a message via NapCat.
// targetType: "group" or "private"
// targetID: QQ group ID or user ID
// msg: message content
// hasAt: whether to prepend an @mention
// atTargetID: the QQ ID to @mention
func (c *Client) SendMsg(targetType string, targetID int64, msg string, hasAt bool, atTargetID int64) (*APIResponse, error) {
	// Build message with optional @mention.
	message := msg
	if hasAt && atTargetID > 0 {
		message = fmt.Sprintf("[CQ:at,qq=%d] %s", atTargetID, msg)
	}

	var endpoint string
	var napcatReq map[string]interface{}

	switch targetType {
	case "group":
		endpoint = "/send_group_msg"
		napcatReq = map[string]interface{}{
			"group_id": targetID,
			"message":  message,
		}
	case "private":
		endpoint = "/send_private_msg"
		napcatReq = map[string]interface{}{
			"user_id": targetID,
			"message": message,
		}
	default:
		return nil, fmt.Errorf("invalid targetType: %s, must be 'group' or 'private'", targetType)
	}

	reqBody, err := json.Marshal(napcatReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if config.C_globalConfig.Debug.ShowNapcatAction {
		postLog.Debug(fmt.Sprintf("[Napcat] SendMsg -> %s (%s): %s", endpoint, targetType, message))
	}

	respBody, err := c.sendRequest(http.MethodPost, endpoint, reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseResponse(respBody, "SendMsg")
}

// RecallMsg recalls (deletes) a sent message via NapCat.
func (c *Client) RecallMsg(msgID int64) (*APIResponse, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"message_id": msgID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if config.C_globalConfig.Debug.ShowNapcatAction {
		postLog.Debug(fmt.Sprintf("[Napcat] RecallMsg -> /delete_msg: %d", msgID))
	}

	respBody, err := c.sendRequest(http.MethodPost, "/delete_msg", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseResponse(respBody, "RecallMsg")
}

// GetGroupList retrieves the list of joined groups from NapCat.
func (c *Client) GetGroupList() (*APIResponse, error) {
	if config.C_globalConfig.Debug.ShowNapcatAction {
		postLog.Debug("[Napcat] GetGroupList -> /get_group_list")
	}

	respBody, err := c.sendRequest(http.MethodGet, "/get_group_list", nil)
	if err != nil {
		return nil, err
	}

	return c.parseResponse(respBody, "GetGroupList")
}

// GetGroupInfo retrieves detailed information about a specific group from NapCat.
func (c *Client) GetGroupInfo(groupID int64) (*APIResponse, error) {
	reqBody, err := json.Marshal(map[string]interface{}{
		"group_id": groupID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if config.C_globalConfig.Debug.ShowNapcatAction {
		postLog.Debug(fmt.Sprintf("[Napcat] GetGroupInfo -> /get_group_info: %d", groupID))
	}

	respBody, err := c.sendRequest(http.MethodPost, "/get_group_info", reqBody)
	if err != nil {
		return nil, err
	}

	return c.parseResponse(respBody, "GetGroupInfo")
}

// GetFriendsList retrieves the friends list from NapCat.
func (c *Client) GetFriendsList() (*APIResponse, error) {
	if config.C_globalConfig.Debug.ShowNapcatAction {
		postLog.Debug("[Napcat] GetFriendsList -> /get_friend_list")
	}

	respBody, err := c.sendRequest(http.MethodGet, "/get_friend_list", nil)
	if err != nil {
		return nil, err
	}

	return c.parseResponse(respBody, "GetFriendsList")
}

// Listen connects to the NapCat WebSocket server and calls onEvent for every raw
// message. It blocks forever, reconnecting every 5s after a disconnect, until
// Stop is called. Run it in a goroutine.
func (c *Client) Listen(onEvent func(raw []byte)) {
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		c.listenOnce(onEvent)

		postLog.Warning("[Napcat] WebSocket disconnected, reconnecting in 5s...")
		select {
		case <-c.stopCh:
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// listenOnce connects to the NapCat WebSocket and reads events until the
// connection drops or Stop is called.
func (c *Client) listenOnce(onEvent func(raw []byte)) {
	wsURL := fmt.Sprintf("ws://%s:%s/", c.addr, c.port)
	header := http.Header{}
	if c.token != "" {
		wsURL += "?access_token=" + url.QueryEscape(c.token)
		header.Set("Authorization", "Bearer "+c.token)
	}

	dialer := *websocket.DefaultDialer
	dialer.Proxy = netproxy.ProxyFunc(c.useProxy)
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		postLog.Error(fmt.Sprintf("[Napcat] Failed to connect to NapCat WebSocket: %v", err))
		return
	}

	c.connMu.Lock()
	c.conn = conn
	c.connMu.Unlock()
	defer func() {
		c.connMu.Lock()
		if c.conn == conn {
			c.conn = nil
		}
		c.connMu.Unlock()
		conn.Close()
	}()

	postLog.Info(fmt.Sprintf("[Napcat] Connected to NapCat WebSocket at %s", wsURL))

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			postLog.Error(fmt.Sprintf("[Napcat] WebSocket read error: %v", err))
			return
		}

		onEvent(msgBytes)
	}
}

// Stop closes any open WebSocket connection and unblocks the Listen reconnect loop.
func (c *Client) Stop() {
	c.stopOnce.Do(func() { close(c.stopCh) })

	c.connMu.Lock()
	defer c.connMu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
}
