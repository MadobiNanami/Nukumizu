package handler

import (
	"net/http"

	"nukumizu-backend/database"
	"nukumizu-backend/utils"
)

// HealthHandler handles GET /health.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		utils.SendErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	dbStatus := "ok"
	if database.UserDB == nil {
		dbStatus = "not initialized"
	} else if err := database.UserDB.Ping(); err != nil {
		dbStatus = "error: " + err.Error()
	}

	utils.SendSuccessResponse(w, "", map[string]interface{}{
		"status":   "ok",
		"database": dbStatus,
	})
}
