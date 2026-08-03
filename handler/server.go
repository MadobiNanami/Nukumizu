package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"nukumizu-backend/internal/komari"
	"nukumizu-backend/internal/node"
	"nukumizu-backend/internal/template"
	"nukumizu-backend/postLog"
	"nukumizu-backend/utils"
)

// ServerExecRequest represents the request body for POST /api/server/exec.
type ServerExecRequest struct {
	UUID    []string `json:"uuid"`
	Command string   `json:"command"`
}

// ServerListHandler handles GET /api/server/list.
func ServerListHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "GET", "bot") {
		return
	}

	tracker := node.GetTracker()
	if tracker == nil {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "node tracker not initialized")
		return
	}

	params := template.BuildParamsFromServerList()
	result := template.Render("", params)

	utils.SendSuccessResponse(w, "", map[string]interface{}{
		"list": result,
	})
}

// ServerGetStatusHandler handles GET /api/server/getStatus?uuid=xxx.
func ServerGetStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "GET", "bot") {
		return
	}

	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing uuid parameter")
		return
	}

	// First try to get data from the local tracker.
	tracker := node.GetTracker()
	if tracker != nil {
		if n, exists := tracker.GetNode(uuid); exists && n.LatestReport != nil {
			utils.SendSuccessResponse(w, "", map[string]interface{}{
				"uuid":   uuid,
				"report": n.LatestReport,
			})
			return
		}
	}

	utils.SendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("no recent data for uuid: %s", uuid))
}

// ServerExecHandler handles POST /api/server/exec.
func ServerExecHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "POST", "bot") {
		return
	}

	var req ServerExecRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if len(req.UUID) == 0 {
		utils.SendErrorResponse(w, http.StatusBadRequest, "uuid array is required")
		return
	}

	if req.Command == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "command is required")
		return
	}

	// Get the Komari client from the global state.
	komariClient := komari.GetClient()
	if komariClient == nil {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "komari client not initialized")
		return
	}

	taskID, err := komariClient.ExecTask(req.UUID, req.Command)
	if err != nil {
		postLog.Error("Komari exec task failed: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to execute command: "+err.Error())
		return
	}

	results, err := komariClient.PollTaskResult(taskID)
	if err != nil {
		postLog.Error("Komari task polling failed: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to get task result: "+err.Error())
		return
	}

	utils.SendSuccessResponse(w, "", map[string]interface{}{
		"taskID":  taskID,
		"results": results,
	})
}
