package utils

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"nukumizu-backend/config"
	"nukumizu-backend/postLog"
)

// TokenInfo holds information about an authenticated session token.
type TokenInfo struct {
	UserID     int64     `json:"userID"`
	Level      string    `json:"level"`
	UserName   string    `json:"userName"`
	CreatedAt  time.Time `json:"createdAt"`
	LastAccess time.Time `json:"lastAccess"`
}

var (
	tokenStore     = make(map[string]*TokenInfo)
	tokenStoreLock sync.RWMutex
)

// GenerateToken creates a cryptographically random 32-byte hex token.
func GenerateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

// AddToken adds a token to the in-memory token store.
func AddToken(token string, userID int64, level string, userName string) {
	tokenStoreLock.Lock()
	defer tokenStoreLock.Unlock()
	now := time.Now()
	tokenStore[token] = &TokenInfo{
		UserID:     userID,
		Level:      level,
		UserName:   userName,
		CreatedAt:  now,
		LastAccess: now,
	}
}

// GetTokenInfo retrieves token information from the store.
func GetTokenInfo(token string) (*TokenInfo, bool) {
	tokenStoreLock.RLock()
	defer tokenStoreLock.RUnlock()
	info, exists := tokenStore[token]
	return info, exists
}

// RefreshToken updates the LastAccess time for an active token.
func RefreshToken(token string) {
	tokenStoreLock.Lock()
	defer tokenStoreLock.Unlock()
	if info, exists := tokenStore[token]; exists {
		info.LastAccess = time.Now()
	}
}

// RemoveToken deletes a token from the store.
func RemoveToken(token string) {
	tokenStoreLock.Lock()
	defer tokenStoreLock.Unlock()
	delete(tokenStore, token)
}

// GetUserIDFromRequest extracts the user ID from the request's X-Token header.
func GetUserIDFromRequest(r *http.Request) int64 {
	token := r.Header.Get("X-Token")
	if token == "" {
		return 0
	}
	tokenInfo, exists := GetTokenInfo(token)
	if !exists {
		return 0
	}
	return tokenInfo.UserID
}

// GetUserLevelFromRequest extracts the user level from the request's X-Token header.
func GetUserLevelFromRequest(r *http.Request) string {
	token := r.Header.Get("X-Token")
	if token == "" {
		return ""
	}
	tokenInfo, exists := GetTokenInfo(token)
	if !exists {
		return ""
	}
	return tokenInfo.Level
}

// Auth is the central authentication and authorization function.
// It validates the request method, X-Timestamp header (30min tolerance),
// X-Token header, and permission level. Returns true if the request is authorized.
//
// Permission levels: "None" (public, no token required), "bot", "admin".
// When level is "bot", both "bot" and "admin" tokens are accepted.
// When level is "admin", only "admin" tokens are accepted.
func Auth(w http.ResponseWriter, r *http.Request, targetMethod string, targetLevel string) bool {
	// Validate HTTP method.
	if r.Method != targetMethod {
		SendErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}

	// Validate X-Timestamp.
	timestamp := r.Header.Get("X-Timestamp")
	if !config.IsDebugMode() {
		if timestamp == "" {
			SendErrorResponse(w, http.StatusUnauthorized, "missing timestamp")
			return false
		}

		ts, err := strconv.ParseInt(timestamp, 10, 64)
		if err != nil {
			SendErrorResponse(w, http.StatusUnauthorized, "invalid timestamp")
			return false
		}

		now := time.Now().Unix()
		diff := now - ts
		if diff < 0 {
			diff = -diff
		}
		// 30 minute tolerance per agent.md.
		if diff > 1800 {
			SendErrorResponse(w, http.StatusUnauthorized, "request expired")
			return false
		}
	}

	// Public endpoints require no token.
	if targetLevel == "None" {
		return true
	}

	// Validate X-Token.
	token := r.Header.Get("X-Token")
	if token == "" {
		SendErrorResponse(w, http.StatusUnauthorized, "missing token")
		return false
	}

	tokenInfo, exists := GetTokenInfo(token)
	if !exists {
		SendErrorResponse(w, http.StatusUnauthorized, "invalid token")
		return false
	}

	// Check permission level.
	// "bot" level accepts both "bot" and "admin" tokens.
	// "admin" level accepts only "admin" tokens.
	switch targetLevel {
	case "admin":
		if tokenInfo.Level != "admin" {
			SendErrorResponse(w, http.StatusForbidden, "permission denied")
			return false
		}
	case "bot":
		if tokenInfo.Level != "bot" && tokenInfo.Level != "admin" {
			SendErrorResponse(w, http.StatusForbidden, "permission denied")
			return false
		}
	}

	// Refresh token last access time.
	RefreshToken(token)
	return true
}

// CleanExpiredTokens removes tokens that have been idle for over 1 hour.
func CleanExpiredTokens() {
	tokenStoreLock.Lock()
	defer tokenStoreLock.Unlock()
	now := time.Now()
	for token, info := range tokenStore {
		if now.Sub(info.LastAccess) > 1*time.Hour {
			delete(tokenStore, token)
			postLog.Debug("Expired token removed for user: " + info.UserName)
		}
	}
}

// StartTokenCleaner starts a background goroutine that periodically cleans
// expired tokens.
func StartTokenCleaner() {
	ticker := time.NewTicker(10 * time.Minute)
	go func() {
		for range ticker.C {
			CleanExpiredTokens()
		}
	}()
}

// SendSuccessResponse sends a standardized JSON success response.
func SendSuccessResponse(w http.ResponseWriter, message string, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"success": true,
	}
	if message != "" {
		resp["message"] = message
	}
	for k, v := range data {
		resp[k] = v
	}
	json.NewEncoder(w).Encode(resp)
}

// SendErrorResponse sends a standardized JSON error response.
func SendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"message": message,
	})
}
