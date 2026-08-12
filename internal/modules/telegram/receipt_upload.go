package telegram

import (
	"context"
	"fmt"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/account"
	"github.com/mthidayat/dompet-cerdas-go/internal/modules/transaction"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/storage"
)

// ReceiptUploader stores a receipt photo as a private transaction attachment.
// The handler treats a nil uploader as "attachments unavailable" and saves the
// transaction without one, so storage is optional.
type ReceiptUploader interface {
	UploadReceipt(ctx context.Context, ac *account.Context, image []byte) (domain.Attachment, error)
}

// receiptStoragePath mirrors the web app's getScopedStoragePath
// (accountService.ts): account-scoped paths are the ones storage.rules grants
// the owner read access to, which is what lets the web client resolve a
// display URL from the stored path.
func receiptStoragePath(ac *account.Context, fileName string) string {
	if ac.SharedAccountID != "" {
		return "sharedAccounts/" + ac.SharedAccountID + "/attachments/" + fileName
	}
	return "users/" + ac.UserID + "/accounts/" + ac.AccountID + "/attachments/" + fileName
}

type receiptUploader struct {
	store *storage.Client
}

// NewReceiptUploader builds the production uploader.
func NewReceiptUploader(store *storage.Client) ReceiptUploader {
	return receiptUploader{store: store}
}

// UploadReceipt compresses the photo and stores it privately. The returned
// attachment carries no URL: the web app resolves one from the path, so no
// long-lived or public URL is ever persisted (ADR-017).
func (u receiptUploader) UploadReceipt(ctx context.Context, ac *account.Context, image []byte) (domain.Attachment, error) {
	compressed, contentType := transaction.CompressReceiptImage(image)

	fileName := fmt.Sprintf("receipt_%d.jpg", time.Now().UnixMilli())
	path := receiptStoragePath(ac, fileName)

	if err := u.store.Upload(ctx, path, compressed, contentType); err != nil {
		return domain.Attachment{}, err
	}

	return domain.Attachment{
		Path: path,
		Type: domain.AttachmentTypeImage,
		Name: fileName,
		Size: int64(len(compressed)),
	}, nil
}
