package qq_onebot

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// fakeOneBot is a minimal OneBot 11 forward-WebSocket server. "/api" answers
// every action call by echoing its echo with a fixed ok response; "/" accepts the
// event socket and hands it to the test so events can be pushed on demand.
type fakeOneBot struct {
	upgrader websocket.Upgrader

	server      *httptest.Server
	apiRequests chan fakeAPIRequest
	eventConns  chan *websocket.Conn
}

type fakeAPIRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
	Echo   json.RawMessage `json:"echo"`
}

func newFakeOneBot(t *testing.T) *fakeOneBot {
	t.Helper()

	f := &fakeOneBot{
		apiRequests: make(chan fakeAPIRequest, 16),
		eventConns:  make(chan *websocket.Conn, 16),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/api", func(w http.ResponseWriter, r *http.Request) {
		conn, err := f.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		for {
			_, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			var req fakeAPIRequest
			if err := json.Unmarshal(data, &req); err != nil {
				continue
			}
			f.apiRequests <- req

			resp, _ := json.Marshal(map[string]json.RawMessage{
				"status":  json.RawMessage(`"ok"`),
				"retcode": json.RawMessage(`0`),
				"data":    json.RawMessage(`{"message_id":123}`),
				"echo":    req.Echo,
			})
			if err := conn.WriteMessage(websocket.TextMessage, resp); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		conn, err := f.upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		f.eventConns <- conn

		// Keep the event socket open until the client goes away.
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	})

	f.server = httptest.NewServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

// client returns a Client pointed at this fake server with the proxy disabled.
func (f *fakeOneBot) client(t *testing.T) *Client {
	t.Helper()
	u, err := url.Parse(f.server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	return NewClient(u.Hostname(), u.Port(), "", false)
}

// eventConn waits for the event socket to be established and returns it.
func (f *fakeOneBot) eventConn(t *testing.T) *websocket.Conn {
	t.Helper()
	select {
	case conn := <-f.eventConns:
		return conn
	case <-time.After(3 * time.Second):
		t.Fatal("event WebSocket never connected")
		return nil
	}
}

// TestSendMsgOverWebSocket checks an API call is issued over the "/api" socket
// and its response is decoded.
func TestSendMsgOverWebSocket(t *testing.T) {
	f := newFakeOneBot(t)

	c := f.client(t)
	go c.Listen(func([]byte) {})
	defer c.Stop()

	resp, err := c.SendMsg("group", 12345, "hello", false, 0)
	if err != nil {
		t.Fatalf("SendMsg: %v", err)
	}
	if resp.Status != "ok" || resp.RetCode != 0 {
		t.Fatalf("unexpected response: %+v", resp)
	}

	var data struct {
		MessageID int64 `json:"message_id"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data.MessageID != 123 {
		t.Fatalf("message_id = %d, want 123", data.MessageID)
	}

	req := <-f.apiRequests
	if req.Action != "send_group_msg" {
		t.Fatalf("action = %q, want send_group_msg", req.Action)
	}
	var params map[string]any
	if err := json.Unmarshal(req.Params, &params); err != nil {
		t.Fatalf("decode params: %v", err)
	}
	if params["group_id"].(float64) != 12345 || params["message"].(string) != "hello" {
		t.Fatalf("params = %v", params)
	}
	var echo string
	if err := json.Unmarshal(req.Echo, &echo); err != nil {
		t.Fatalf("echo is not a JSON string: %s", req.Echo)
	}
}

// TestBodylessActionSendsEmptyParams checks actions without arguments still send
// a params object (never null), which some adapters reject.
func TestBodylessActionSendsEmptyParams(t *testing.T) {
	f := newFakeOneBot(t)

	c := f.client(t)
	go c.Listen(func([]byte) {})
	defer c.Stop()

	if _, err := c.GetGroupList(); err != nil {
		t.Fatalf("GetGroupList: %v", err)
	}

	req := <-f.apiRequests
	if req.Action != "get_group_list" {
		t.Fatalf("action = %q, want get_group_list", req.Action)
	}
	if string(req.Params) != "{}" {
		t.Fatalf("params = %s, want {}", req.Params)
	}
}

// TestSendMsgFromEventCallbackDoesNotDeadlock is the regression test for the
// reason the client uses two sockets: the event handler replies with SendMsg from
// inside the event read loop, so a response read on that same connection would
// never arrive.
func TestSendMsgFromEventCallbackDoesNotDeadlock(t *testing.T) {
	f := newFakeOneBot(t)

	c := f.client(t)

	type result struct {
		resp *APIResponse
		err  error
	}
	done := make(chan result, 1)
	go c.Listen(func(raw []byte) {
		resp, err := c.SendMsg("group", 1, "pong", false, 0)
		done <- result{resp, err}
	})
	defer c.Stop()

	evConn := f.eventConn(t)
	event := []byte(`{"post_type":"message","message_type":"group","group_id":1,"user_id":2,"raw_message":"/ping","self_id":9}`)
	if err := evConn.WriteMessage(websocket.TextMessage, event); err != nil {
		t.Fatalf("write event: %v", err)
	}

	select {
	case r := <-done:
		if r.err != nil {
			t.Fatalf("SendMsg from event callback: %v", r.err)
		}
		if r.resp.Status != "ok" {
			t.Fatalf("unexpected response: %+v", r.resp)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("deadlock: SendMsg did not return from within the event callback")
	}
}

// TestEchoKey covers the response-matcher: an absent or null echo is not a
// response, and both string and non-string echoes normalize to a key.
func TestEchoKey(t *testing.T) {
	cases := []struct {
		in  string
		key string
		ok  bool
	}{
		{`"42"`, "42", true},
		{`42`, "42", true},
		{`null`, "", false},
		{``, "", false},
	}
	for _, tc := range cases {
		key, ok := echoKey(json.RawMessage(tc.in))
		if key != tc.key || ok != tc.ok {
			t.Errorf("echoKey(%q) = (%q, %v), want (%q, %v)", tc.in, key, ok, tc.key, tc.ok)
		}
	}
}
