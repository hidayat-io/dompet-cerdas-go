package transaction

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/datetime"
)

func testImage(t *testing.T, w, h int, encodePNG bool) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 128, A: 255})
		}
	}

	var buf bytes.Buffer
	var err error
	if encodePNG {
		err = png.Encode(&buf, img)
	} else {
		err = jpeg.Encode(&buf, img, nil)
	}
	if err != nil {
		t.Fatalf("encode test image: %v", err)
	}
	return buf.Bytes()
}

func TestCompressReceiptImage_DownscalesWideImages(t *testing.T) {
	raw := testImage(t, 3000, 1500, false)

	out, mime := CompressReceiptImage(raw)
	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", mime)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if cfg.Width != receiptMaxWidth {
		t.Errorf("width = %d, want %d", cfg.Width, receiptMaxWidth)
	}
	if len(out) >= len(raw) {
		t.Errorf("output (%d bytes) should be smaller than input (%d bytes)", len(out), len(raw))
	}
}

func TestCompressReceiptImage_LeavesSmallImagesAtSize(t *testing.T) {
	raw := testImage(t, 800, 600, false)

	out, _ := CompressReceiptImage(raw)

	cfg, _, err := image.DecodeConfig(bytes.NewReader(out))
	if err != nil {
		t.Fatalf("decode output: %v", err)
	}
	if cfg.Width != 800 {
		t.Errorf("width = %d, want the original 800 — no upscaling", cfg.Width)
	}
}

// PNG uploads must be converted, not passed through, or the declared mime type
// would not match the bytes.
func TestCompressReceiptImage_ConvertsPNGToJPEG(t *testing.T) {
	out, mime := CompressReceiptImage(testImage(t, 1600, 900, true))

	if mime != "image/jpeg" {
		t.Errorf("mime = %q, want image/jpeg", mime)
	}
	if _, err := jpeg.Decode(bytes.NewReader(out)); err != nil {
		t.Errorf("output is not decodable as JPEG: %v", err)
	}
}

// An undecodable payload is forwarded rather than rejected, matching the legacy
// behavior when compression failed.
func TestCompressReceiptImage_UndecodablePassesThrough(t *testing.T) {
	raw := []byte("not an image at all")

	out, _ := CompressReceiptImage(raw)
	if !bytes.Equal(out, raw) {
		t.Error("undecodable input should be forwarded unchanged")
	}
}

type stubAnalyzer struct {
	response string
	err      error
	gotBytes []byte
}

func (s *stubAnalyzer) AnalyzeReceiptVision(_ context.Context, imageBytes []byte, _ string) (string, error) {
	s.gotBytes = imageBytes
	return s.response, s.err
}

func TestAnalyzeReceipt_ParsesAndNormalizes(t *testing.T) {
	stub := &stubAnalyzer{response: `{"merchant":"Indomaret","totalAmount":45000,"is_receipt":true}`}

	got, err := AnalyzeReceipt(context.Background(), stub, testImage(t, 400, 400, false))
	if err != nil {
		t.Fatalf("AnalyzeReceipt: %v", err)
	}
	if got.Merchant != "Indomaret" || got.TotalAmount != 45000 {
		t.Errorf("data = %+v", got)
	}
	if got.Date != datetime.TodayString() {
		t.Errorf("date = %q, want today's date as the default", got.Date)
	}
	if got.Currency != "IDR" {
		t.Errorf("currency = %q, want IDR", got.Currency)
	}
}

// A model that reports a receipt with no total is not usable; the caller has to
// see is_receipt=false so it can say so.
func TestAnalyzeReceipt_ZeroTotalIsNotAReceipt(t *testing.T) {
	stub := &stubAnalyzer{response: `{"merchant":"Kucing","totalAmount":0,"is_receipt":true}`}

	got, err := AnalyzeReceipt(context.Background(), stub, testImage(t, 200, 200, false))
	if err != nil {
		t.Fatalf("AnalyzeReceipt: %v", err)
	}
	if got.IsReceipt {
		t.Error("a zero total must force is_receipt=false")
	}
}

func TestAnalyzeReceipt_RejectsOversizedUpload(t *testing.T) {
	_, err := AnalyzeReceipt(context.Background(), &stubAnalyzer{}, make([]byte, MaxReceiptBytes+1))
	if !errors.Is(err, ErrReceiptTooLarge) {
		t.Errorf("err = %v, want ErrReceiptTooLarge", err)
	}
}

func TestAnalyzeReceipt_EmptyAndNilAnalyzerAreErrors(t *testing.T) {
	if _, err := AnalyzeReceipt(context.Background(), &stubAnalyzer{}, nil); err == nil {
		t.Error("empty image should be an error")
	}
	if _, err := AnalyzeReceipt(context.Background(), nil, testImage(t, 100, 100, false)); err == nil {
		t.Error("a nil analyzer should be an error, not a panic")
	}
}

func TestAnalyzeReceipt_MalformedJSONIsAnError(t *testing.T) {
	stub := &stubAnalyzer{response: "maaf saya tidak bisa membaca gambar ini"}

	if _, err := AnalyzeReceipt(context.Background(), stub, testImage(t, 100, 100, false)); err == nil {
		t.Error("a non-JSON response should be an error, not empty receipt data")
	}
}

// The model must receive the compressed bytes, not the original.
func TestAnalyzeReceipt_SendsCompressedBytes(t *testing.T) {
	raw := testImage(t, 2400, 1200, false)
	stub := &stubAnalyzer{response: `{"merchant":"X","totalAmount":1000,"is_receipt":true}`}

	if _, err := AnalyzeReceipt(context.Background(), stub, raw); err != nil {
		t.Fatalf("AnalyzeReceipt: %v", err)
	}

	cfg, _, err := image.DecodeConfig(bytes.NewReader(stub.gotBytes))
	if err != nil {
		t.Fatalf("decode forwarded bytes: %v", err)
	}
	if cfg.Width != receiptMaxWidth {
		t.Errorf("forwarded width = %d, want the compressed %d", cfg.Width, receiptMaxWidth)
	}
}

func TestNormalizeReceiptData_KeepsProvidedValues(t *testing.T) {
	got := NormalizeReceiptData(domain.ReceiptData{
		Date: "2026-01-05", Currency: "USD", CategorySuggestion: "Makanan",
		Confidence: "high", TotalAmount: 100, IsReceipt: true,
	})

	if got.Date != "2026-01-05" || got.Currency != "USD" || got.CategorySuggestion != "Makanan" {
		t.Errorf("normalization overwrote provided values: %+v", got)
	}
	if !got.IsReceipt {
		t.Error("a positive total must stay a receipt")
	}
}
