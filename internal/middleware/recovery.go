package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

// Recovery returns a Gin middleware that recovers from panics, logs the
// stack trace, and returns a 500 JSON response instead of crashing the server.
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered",
					"error", err,
					"stack", string(debug.Stack()),
					"path", c.Request.URL.Path,
					"method", c.Request.Method,
				)

				// Avoid writing if headers were already sent.
				if !c.Writer.Written() {
					response.InternalError(c)
				}

				c.Abort()
			}
		}()

		c.Next()
	}
}

// NoRoute returns a handler for unmatched routes.
func NoRoute() gin.HandlerFunc {
	return func(c *gin.Context) {
		response.NotFound(c, "Endpoint tidak ditemukan.")
		c.Abort()
	}
}

// NoMethod returns a handler for disallowed HTTP methods.
func NoMethod() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, response.Envelope{
			Success: false,
			Message: "Metode HTTP tidak diizinkan.",
			Error:   &response.ErrorBody{Code: "METHOD_NOT_ALLOWED"},
		})
		c.Abort()
	}
}
