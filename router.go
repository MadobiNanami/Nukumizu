package main

import (
	"fmt"
	"net/http"

	"nukumizu-backend/handler"
	"nukumizu-backend/postLog"
)

// SetupRouter registers all HTTP routes and returns a configured ServeMux.
func SetupRouter() *http.ServeMux {
	postLog.Info("Setting up routers...")

	mux := http.NewServeMux()

	// User endpoints.
	mux.HandleFunc("/api/user/login", handler.UserLoginHandler)
	mux.HandleFunc("/api/user/register", handler.UserRegisterHandler)

	// Server endpoints (authenticated).
	mux.HandleFunc("/api/server/list", handler.ServerListHandler)
	mux.HandleFunc("/api/server/getStatus", handler.ServerGetStatusHandler)
	mux.HandleFunc("/api/server/exec", handler.ServerExecHandler)

	// Health check endpoint.
	mux.HandleFunc("/health", handler.HealthHandler)

	// WebSocket log streaming endpoint.
	logBroadcaster := postLog.GetLogBroadcaster()
	if logBroadcaster != nil {
		logSocketHandler := postLog.NewLogSocketHandler(logBroadcaster)
		mux.HandleFunc("/api/system/getLogs", logSocketHandler.Handle)
	}

	// Catch-all 404 handler.
	mux.HandleFunc("/", NotFoundHandler)

	postLog.Info("Router setup completed")
	return mux
}

// NotFoundHandler returns a 404 JSON response for unknown routes.
func NotFoundHandler(w http.ResponseWriter, r *http.Request) {
	postLog.Debug(fmt.Sprintf("Unknown request: %s %s", r.Method, r.URL.Path))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	fmt.Fprint(w, `{"success":false,"message":"route not found"}`)
}
