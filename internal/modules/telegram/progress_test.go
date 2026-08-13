package telegram

import (
	"context"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/modules/telegram/botapi"
)

// A progress notice must never break the handler: with an unconfigured bot the
// mark degrades to "nothing to clean up", and clearing that id is a no-op.
func TestProgressNotice_DegradesGracefully(t *testing.T) {
	h := &Handler{bot: botapi.New("")}
	ctx := context.Background()

	if id := h.markProgress(ctx, 123, "⏳"); id != 0 {
		t.Errorf("markProgress with an unconfigured bot = %d, want 0", id)
	}

	h.clearProgress(ctx, 123, 0)
}
