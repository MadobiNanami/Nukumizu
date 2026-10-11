package qq_onebot

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"

	"nukumizu-backend/config"
	"nukumizu-backend/internal/netproxy"
	"nukumizu-backend/postLog"
)

const (
	// onebotReconnectDelay is the pause between reconnect attempts, shared by the
	// event socket and the API socket.
	onebotReconnectDelay = 5 * time.Second
	// onebotAPITimeout bounds one callAPI response wait. It matches the timeout the
	// removed HTTP client (netproxy.HTTPClient(useProxy, 30s)) imposed, so callers
	// see the same worst-case latency they always did.
	onebotAPITimeout = 30 * time.Second
	// onebotConnectWait bounds how long callAPI waits for the API socket to come up
	// when it is not connected yet. It is deliberately short: the startup bot
	// initialization message is sent on the main goroutine, and a down OneBot
	// server must not stall HTTP-server startup for the full API timeout.
	onebotConnectWait = 5 * time.Second
	// onebotWriteTimeout bounds a single socket write so a wedged peer cannot block
	// the caller (and, through writeMu, every other writer) indefinitely.
	onebotWriteTimeout = 10 * time.Second
)

// actionLogEnabled reports whether the OneBot API methods echo each action they
// send, per the showOneBotAction toggle. Unlike the WebSocket message path in
// qq.go it deliberately does not also require debugMode: these methods have
// always logged on this toggle alone, and preserving that is intentional.
func actionLogEnabled() bool {
	cfg := config.Current()
	return cfg != nil && cfg.Debug.ShowOneBotAction
}

