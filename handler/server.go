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

// ServerInfoValue is the per-server payload returned by GET /api/server/getInfo,
// mirroring the static server info the Bot prints for /info.
type ServerInfoValue struct {
	UUID string     `json:"uuid"`
	Name string     `json:"name"`
	Info *node.Info `json:"info"`
}

// ServerStatusValue is the per-server payload returned by GET /api/server/getStatus,
// mirroring the live status the Bot prints for /status. Report is null when the
// node is known but has not delivered a status report yet.
type ServerStatusValue struct {
	UUID   string        `json:"uuid"`
	Name   string        `json:"name"`
	Online bool          `json:"online"`
	Report *node.Report  `json:"report"`
}

// collectTargetNodes resolves the uuid query parameter into the list of nodes
// the caller asked for. uuid "all" selects every tracked node; any other uuid
// selects that single node. A boolean reports whether the uuid was found.
func collectTargetNodes(tracker *node.Tracker, uuid string) ([]*node.Node, bool) {
	if uuid == "all" {
		return tracker.GetAllNodes(), true
	}
	n, exists := tracker.GetNode(uuid)
	if !exists {
		return nil, false
	}
	return []*node.Node{n}, true
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

// ServerGetInfoHandler handles GET /api/server/getInfo?uuid=xxx|all.
// Returns the static info of the requested server(s) as a uuid-keyed object,
// mirroring the data the Bot uses for /info.
func ServerGetInfoHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "GET", "admin") {
		return
	}

	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing uuid parameter")
		return
	}

	tracker := node.GetTracker()
	if tracker == nil {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "node tracker not initialized")
		return
	}

	nodes, found := collectTargetNodes(tracker, uuid)
	if !found {
		utils.SendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("server with uuid %s not found", uuid))
		return
	}

	result := make(map[string]interface{}, len(nodes))
	for _, n := range nodes {
		result[n.UUID] = ServerInfoValue{UUID: n.UUID, Name: n.Name, Info: n.Info}
	}
	utils.SendSuccessResponse(w, "", result)
}

// ServerGetStatusHandler handles GET /api/server/getStatus?uuid=xxx|all.
// Returns the live status of the requested server(s) as a uuid-keyed object,
// mirroring the data the Bot uses for /status.
func ServerGetStatusHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "GET", "admin") {
		return
	}

	uuid := r.URL.Query().Get("uuid")
	if uuid == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "missing uuid parameter")
		return
	}

	tracker := node.GetTracker()
	if tracker == nil {
		utils.SendErrorResponse(w, http.StatusInternalServerError, "node tracker not initialized")
		return
	}

	nodes, found := collectTargetNodes(tracker, uuid)
	if !found {
		utils.SendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("server with uuid %s not found", uuid))
		return
	}

	result := make(map[string]interface{}, len(nodes))
	for _, n := range nodes {
		result[n.UUID] = ServerStatusValue{UUID: n.UUID, Name: n.Name, Online: n.Online, Report: n.LatestReport}
	}
	utils.SendSuccessResponse(w, "", result)
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
