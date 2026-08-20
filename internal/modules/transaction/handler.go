package transaction

import (
	"encoding/base64"
	"errors"
	"log/slog"

	"github.com/gin-gonic/gin"

	"github.com/mthidayat/dompet-cerdas-go/internal/middleware"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

// Handler exposes the transaction endpoints used by the web app.
type Handler struct {
	analyzer ReceiptAnalyzer
}

func NewHandler(analyzer ReceiptAnalyzer) *Handler {
	return &Handler{analyzer: analyzer}
}

// Register mounts the routes. The group must already carry the auth middleware.
func (h *Handler) Register(rg *gin.RouterGroup) {
	rg.POST("/transactions/scan-receipt", h.ScanReceipt)
}

// ScanReceipt extracts structured data from an uploaded receipt photo, replacing
// the scanReceipt Firebase callable.
//
// Nothing is written here: the endpoint only reads the image and returns fields
// for the web form to prefill, so the user still confirms before anything is
// saved.
func (h *Handler) ScanReceipt(c *gin.Context) {
	if _, ok := middleware.RequireAuth(c); !ok {
		return
	}

	var req struct {
		ImageBase64 string `json:"imageBase64" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.BadRequest(c, "Gambar struk wajib diisi.", "INVALID_REQUEST", err.Error())
		return
	}

	raw, err := base64.StdEncoding.DecodeString(req.ImageBase64)
	if err != nil {
		response.BadRequest(c, "Format gambar tidak valid.", "INVALID_IMAGE", nil)
		return
	}

	data, err := AnalyzeReceipt(c.Request.Context(), h.analyzer, raw)
	if err != nil {
		if errors.Is(err, ErrReceiptTooLarge) {
			response.BadRequest(c, "Ukuran gambar melebihi 5MB.", "IMAGE_TOO_LARGE", nil)
			return
		}
		if errors.Is(err, ErrQuotaExceeded) {
			slog.Error("scan receipt failed: quota exceeded",
				"error", err,
				"bytes", len(raw),
			)
			response.Fail(c, 429, "Kuota AI sedang habis (Gemini spending cap). Silakan coba lagi nanti.", "QUOTA_EXCEEDED", nil)
			return
		}
		slog.Error("scan receipt failed",
			"error", err,
			"bytes", len(raw),
		)
		response.InternalErrorWithMessage(c, "Gagal membaca struk. Coba foto ulang dengan pencahayaan lebih baik.")
		return
	}

	response.OK(c, "Struk berhasil dibaca", data)
}
