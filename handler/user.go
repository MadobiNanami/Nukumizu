package handler

import (
	"encoding/json"
	"net/http"

	db "nukumizu-backend/database"
	"nukumizu-backend/postLog"
	"nukumizu-backend/utils"
)

// UserLoginRequest represents the login request body.
type UserLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// UserLoginHandler handles POST /api/user/login.
func UserLoginHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "POST", "None") {
		return
	}

	var req UserLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "username and password are required")
		return
	}

	userID, username, level, registerDate, err := db.GetUserByUsername(req.Username, req.Password)
	if err != nil {
		postLog.Warning("Login failed for user " + req.Username + ": " + err.Error())
		utils.SendErrorResponse(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	token, err := utils.GenerateToken()
	if err != nil {
		postLog.Error("Failed to generate token: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	utils.AddToken(token, userID, level, username)
	postLog.Info("User logged in: " + username)

	utils.SendSuccessResponse(w, "login successful", map[string]interface{}{
		"token":        token,
		"userID":       userID,
		"username":     username,
		"level":        level,
		"registerDate": registerDate,
	})
}

// UserRegisterHandler handles POST /api/user/register.
// Only allowed when there are no existing users in the database.
func UserRegisterHandler(w http.ResponseWriter, r *http.Request) {
	if !utils.Auth(w, r, "POST", "None") {
		return
	}

	// Check if any user already exists.
	count, err := db.GetUserCount()
	if err != nil {
		postLog.Error("Failed to check user count: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "database error")
		return
	}
	if count > 0 {
		utils.SendErrorResponse(w, http.StatusForbidden, "registration is closed: users already exist")
		return
	}

	var req UserLoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		utils.SendErrorResponse(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		utils.SendErrorResponse(w, http.StatusBadRequest, "username and password are required")
		return
	}

	if len(req.Username) > 50 {
		utils.SendErrorResponse(w, http.StatusBadRequest, "username too long (max 50 characters)")
		return
	}

	if len(req.Password) < 6 || len(req.Password) > 100 {
		utils.SendErrorResponse(w, http.StatusBadRequest, "password must be between 6 and 100 characters")
		return
	}

	userID, err := db.CreateUser(req.Username, req.Password, "admin")
	if err != nil {
		postLog.Error("Failed to register user: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to register user, username may already exist")
		return
	}

	token, err := utils.GenerateToken()
	if err != nil {
		postLog.Error("Failed to generate token: " + err.Error())
		utils.SendErrorResponse(w, http.StatusInternalServerError, "failed to generate token")
		return
	}

	utils.AddToken(token, userID, "admin", req.Username)
	postLog.Info("User registered: " + req.Username)

	utils.SendSuccessResponse(w, "user registered successfully", map[string]interface{}{
		"token":    token,
		"userID":   userID,
		"username": req.Username,
		"level":    "admin",
	})
}
