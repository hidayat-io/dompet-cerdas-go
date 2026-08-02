package telegram

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/mthidayat/dompet-cerdas-go/internal/shared/response"
)

// HandleWebhook accepts a Telegram update and claims it exactly once.
func (h *Handler) HandleWebhook(c *gin.Context) {
	if h.webhookSecret != "" && c.Query("token") != h.webhookSecret {
		response.Unauthorized(c, "Token webhook tidak valid.")
		return
	}

	var update map[string]interface{}
	if err := c.ShouldBindJSON(&update); err != nil {
		slog.Warn("telegram webhook: malformed payload", "error", err)
		c.String(http.StatusOK, "OK")
		return
	}

	updateID, ok := extractUpdateID(update)
	if !ok {
		slog.Warn("telegram webhook: missing update_id")
		c.String(http.StatusOK, "OK")
		return
	}

	docRef := h.db.Collection("telegram_processed_updates").Doc(strconv.FormatInt(updateID, 10))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	_, err := docRef.Create(ctx, map[string]interface{}{
		"updateId":  updateID,
		"status":    "processing",
		"createdAt": time.Now(),
	})
	if err != nil {
		if status.Code(err) == codes.AlreadyExists {
			slog.Info("telegram webhook: duplicate update ignored", "updateId", updateID)
			c.String(http.StatusOK, "OK")
			return
		}
		slog.Error("telegram webhook: failed to claim update", "updateId", updateID, "error", err)
		c.String(http.StatusOK, "OK")
		return
	}

	// Route message asynchronously
	go func(up map[string]interface{}, ref *firestore.DocumentRef) {
		bgCtx := context.Background()
		err := h.ProcessBotUpdate(bgCtx, up)

		statusStr := "processed"
		errMsg := ""
		if err != nil {
			statusStr = "failed"
			errMsg = err.Error()
			slog.Error("telegram bot processing failed", "updateId", updateID, "error", err)
		}

		updateData := map[string]interface{}{
			"status":      statusStr,
			"processedAt": time.Now(),
		}
		if errMsg != "" {
			updateData["error"] = errMsg
		}

		markCtx, markCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer markCancel()
		_, _ = ref.Set(markCtx, updateData, firestore.MergeAll)
	}(update, docRef)

	c.String(http.StatusOK, "OK")
}

func extractUpdateID(update map[string]interface{}) (int64, bool) {
	raw, exists := update["update_id"]
	if !exists {
		return 0, false
	}
	switch v := raw.(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case int:
		return int64(v), true
	}
	return 0, false
}
