package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"nukumizu-backend/internal/node"
	"nukumizu-backend/utils"
)

// adminGet builds an authenticated GET request for the given path using an
// admin token and a fresh X-Timestamp.
func adminGet(t *testing.T, path string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Header.Set("X-Token", "test-admin-token")
	req.Header.Set("X-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	return req
}

// seedTracker initializes the global node tracker with two nodes, one online
// with a status report and one offline that has not reported yet.
func seedTracker(t *testing.T) {
	t.Helper()
	node.InitTracker()

	tracker := node.GetTracker()
	tracker.UpdateNodeList(map[string]node.NodeListEntry{
		"u1": {
			Name: "alpha",
			Info: &node.Info{},
		},
		"u2": {
			Name: "beta",
			Info: &node.Info{},
		},
	})

	report := node.Report{}
	report.CPU.Usage = 12.5
	report.RAM.Total = 1024
	report.RAM.Used = 512
	tracker.UpdateStatus([]string{"u1"}, map[string]node.Report{"u1": report})
}

func setupAdminToken() {
	utils.AddToken("test-admin-token", 1, "admin", "tester")
}

func decodeResponse(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return body
}

func TestServerGetInfoAll(t *testing.T) {
	setupAdminToken()
	seedTracker(t)

	w := httptest.NewRecorder()
	ServerGetInfoHandler(w, adminGet(t, "/api/server/getInfo?uuid=all"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	body := decodeResponse(t, w)
	for _, uuid := range []string{"u1", "u2"} {
		if _, ok := body[uuid]; !ok {
			t.Errorf("response missing uuid %q: %s", uuid, w.Body.String())
		}
	}

	var one struct {
		UUID string     `json:"uuid"`
		Name string     `json:"name"`
		Info *node.Info `json:"info"`
	}
	if err := json.Unmarshal(body["u1"], &one); err != nil {
		t.Fatalf("decode u1: %v", err)
	}
	if one.UUID != "u1" || one.Name != "alpha" {
		t.Errorf("u1 = %+v", one)
	}
	if one.Info == nil {
		t.Error("expected static info present for u1")
	}
}

func TestServerGetInfoSingleAndMissing(t *testing.T) {
	setupAdminToken()
	seedTracker(t)

	// Single existing uuid: response is keyed by that uuid (uniform shape).
	w := httptest.NewRecorder()
	ServerGetInfoHandler(w, adminGet(t, "/api/server/getInfo?uuid=u1"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := decodeResponse(t, w)
	if _, ok := body["u1"]; !ok {
		t.Errorf("single-uuid response missing key u1: %s", w.Body.String())
	}

	// Unknown uuid yields 404.
	w2 := httptest.NewRecorder()
	ServerGetInfoHandler(w2, adminGet(t, "/api/server/getInfo?uuid=ghost"))
	if w2.Code != http.StatusNotFound {
		t.Errorf("missing uuid status = %d, want 404", w2.Code)
	}
}

func TestServerGetStatusAll(t *testing.T) {
	setupAdminToken()
	seedTracker(t)

	w := httptest.NewRecorder()
	ServerGetStatusHandler(w, adminGet(t, "/api/server/getStatus?uuid=all"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	body := decodeResponse(t, w)

	var online struct {
		UUID   string       `json:"uuid"`
		Name   string       `json:"name"`
		Online bool         `json:"online"`
		Report *node.Report `json:"report"`
	}
	if err := json.Unmarshal(body["u1"], &online); err != nil {
		t.Fatalf("decode u1: %v", err)
	}
	if !online.Online || online.Report == nil {
		t.Errorf("u1 should be online with a report: %+v", online)
	}

	var offline struct {
		Online bool         `json:"online"`
		Report *node.Report `json:"report"`
	}
	if err := json.Unmarshal(body["u2"], &offline); err != nil {
		t.Fatalf("decode u2: %v", err)
	}
	if offline.Online {
		t.Error("u2 should be offline")
	}
	if offline.Report != nil {
		t.Errorf("u2 report should be null, got %+v", offline.Report)
	}
}
