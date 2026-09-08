package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	db "nukumizu-backend/database"
)

// initHandlerUserDB opens a fresh user database in a temp directory for the
// register handler tests.
func initHandlerUserDB(t *testing.T) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "user.db")
	if err := db.InitUserDB(path); err != nil {
		t.Fatalf("InitUserDB: %v", err)
	}
	t.Cleanup(db.CloseUserDB)
}

func registerRequest(t *testing.T, username, password string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": password})
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/user/register", bytes.NewReader(body))
	req.Header.Set("X-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	return req
}

// TestRegisterOnlyFirstUser exercises the API rule: the database accepts
// exactly the first registration and rejects every later one.
func TestRegisterOnlyFirstUser(t *testing.T) {
	initHandlerUserDB(t)

	w := httptest.NewRecorder()
	UserRegisterHandler(w, registerRequest(t, "alice", "password1"))
	if w.Code != http.StatusOK {
		t.Fatalf("first register status = %d, body = %s", w.Code, w.Body.String())
	}

	w2 := httptest.NewRecorder()
	UserRegisterHandler(w2, registerRequest(t, "bob", "password2"))
	if w2.Code != http.StatusForbidden {
		t.Fatalf("second register status = %d, body = %s", w2.Code, w2.Body.String())
	}
}
