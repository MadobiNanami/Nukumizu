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

// checkTimestamp validates a Unix timestamp in seconds against the server clock
// (30 minute tolerance per agent.md). The check is skipped entirely in debug
// mode. It returns 0 when the timestamp is acceptable, otherwise the HTTP status
// and message to reject the request with.
func checkTimestamp(timestamp string) (int, string) {
	if config.IsDebugMode() {
		return 0, ""
	}

	if timestamp == "" {
		return http.StatusUnauthorized, "missing timestamp"
	}

	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return http.StatusUnauthorized, "invalid timestamp"
	}

	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
	}
	if diff > 1800 {
		return http.StatusUnauthorized, "request expired"
	}

	return 0, ""
}

// checkPermission validates a session token against the required permission
// level and refreshes the token's idle timer on success.
//
// Permission levels: "None" (public, no token required), "bot", "admin".
// When level is "bot", both "bot" and "admin" tokens are accepted.
// When level is "admin", only "admin" tokens are accepted.
//
// It returns 0 when the token is authorized, otherwise the HTTP status and
// message to reject the request with.
func checkPermission(token string, targetLevel string) (int, string) {
	// Public endpoints require no token.
	if targetLevel == "None" {
		return 0, ""
	}

	if token == "" {
		return http.StatusUnauthorized, "missing token"
	}

	tokenInfo, exists := GetTokenInfo(token)
	if !exists {
		return http.StatusUnauthorized, "invalid token"
	}

	// Check permission level.
	// "bot" level accepts both "bot" and "admin" tokens.
	// "admin" level accepts only "admin" tokens.
	switch targetLevel {
	case "admin":
		if tokenInfo.Level != "admin" {
			return http.StatusForbidden, "permission denied"
		}
	case "bot":
		if tokenInfo.Level != "bot" && tokenInfo.Level != "admin" {
			return http.StatusForbidden, "permission denied"
		}
	}

	// Refresh token last access time.
	RefreshToken(token)
	return 0, ""
}

// Auth is the central authentication and authorization function.
// It validates the request method, X-Timestamp header (30min tolerance),
// X-Token header, and permission level. Returns true if the request is authorized.
//
// Permission levels: "None" (public, no token required), "bot", "admin".
// When level is "bot", both "bot" and "admin" tokens are accepted.
// When level is "admin", only "admin" tokens are accepted.
//
// WebSocket upgrades cannot carry custom headers from a browser; those endpoints
// use WebSocketAuthMiddleware instead, which also accepts the credentials as
// query parameters.
func Auth(w http.ResponseWriter, r *http.Request, targetMethod string, targetLevel string) bool {
	// Validate HTTP method.
	if r.Method != targetMethod {
		SendErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
		return false
	}

	// Validate X-Timestamp.
	if status, message := checkTimestamp(r.Header.Get("X-Timestamp")); status != 0 {
		SendErrorResponse(w, status, message)
		return false
	}

	// Validate X-Token and its permission level.
	if status, message := checkPermission(r.Header.Get("X-Token"), targetLevel); status != 0 {
		SendErrorResponse(w, status, message)
		return false
	}

	return true
}


// AuthWS gates a WebSocket endpoint behind the given permission
// level ("bot" or "admin"), authenticating the upgrade request before the
// connection is handed to the handler. Unauthorized requests are answered with
// the standard JSON error response and are never upgraded.
//
// A browser cannot set custom headers on a WebSocket handshake, so the session
// token and timestamp are read from the X-Token / X-Timestamp headers when
// present and otherwise from the "token" and "timestamp" query parameters:
//
//	ws://host/api/system/getLogs?token=<token>&timestamp=<unix seconds>
//
// The timestamp is only checked at handshake time, so a long-lived connection
// stays open past its tolerance window. Because a query string commonly ends up
// in proxy and access logs, a token-carrying URL should be treated as a secret.
func AuthWS(targetLevel string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// An upgrade request is always a GET.
			if r.Method != http.MethodGet {
				SendErrorResponse(w, http.StatusMethodNotAllowed, "method not allowed")
				return
			}

			// Headers win over query parameters so programmatic clients can keep
			// the credentials out of the URL.
			token := r.Header.Get("X-Token")
			timestamp := r.Header.Get("X-Timestamp")
			query := r.URL.Query()
			if token == "" {
				token = query.Get("token")
			}
			if timestamp == "" {
				timestamp = query.Get("timestamp")
			}

			if status, message := checkTimestamp(timestamp); status != 0 {
				SendErrorResponse(w, status, message)
				return
			}
			if status, message := checkPermission(token, targetLevel); status != 0 {
				SendErrorResponse(w, status, message)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
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

// SendSuccessResponse sends a standardized JSON success response. Every payload
// is nested under a single "data" key, so success responses use the envelope
// {"success": true, "message": "...", "data": {...}}.
func SendSuccessResponse(w http.ResponseWriter, message string, data map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	resp := map[string]interface{}{
		"success": true,
	}
	if message != "" {
		resp["message"] = message
	}
	if data == nil {
		data = map[string]interface{}{}
	}
	resp["data"] = data
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
