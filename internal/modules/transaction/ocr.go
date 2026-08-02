package transaction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/disintegration/imaging"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
)

// MaxReceiptBytes caps the decoded upload, matching the legacy scanReceipt
// callable. Anything larger is rejected before it reaches the decoder.
const MaxReceiptBytes = 5 * 1024 * 1024

// receiptMaxWidth and receiptJPEGQuality control the downscale applied before
// upload. ADR-003 replaced the Tesseract pipeline with Gemini Vision, so the
// preprocessing that existed to help OCR (grayscale, contrast normalisation) is
// gone; all that remains is making the image cheap to send.
const (
	receiptMaxWidth    = 1200
	receiptJPEGQuality = 80
)

// ErrReceiptTooLarge is returned for an oversized upload.
var ErrReceiptTooLarge = errors.New("receipt image exceeds the size limit")

// ReceiptAnalyzer is the vision leg of receipt scanning, kept as an interface so
// the resize path can be tested without calling the model.
type ReceiptAnalyzer interface {
	AnalyzeReceiptVision(ctx context.Context, imageBytes []byte, mimeType string) (string, error)
}

// CompressReceiptImage downscales an image to at most receiptMaxWidth and
// re-encodes it as JPEG.
//
// A decode failure is not fatal: the original bytes are returned so an exotic
// but valid format still reaches the model, which is what the legacy sharp path
// did when compression failed.
func CompressReceiptImage(raw []byte) ([]byte, string) {
	img, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return raw, "image/jpeg"
	}

	if img.Bounds().Dx() > receiptMaxWidth {
		img = imaging.Resize(img, receiptMaxWidth, 0, imaging.Lanczos)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: receiptJPEGQuality}); err != nil {
		return raw, "image/jpeg"
	}
	return buf.Bytes(), "image/jpeg"
}

// AnalyzeReceipt extracts structured data from a receipt photo, porting
// analyzeReceipt (geminiService.ts) onto the Gemini Vision path chosen in
// ADR-003.
//
// Defaults are filled in after parsing rather than trusted from the model: an
// empty date or currency would otherwise be written straight onto a transaction.
func AnalyzeReceipt(ctx context.Context, analyzer ReceiptAnalyzer, raw []byte) (domain.ReceiptData, error) {
	if len(raw) == 0 {
		return domain.ReceiptData{}, errors.New("empty image")
	}
	if len(raw) > MaxReceiptBytes {
		return domain.ReceiptData{}, ErrReceiptTooLarge
	}
	if analyzer == nil {
		return domain.ReceiptData{}, errors.New("receipt analysis is unavailable")
	}

	compressed, mimeType := CompressReceiptImage(raw)

	rawJSON, err := analyzer.AnalyzeReceiptVision(ctx, compressed, mimeType)
	if err != nil {
		return domain.ReceiptData{}, fmt.Errorf("analyze receipt: %w", err)
	}

	data, err := gemini.ParseJSONResult[domain.ReceiptData](rawJSON)
	if err != nil {
		return domain.ReceiptData{}, fmt.Errorf("analyze receipt: parse response: %w", err)
	}

	return NormalizeReceiptData(data), nil
}

// NormalizeReceiptData fills in the defaults the legacy handler relied on.
func NormalizeReceiptData(data domain.ReceiptData) domain.ReceiptData {
	if data.Date == "" {
		data.Date = datetime.TodayString()
	}
	if data.Currency == "" {
		data.Currency = "IDR"
	}
	if data.CategorySuggestion == "" {
		data.CategorySuggestion = "Lainnya"
	}
	if data.Confidence == "" {
		data.Confidence = domain.Confidence(gemini.ConfidenceLow)
	}
	// A zero total cannot be a usable receipt, whatever the model claimed, and
	// the caller has to be able to tell the user that plainly.
	if data.TotalAmount <= 0 {
		data.IsReceipt = false
	}
	return data
}
