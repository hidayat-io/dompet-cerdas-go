// Package health provides the health check endpoint.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/db"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

// version is set at build time via -ldflags.
var version = "dev"

// Handler holds dependencies for the health check endpoint.
type Handler struct {
	firebase *db.Firebase
	env      string
}

// NewHandler creates a health check handler.
func NewHandler(fb *db.Firebase, env string) *Handler {
	return &Handler{firebase: fb, env: env}
}

// Register mounts the health check route on the given router group.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.GET("/health", h.Check)
}

// Check handles GET /api/v1/health.
// Returns 200 if Firestore is reachable, 503 otherwise (so load balancers can act).
func (h *Handler) Check(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	firestoreStatus := "connected"
	httpStatus := http.StatusOK

	if err := h.firebase.Ping(ctx); err != nil {
		firestoreStatus = "unreachable"
		httpStatus = http.StatusServiceUnavailable
	}

	data := gin.H{
		"status":    statusString(httpStatus),
		"timestamp": datetime.Now().Format(time.RFC3339),
		"version":   version,
		"env":       h.env,
		"firestore": firestoreStatus,
	}

	if httpStatus != http.StatusOK {
		c.JSON(httpStatus, response.Envelope{
			Success: false,
			Message: "Layanan tidak tersedia.",
			Data:    data,
			Error:   &response.ErrorBody{Code: "SERVICE_UNAVAILABLE"},
		})
		return
	}

	response.OK(c, "OK", data)
}

// statusString converts an HTTP status code to a human-readable status.
func statusString(code int) string {
	if code == http.StatusOK {
		return "healthy"
	}
	return "unhealthy"
}
