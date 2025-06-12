package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// AuthMiddleware handles authentication
type AuthMiddleware struct {
	password string
	sessions map[string]*Session
	mu       sync.Mutex
}

// Session represents an authenticated session
type Session struct {
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewAuthMiddleware creates a new authentication middleware
func NewAuthMiddleware(password string) *AuthMiddleware {
	return &AuthMiddleware{
		password: password,
		sessions: make(map[string]*Session),
	}
}

// generateToken generates a random session token
func (am *AuthMiddleware) generateToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// HandleLogin handles login requests
func (am *AuthMiddleware) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var loginData struct {
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&loginData); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if loginData.Password != am.password {
		http.Error(w, "Invalid password", http.StatusUnauthorized)
		return
	}

	// Generate session token
	token, err := am.generateToken()
	if err != nil {
		http.Error(w, "Failed to generate session", http.StatusInternalServerError)
		return
	}

	session := &Session{
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // 24 hour session
	}

	am.mu.Lock()
	am.sessions[token] = session
	am.mu.Unlock()

	// Clean up expired sessions
	go am.cleanupExpiredSessions()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"expires_at": session.ExpiresAt.Format(time.RFC3339),
	})
}

// HandleLogout handles logout requests
func (am *AuthMiddleware) HandleLogout(w http.ResponseWriter, r *http.Request) {
	token := am.extractToken(r)
	if token == "" {
		http.Error(w, "No token provided", http.StatusBadRequest)
		return
	}

	am.mu.Lock()
	delete(am.sessions, token)
	am.mu.Unlock()

	w.WriteHeader(http.StatusOK)
}

// extractToken extracts the token from the request
func (am *AuthMiddleware) extractToken(r *http.Request) string {
	// Check Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader != "" {
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}

	// Check query parameter
	return r.URL.Query().Get("token")
}

// isValidToken checks if a token is valid and not expired
func (am *AuthMiddleware) isValidToken(token string) bool {
	am.mu.Lock()
	defer am.mu.Unlock()

	session, exists := am.sessions[token]
	if !exists {
		return false
	}

	if time.Now().After(session.ExpiresAt) {
		delete(am.sessions, token)
		return false
	}

	return true
}

// Middleware is the authentication middleware function
func (am *AuthMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip authentication for login endpoint
		if strings.HasSuffix(r.URL.Path, "/auth/login") {
			next.ServeHTTP(w, r)
			return
		}

		token := am.extractToken(r)
		if token == "" || !am.isValidToken(token) {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// cleanupExpiredSessions removes expired sessions
func (am *AuthMiddleware) cleanupExpiredSessions() {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	for token, session := range am.sessions {
		if now.After(session.ExpiresAt) {
			delete(am.sessions, token)
		}
	}
}
