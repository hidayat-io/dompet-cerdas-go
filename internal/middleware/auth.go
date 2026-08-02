// Package middleware provides Gin middleware for auth, CORS, logging, and recovery.
package middleware

import (
	"log/slog"
	"net/http"
	"strings"

	"firebase.google.com/go/v4/auth"
	"github.com/gin-gonic/gin"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

const (
	// contextKeyUserID is the Gin context key for the authenticated user's Firebase UID.
	contextKeyUserID = "userId"
	// contextKeyUserEmail is the Gin context key for the authenticated user's email.
	contextKeyUserEmail = "userEmail"
	// contextKeyUserName is the Gin context key for the authenticated user's display name.
	contextKeyUserName = "userName"
)

// Auth returns a Gin middleware that verifies Firebase ID Tokens from the
// Authorization header. CRITICAL: the auth.Client is created ONCE at startup
// and injected here — we do NOT call app.Auth(ctx) per request.
func Auth(authClient *auth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			response.Unauthorized(c, "Token autentikasi diperlukan.")
			c.Abort()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			response.Unauthorized(c, "Format token tidak valid. Gunakan: Bearer <token>")
			c.Abort()
			return
		}

		token, err := authClient.VerifyIDToken(c.Request.Context(), parts[1])
		if err != nil {
			slog.Warn("auth: invalid token", "error", err)
			response.Unauthorized(c, "Token tidak valid atau sudah kedaluwarsa.")
			c.Abort()
			return
		}

		c.Set(contextKeyUserID, token.UID)
		if email, ok := token.Claims["email"].(string); ok {
			c.Set(contextKeyUserEmail, email)
		}
		if name, ok := token.Claims["name"].(string); ok {
			c.Set(contextKeyUserName, name)
		}

		c.Next()
	}
}

// UserID extracts the authenticated user's Firebase UID from the Gin context.
// Returns ("", false) if the auth middleware has not run or the value is missing.
func UserID(c *gin.Context) (string, bool) {
	uid, exists := c.Get(contextKeyUserID)
	if !exists {
		return "", false
	}
	s, ok := uid.(string)
	return s, ok && s != ""
}

// UserEmail extracts the authenticated user's email from the Gin context.
// Returns ("", false) if not available.
func UserEmail(c *gin.Context) (string, bool) {
	email, exists := c.Get(contextKeyUserEmail)
	if !exists {
		return "", false
	}
	s, ok := email.(string)
	return s, ok && s != ""
}

// UserName extracts the authenticated user's display name from the Gin context.
func UserName(c *gin.Context) (string, bool) {
	name, exists := c.Get(contextKeyUserName)
	if !exists {
		return "", false
	}
	s, ok := name.(string)
	return s, ok && s != ""
}

// RequireAuth is a convenience shorthand that extracts the user ID or aborts
// with 401. Intended for use inside handlers, not as middleware.
func RequireAuth(c *gin.Context) (string, bool) {
	uid, ok := UserID(c)
	if !ok {
		response.Unauthorized(c, "Autentikasi diperlukan.")
		c.Abort()
		return "", false
	}
	return uid, true
}

// OptionalAuth is similar to Auth but does NOT abort if no token is present.
// If a valid token is provided, user info is set in context. If no token or
// invalid token, the request proceeds without authentication context.
func OptionalAuth(authClient *auth.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.Next()
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			c.Next()
			return
		}

		token, err := authClient.VerifyIDToken(c.Request.Context(), parts[1])
		if err != nil {
			c.Next()
			return
		}

		c.Set(contextKeyUserID, token.UID)
		if email, ok := token.Claims["email"].(string); ok {
			c.Set(contextKeyUserEmail, email)
		}
		if name, ok := token.Claims["name"].(string); ok {
			c.Set(contextKeyUserName, name)
		}

		c.Next()
	}
}

// HealthBypass skips middleware for health check endpoints.
func HealthBypass(next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.URL.Path == "/api/v1/health" && c.Request.Method == http.MethodGet {
			c.Next()
			return
		}
		next(c)
	}
}
