package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/global"
)

// tempConfigFile points one of the settings paths at a throwaway file, so a
// test can drive the settings API without touching the config files in the
// working directory.
func tempConfigFile(t *testing.T, field *string, content string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp config: %v", err)
	}
	original := *field
	*field = path
	t.Cleanup(func() { *field = original })
}

// settingsSetRequest builds an authenticated POST for the settings endpoint.
func settingsSetRequest(t *testing.T, settingsType, body string) *http.Request {
	t.Helper()
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/settings/set?type="+settingsType,
		strings.NewReader(body),
	)
	req.Header.Set("X-Token", "test-admin-token")
	req.Header.Set("X-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	return req
}

// settingsSetResponse is the envelope /api/settings/set answers with.
type settingsSetResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Type            string   `json:"type"`
		RestartRequired []string `json:"restartRequired"`
	} `json:"data"`
}

func decodeSettingsSetResponse(t *testing.T, w *httptest.ResponseRecorder) settingsSetResponse {
	t.Helper()
	var body settingsSetResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, w.Body.String())
	}
	return body
}

// TestSettingsSetReportsStartupOnlyKeys covers the field the console reads to
// tell the user which of their edits are not live yet.
func TestSettingsSetReportsStartupOnlyKeys(t *testing.T) {
	setupAdminToken()
	tempConfigFile(t, &global.ConfigPath.Global, `{"system":{"debugMode":false,"listenPort":"8080"}}`)

	w := httptest.NewRecorder()
	SettingsSetHandler(w, settingsSetRequest(t, "global",
		`{"system":{"listenPort":"9090","debugMode":true}}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	body := decodeSettingsSetResponse(t, w)
	if !body.Success {
		t.Fatalf("not a success envelope: %s", w.Body.String())
	}
	if body.Data.Type != "global" {
		t.Errorf("data.type = %q, want global", body.Data.Type)
	}
	if len(body.Data.RestartRequired) != 1 || body.Data.RestartRequired[0] != "system.listenPort" {
		t.Errorf("restartRequired = %v, want [system.listenPort]", body.Data.RestartRequired)
	}

	// Being reported as startup-only must not stop the write.
	data, err := config.GetSettings(config.SettingGlobal)
	if err != nil {
		t.Fatalf("GetSettings: %v", err)
	}
	for _, want := range []string{`"listenPort": "9090"`, `"debugMode": true`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the patch was not applied, missing %s:\n%s", want, data)
		}
	}
}

// TestSettingsSetRestartRequiredIsAlwaysAnArray pins the shape a client
// iterates over: an update with nothing to report must answer [] and not null.
func TestSettingsSetRestartRequiredIsAlwaysAnArray(t *testing.T) {
	setupAdminToken()
	tempConfigFile(t, &global.ConfigPath.Global, `{"system":{"debugMode":false}}`)

	w := httptest.NewRecorder()
	SettingsSetHandler(w, settingsSetRequest(t, "global", `{"system":{"debugMode":true}}`))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"restartRequired":[]`) {
		t.Errorf("restartRequired should serialize as an empty array: %s", w.Body.String())
	}
}

// TestSettingsSetRejectsUnknownType keeps the failure path intact now that the
// success path computes an extra field.
func TestSettingsSetRejectsUnknownType(t *testing.T) {
	setupAdminToken()
	tempConfigFile(t, &global.ConfigPath.Global, `{}`)

	w := httptest.NewRecorder()
	SettingsSetHandler(w, settingsSetRequest(t, "nonsense", `{}`))

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
