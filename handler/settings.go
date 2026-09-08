package handler

import (
	"encoding/json"
	"net/http"

	"nukumizu-backend/config"
	"nukumizu-backend/postLog"
	"nukumizu-backend/utils"
)

// SettingsGetHandler handles GET /api/settings/get?type=xxx.
// type selects which config file to return and may be one of
// "global", "bot_user_config" or "bot_node_config"; the returned "config"
// object has the same layout as the source JSON file.
func SettingsGetHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "GET", "admin") {
		return
	}

	settingsType := r.URL.Query().Get("type")
	if settingsType == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing type parameter")
		return
	}
	if !config.IsValidSettingsType(settingsType) {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid type parameter: "+settingsType)
		return
	}

	data, err := config.GetSettings(settingsType)
	if err != nil {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to get settings: "+err.Error())
		return
	}

	utils.SendSuccessResponse(w, "", map[string]interface{}{
		"config": json.RawMessage(data),
	})
}

// SettingsSetHandler handles POST /api/settings/set?type=xxx.
// type selects which config file to update and may be one of "global",
// "bot_user_config" or "bot_node_config". The JSON body is a partial config
// object whose keys map directly to config entries, e.g.
//
//	{"system": {"debugMode": true}}
//
// Multiple entries may be given at once; only the provided keys are changed.
func SettingsSetHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "POST", "admin") {
		return
	}

	settingsType := r.URL.Query().Get("type")
	if settingsType == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing type parameter")
		return
	}
	if !config.IsValidSettingsType(settingsType) {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid type parameter: "+settingsType)
		return
	}

	dec := json.NewDecoder(r.Body)
	dec.UseNumber() // Keep numeric values verbatim (e.g. QQ IDs) instead of float64.
	var patch map[string]interface{}
	if err := dec.Decode(&patch); err != nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid request body: expected a JSON object")
		return
	}
	if patch == nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "request body must be a JSON object")
		return
	}

	if err := config.UpdateSettings(settingsType, patch); err != nil {
		postLog.Error("Failed to update " + settingsType + " settings: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to update settings: "+err.Error())
		return
	}

	utils.SendSuccessResponse(w, "settings updated successfully", map[string]interface{}{
		"type": settingsType,
	})
}
