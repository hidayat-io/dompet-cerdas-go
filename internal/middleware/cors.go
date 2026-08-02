package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS returns a Gin middleware that handles Cross-Origin Resource Sharing.
// It uses an allowlist of origins from config — never a wildcard "*" when
// credentials/auth headers are involved (browsers reject that combination).
//
// Requests whose Origin is not allowlisted receive no CORS headers, so the
// browser blocks the response. A preflight for such an origin is answered 403
// rather than 204, so a misconfigured allowlist surfaces as an explicit
// rejection instead of an opaque CORS failure.
//
// Vary: Origin is always set, otherwise a shared cache could serve a response
// carrying one origin's Access-Control-Allow-Origin to a different origin.
func CORS(allowedOrigins []string) gin.HandlerFunc {
	originSet := make(map[string]struct{}, len(allowedOrigins))
	for _, o := range allowedOrigins {
		originSet[strings.TrimRight(o, "/")] = struct{}{}
	}

	return func(c *gin.Context) {
		c.Header("Vary", "Origin")

		origin := c.GetHeader("Origin")
		_, allowed := originSet[strings.TrimRight(origin, "/")]

		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Credentials", "true")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			c.Header("Access-Control-Max-Age", "86400")
		}

		if c.Request.Method == http.MethodOptions {
			if !allowed {
				c.AbortWithStatus(http.StatusForbidden)
				return
			}
			c.AbortWithStatus(http.StatusNoContent)
			return
		}

		c.Next()
	}
}