// APIResponse mirrors OneBot's API response envelope. Echo is the correlation id
// the client sent, echoed back by the server; callAPI uses it internally to match
// a response to its request. Data stays raw so callers decode get_group_list /
// get_friend_list arrays themselves.
type APIResponse struct {
	Status  string          `json:"status"`
	RetCode int             `json:"retcode"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
	Echo    json.RawMessage `json:"echo"`
}

// apiRequest is one OneBot API call written to the API socket. A struct rather
// than a map guarantees Echo is encoded as a JSON string, which is what the
// server sends back verbatim.
type apiRequest struct {
	Action string                 `json:"action"`
	Params map[string]interface{} `json:"params"`
	Echo   string                 `json:"echo"`
}

// apiResult carries one decoded API response (or a transport error) from the API
// read loop back to the callAPI invocation waiting on its echo.
type apiResult struct {
	resp *APIResponse
	err  error
}

// Client is the WebSocket client for a single OneBot instance. It uses two
// forward-WebSocket connections, per the OneBot 11 split between events and API
// calls:
//
//   - the event socket ("/") only reads: one goroutine feeds every frame to
//     onEvent. Nothing is ever written to it.
//   - the API socket ("/api") only issues actions: callAPI writes a
//     {"action","params","echo"} frame there, and its dedicated read loop matches
//     the reply by echo.
//
// They must be separate: qq.go's event handler replies with SendMsg from inside
// the event read loop, so reading API responses on that same socket would
// deadlock (the loop would be blocked in the handler, waiting on a response only
// it could deliver).
type Client struct {
	addr     string
	port     string
	token    string
	useProxy bool

	// Event socket ("/"): one reader (listenOnce), closed only by Stop or its own
	// cleanup, never written to.
	connMu sync.Mutex
	conn   *websocket.Conn

	// API socket ("/api"). apiMu guards the pointer, the epoch channel and the
	// pending table; writeMu serializes the actual socket writes, so apiMu is
	// never held across I/O (which would stall response dispatch behind a slow
	// write).
	apiMu      sync.Mutex
	apiConn    *websocket.Conn
	apiChanged chan struct{} // closed and replaced on every connect/disconnect
	writeMu    sync.Mutex
	pending    map[string]chan apiResult
	echoSeq    atomic.Uint64
	stopped    bool

	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewClient creates a OneBot client for the given host/port/token. When useProxy
// is set, both WebSocket connections are routed through the system-wide network
// proxy. It performs no I/O; Listen starts the sockets.
func NewClient(addr, port, token string, useProxy bool) *Client {
	return &Client{
		addr:       addr,
		port:       port,
		token:      token,
		useProxy:   useProxy,
		apiChanged: make(chan struct{}),
		pending:    make(map[string]chan apiResult),
		stopCh:     make(chan struct{}),
	}
}

// dialWS opens one forward-WebSocket connection to the given path. The token is
// passed both as an access_token query parameter and as an Authorization: Bearer
// header (forward adapters check one or the other), and the dial goes through the
// system proxy when useProxy is set.
func (c *Client) dialWS(path string) (*websocket.Conn, error) {
	wsURL := fmt.Sprintf("ws://%s:%s%s", c.addr, c.port, path)
	header := http.Header{}
	if c.token != "" {
		wsURL += "?access_token=" + url.QueryEscape(c.token)
		header.Set("Authorization", "Bearer "+c.token)
	}

	dialer := *websocket.DefaultDialer
	dialer.Proxy = netproxy.ProxyFunc(c.useProxy)
	conn, _, err := dialer.Dial(wsURL, header)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to onebot websocket %s: %w", path, err)
	}
	return conn, nil
}

// SendMsg sends a message via OneBot.
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

	var action string
	var params map[string]interface{}

	switch targetType {
	case "group":
		action = "send_group_msg"
		params = map[string]interface{}{
			"group_id": targetID,
			"message":  message,
		}
	case "private":
		action = "send_private_msg"
		params = map[string]interface{}{
			"user_id": targetID,
			"message": message,
		}
	default:
		return nil, fmt.Errorf("invalid targetType: %s, must be 'group' or 'private'", targetType)
	}

	if actionLogEnabled() {
		postLog.Debug(fmt.Sprintf("[qq_onebot] SendMsg -> %s (%s): %s", action, targetType, message))
	}

	return c.callAPI(action, params)
}

// RecallMsg recalls (deletes) a sent message via OneBot.
func (c *Client) RecallMsg(msgID int64) (*APIResponse, error) {
	if actionLogEnabled() {
		postLog.Debug(fmt.Sprintf("[qq_onebot] RecallMsg -> delete_msg: %d", msgID))
	}

	return c.callAPI("delete_msg", map[string]interface{}{"message_id": msgID})
}

// GetGroupList retrieves the list of joined groups from OneBot.
func (c *Client) GetGroupList() (*APIResponse, error) {
	if actionLogEnabled() {
		postLog.Debug("[qq_onebot] GetGroupList -> get_group_list")
	}

	return c.callAPI("get_group_list", nil)
}

// GetGroupInfo retrieves detailed information about a specific group from OneBot.
func (c *Client) GetGroupInfo(groupID int64) (*APIResponse, error) {
	if actionLogEnabled() {
		postLog.Debug(fmt.Sprintf("[qq_onebot] GetGroupInfo -> get_group_info: %d", groupID))
	}

	return c.callAPI("get_group_info", map[string]interface{}{"group_id": groupID})
}

// GetFriendsList retrieves the friends list from OneBot.
func (c *Client) GetFriendsList() (*APIResponse, error) {
	if actionLogEnabled() {
		postLog.Debug("[qq_onebot] GetFriendsList -> get_friend_list")
	}

	return c.callAPI("get_friend_list", nil)
}

// callAPI issues one OneBot action over the API socket and waits for its reply.
//
// It returns a Go error only for transport failures (not connected, write error,
// disconnect, timeout, Stop). An OneBot-level failure (status "failed", retcode
// 1xxx) is returned as an ordinary (*APIResponse, nil), matching the behavior of
// the HTTP client this replaces.
func (c *Client) callAPI(action string, params map[string]interface{}) (*APIResponse, error) {
	// The spec allows omitting params entirely, but some adapters reject a null
	// value, so a bodyless action still sends an empty object.
	if params == nil {
		params = map[string]interface{}{}
	}
	echo := strconv.FormatUint(c.echoSeq.Add(1), 10)

	payload, err := json.Marshal(apiRequest{Action: action, Params: params, Echo: echo})
	if err != nil {
		return nil, fmt.Errorf("failed to marshal onebot request: %w", err)
	}

	// Buffered cap 1: the API read loop and failPending can each deliver at most
	// once, and delivering must never block the read loop.
	ch := make(chan apiResult, 1)

	c.apiMu.Lock()
	if c.stopped {
		c.apiMu.Unlock()
		return nil, fmt.Errorf("onebot client stopped")
	}
	c.pending[echo] = ch
	c.apiMu.Unlock()

	// Deregister on every exit path. dispatchAPI deletes the entry too, so this is
	// a no-op when the response already arrived.
	defer func() {
		c.apiMu.Lock()
		delete(c.pending, echo)
		c.apiMu.Unlock()
	}()

	conn, err := c.waitForAPIConn()
	if err != nil {
		return nil, err
	}

	if err := c.writeAPI(conn, payload); err != nil {
		return nil, fmt.Errorf("failed to send onebot request: %w", err)
	}

	timer := time.NewTimer(onebotAPITimeout)
	defer timer.Stop()

	select {
	case res := <-ch:
		return res.resp, res.err
	case <-timer.C:
		return nil, fmt.Errorf("onebot API %s timed out after %s", action, onebotAPITimeout)
	case <-c.stopCh:
		return nil, fmt.Errorf("onebot client stopped")
	}
}

// waitForAPIConn returns the live API connection, waiting (bounded by
// onebotConnectWait, or until Stop) for apiLoop to establish one when it is not
// connected yet. It re-checks the state on every apiChanged transition, so a
// closed channel left over from a previous connection cannot fool it.
func (c *Client) waitForAPIConn() (*websocket.Conn, error) {
	timer := time.NewTimer(onebotConnectWait)
	defer timer.Stop()

	for {
		c.apiMu.Lock()
		conn := c.apiConn
		changed := c.apiChanged
		stopped := c.stopped
		c.apiMu.Unlock()

		if conn != nil {
			return conn, nil
		}
		if stopped {
			return nil, fmt.Errorf("onebot client stopped")
		}

		select {
		case <-changed:
			// Connection state changed; re-check.
		case <-timer.C:
			return nil, fmt.Errorf("onebot API WebSocket not connected")
		case <-c.stopCh:
			return nil, fmt.Errorf("onebot client stopped")
		}
	}
}

// writeAPI serializes one request frame onto conn under writeMu, with a write
// deadline. gorilla/websocket panics on concurrent writes, and both the event
// handler and every SendMessage path call callAPI, so concurrency is the norm.
func (c *Client) writeAPI(conn *websocket.Conn, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	if err := conn.SetWriteDeadline(time.Now().Add(onebotWriteTimeout)); err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, payload)
}

// Listen connects the event WebSocket and calls onEvent for every raw message. It
// starts the API socket loop as well, since NewClient is side-effect-free and
// Listen is the single entry point the controller drives. It blocks forever,
// reconnecting every onebotReconnectDelay after a disconnect, until Stop is
// called. Run it in a goroutine.
func (c *Client) Listen(onEvent func(raw []byte)) {
	go c.apiLoop()

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		c.listenOnce(onEvent)

		postLog.Warning("[qq_onebot] Event WebSocket disconnected, reconnecting in 5s...")
		select {
		case <-c.stopCh:
			return
		case <-time.After(onebotReconnectDelay):
		}
	}
}

// listenOnce connects to the OneBot event WebSocket and reads events until the
// connection drops or Stop is called.
func (c *Client) listenOnce(onEvent func(raw []byte)) {
	conn, err := c.dialWS("/")
	if err != nil {
		postLog.Error(fmt.Sprintf("[qq_onebot] Failed to connect to OneBot event WebSocket: %v", err))
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

	postLog.Info("[qq_onebot] Connected to OneBot event WebSocket")

	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		_, msgBytes, err := conn.ReadMessage()
		if err != nil {
			postLog.Error(fmt.Sprintf("[qq_onebot] Event WebSocket read error: %v", err))
			return
		}

		onEvent(msgBytes)
	}
}

// apiLoop owns the API socket: it keeps a connection to "/api" up, dispatching
// any reply its read loop receives and reconnecting every onebotReconnectDelay.
// It exits on Stop.
func (c *Client) apiLoop() {
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		c.runAPIConnection()

		select {
		case <-c.stopCh:
			return
		case <-time.After(onebotReconnectDelay):
		}
	}
}

// runAPIConnection dials "/api", publishes the connection to waiters, and reads
// response frames until the connection drops or Stop is called. On the way out it
// marks the socket down (waking waiters and failing in-flight calls) so a caller
// does not wait the full timeout for a reply that can no longer arrive.
func (c *Client) runAPIConnection() {
	conn, err := c.dialWS("/api")
	if err != nil {
		postLog.Error(fmt.Sprintf("[qq_onebot] Failed to connect to OneBot API WebSocket: %v", err))
		return
	}

	c.apiMu.Lock()
	c.apiConn = conn
	close(c.apiChanged)
	c.apiChanged = make(chan struct{})
	c.apiMu.Unlock()

	postLog.Info("[qq_onebot] Connected to OneBot API WebSocket")

	c.readAPILoop(conn)

	c.apiMu.Lock()
	if c.apiConn == conn {
		c.apiConn = nil
	}
	close(c.apiChanged)
	c.apiChanged = make(chan struct{})
	c.apiMu.Unlock()

	conn.Close()
	c.failPending(fmt.Errorf("onebot API WebSocket disconnected"))
}

// readAPILoop reads response frames from the API socket until the connection
// drops or Stop is called. It is the only reader of that connection.
func (c *Client) readAPILoop(conn *websocket.Conn) {
	for {
		select {
		case <-c.stopCh:
			return
		default:
		}

		_, data, err := conn.ReadMessage()
		if err != nil {
			select {
			case <-c.stopCh:
				// Stopping: the read error is just the closed socket.
			default:
				postLog.Error(fmt.Sprintf("[qq_onebot] API WebSocket read error: %v", err))
			}
			return
		}

		c.dispatchAPI(data)
	}
}

// dispatchAPI decodes one inbound frame on the API socket. A frame with no usable
// echo is not an API response and is ignored (the event socket is the only place
// events are expected); otherwise the pending channel registered for that echo
// receives the envelope and is removed, both under apiMu.
func (c *Client) dispatchAPI(raw []byte) {
	var resp APIResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		postLog.Debug("[qq_onebot] Failed to parse OneBot API response: " + err.Error())
		return
	}

	key, ok := echoKey(resp.Echo)
	if !ok {
		postLog.Debug("[qq_onebot] Ignoring non-response frame on OneBot API WebSocket: " + string(raw))
		return
	}

	c.apiMu.Lock()
	ch := c.pending[key]
	delete(c.pending, key)
	c.apiMu.Unlock()

	if ch != nil {
		ch <- apiResult{resp: &resp}
	}
}

// echoKey normalizes an echo value to its pending-map key. An absent or null echo
// yields ok=false, i.e. the frame is not an API response. A JSON string is
// unquoted; any other type falls back to its verbatim bytes so an adapter that
// returns the id unquoted still matches.
func echoKey(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, true
	}
	return string(raw), true
}

// failPending delivers err to every call waiting on a response and clears the
// table, so a dropped API socket unblocks its callers immediately. Called on
// disconnect and by Stop; safe to call repeatedly.
func (c *Client) failPending(err error) {
	c.apiMu.Lock()
	pending := c.pending
	c.pending = make(map[string]chan apiResult)
	c.apiMu.Unlock()

	for _, ch := range pending {
		// Buffered cap 1, and each entry is owned exactly once, so this never blocks.
		ch <- apiResult{err: err}
	}
}

// Stop closes both WebSocket connections and unblocks the event reconnect loop,
// the API loop and every in-flight callAPI. It is idempotent and safe to call
// even when Listen was never started.
func (c *Client) Stop() {
	c.stopOnce.Do(func() {
		close(c.stopCh)

		c.apiMu.Lock()
		c.stopped = true
		apiConn := c.apiConn
		c.apiMu.Unlock()

		c.connMu.Lock()
		if c.conn != nil {
			c.conn.Close()
			c.conn = nil
		}
		c.connMu.Unlock()

		if apiConn != nil {
			apiConn.Close()
		}

		c.failPending(fmt.Errorf("onebot client stopped"))
	})
}
