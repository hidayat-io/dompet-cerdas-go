package advisor

import (
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mthidayat/dompet-cerdas-go/internal/middleware"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

// Handler exposes the AI financial advisor endpoints.
type Handler struct {
	service *Service
}

// NewHandler constructs the advisor HTTP handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// Register mounts the advisor routes. The supplied router group must already
// have the Firebase auth middleware applied.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/advisor/analyze", h.Analyze)
}

// Analyze runs the requested financial analysis for the caller's account.
func (h *Handler) Analyze(c *gin.Context) {
	userID, ok := middleware.RequireAuth(c)
	if !ok {
		return
	}

	var req struct {
		Mode      string `json:"mode" binding:"required"`
		AccountID string `json:"accountId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Data tidak valid", "INVALID_REQUEST", err.Error())
		return
	}

	mode, err := ModeFromString(req.Mode)
	if err != nil {
		response.BadRequest(c, "Mode analisis tidak valid.", "INVALID_MODE",
			gin.H{"allowed": []string{string(ModeHealth), string(ModeSpending), string(ModeSavings)}})
		return
	}

	result, err := h.service.Analyze(c.Request.Context(), userID, req.AccountID, mode)
	if err != nil {
		switch {
		case errors.Is(err, ErrUnavailable):
			response.FailedPrecondition(c, "Analisa AI sedang tidak tersedia.", "AI_UNAVAILABLE")
		case errors.Is(err, ErrQuotaExceeded):
			response.FailedPrecondition(c, err.Error(), "QUOTA_EXCEEDED")
		case errors.Is(err, ErrCooldown):
			response.FailedPrecondition(c, err.Error(), "COOLDOWN")
		default:
			slog.Error("advisor: analysis failed", "userId", userID, "mode", mode, "error", err)
			response.InternalErrorWithMessage(c, "Gagal menganalisis data keuangan. Coba lagi sebentar.")
		}
		return
	}

	response.OK(c, "Analisa berhasil", result)
}
