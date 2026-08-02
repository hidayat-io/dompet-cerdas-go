package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"google.golang.org/genai"
)

const ModelFlash = "gemini-2.5-flash"

// Confidence levels returned by the classifiers, mirroring the legacy union type.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
	ConfidenceLow    = "low"
)

type Client struct {
	client *genai.Client
}

func NewClient(ctx context.Context, apiKey string) (*Client, error) {
	if apiKey == "" {
		return nil, errors.New("gemini api key is required")
	}
	client, err := genai.NewClient(ctx, &genai.ClientConfig{APIKey: apiKey})
	if err != nil {
		return nil, err
	}
	return &Client{client: client}, nil
}

type Usage struct {
	PromptTokens    int
	CandidateTokens int
	TotalTokens     int
}

func executeWithRetry[T any](ctx context.Context, maxRetries int, fn func() (T, error)) (T, error) {
	var result T
	var err error
	for i := 0; i <= maxRetries; i++ {
		result, err = fn()
		if err == nil {
			return result, nil
		}
		if i < maxRetries {
			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(time.Duration(i+1) * time.Second):
			}
		}
	}
	return result, err
}

func (c *Client) TranscribeAudio(ctx context.Context, audioBytes []byte, mimeType string) (string, error) {
	prompt := `Transkripsikan audio bahasa Indonesia ini menjadi teks singkat yang mudah diparse untuk catatan transaksi.
Aturan:
- Return teks biasa saja, tanpa markdown, tanpa penjelasan tambahan.
- Jika ada beberapa transaksi, pisahkan dengan koma.
- Pertahankan nominal agar mudah dipahami, misalnya: 25rb, 10 ribu, 2,5 juta.
- Jika audio terlalu tidak jelas untuk ditranskripsikan, return string kosong.`

	return executeWithRetry(ctx, 2, func() (string, error) {
		part := genai.NewPartFromBytes(audioBytes, mimeType)
		textPart := genai.NewPartFromText(prompt)
		content := genai.NewContentFromParts([]*genai.Part{part, textPart}, "user")

		resp, err := c.client.Models.GenerateContent(ctx, ModelFlash, []*genai.Content{content}, nil)
		if err != nil {
			return "", err
		}
		return cleanCodeBlocks(resp.Text()), nil
	})
}

func (c *Client) AnalyzeReceiptVision(ctx context.Context, imageBytes []byte, mimeType string) (string, error) {
	prompt := `Analyze this Indonesian receipt image and extract transaction information.

Return ONLY valid JSON (no markdown, no code blocks):
{
  "is_receipt": boolean (true if image looks like a receipt/invoice/bill, false otherwise),
  "merchant": "store/restaurant name",
  "totalAmount": number (final total only, not subtotals. Return 0 if not found),
  "date": "YYYY-MM-DD" (if found, else today's date),
  "items": ["item1", "item2"],
  "categorySuggestion": "Makanan | Belanja Harian | Transport | Kesehatan | Hiburan | Tagihan | Lainnya",
  "receiptType": "retail | restaurant | transport | bill | other",
  "confidence": "high | medium | low",
  "currency": "IDR",
  "notes": "rangkuman singkat transaksi dalam Bahasa Indonesia yang alami & manusiawi (misal: 'Belanja grosir sembako di Toko Abang')"
}

Rules:
1. Set "is_receipt" to false if the image does not look like a shopping receipt, invoice, or payment proof.
2. Extract ONLY the final total (Grand Total / Total Bayar / Total).
3. Ignore tax, service charge, subtotals individually.
4. If date not found, use today's date.
5. Suggest category based on merchant type and items.
6. Write "notes" ALWAYS in friendly, natural Indonesian language.
7. For poor quality images, mark confidence as "medium" or "low".
8. Return only the JSON object, no other text.`

	return executeWithRetry(ctx, 2, func() (string, error) {
		part := genai.NewPartFromBytes(imageBytes, mimeType)
		textPart := genai.NewPartFromText(prompt)
		content := genai.NewContentFromParts([]*genai.Part{part, textPart}, "user")

		resp, err := c.client.Models.GenerateContent(ctx, ModelFlash, []*genai.Content{content}, nil)
		if err != nil {
			return "", err
		}
		return cleanCodeBlocks(resp.Text()), nil
	})
}

type InsightResult struct {
	Text  string
	Usage Usage
}

func (c *Client) GenerateInsights(ctx context.Context, prompt string, systemInstruction string) (string, Usage, error) {
	res, err := executeWithRetry(ctx, 2, func() (InsightResult, error) {
		sysContent := genai.NewContentFromText(systemInstruction, "system")
		temp := float32(0.7)
		config := &genai.GenerateContentConfig{
			SystemInstruction: sysContent,
			Temperature:       &temp,
		}

		resp, err := c.client.Models.GenerateContent(ctx, ModelFlash, genai.Text(prompt), config)
		if err != nil {
			return InsightResult{}, err
		}

		usage := Usage{}
		if resp.UsageMetadata != nil {
			usage.PromptTokens = int(resp.UsageMetadata.PromptTokenCount)
			usage.CandidateTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			usage.TotalTokens = int(resp.UsageMetadata.TotalTokenCount)
		}

		return InsightResult{Text: resp.Text(), Usage: usage}, nil
	})
	return res.Text, res.Usage, err
}

func cleanCodeBlocks(s string) string {
	s = strings.ReplaceAll(s, "```json", "")
	s = strings.ReplaceAll(s, "```", "")
	return strings.TrimSpace(s)
}

func ParseJSONResult[T any](raw string) (T, error) {
	var result T
	clean := cleanCodeBlocks(raw)

	firstBrace := strings.Index(clean, "{")
	lastBrace := strings.LastIndex(clean, "}")
	if firstBrace != -1 && lastBrace > firstBrace {
		clean = clean[firstBrace : lastBrace+1]
	}

	err := json.Unmarshal([]byte(clean), &result)
	return result, err
}

// CategoryCandidate is one selectable category offered to the classifier.
type CategoryCandidate struct {
	ID   string
	Name string
	Type string
}

// CategoryChoice is the classifier's pick. Confidence drives whether the bot
// saves without asking, so a missing confidence must degrade, never default to
// "high".
type CategoryChoice struct {
	CategoryID   string `json:"categoryId"`
	CategoryName string `json:"categoryName"`
	Confidence   string `json:"confidence"`
}

// ClassifyCategory picks the best-fitting category for a transaction
// description, porting classifyCategory (nluService.ts:906).
//
// The prompt is reproduced verbatim, including the instruction to fall back to
// "Lainnya"/"Other": changing the wording changes which category real
// transactions land in.
func (c *Client) ClassifyCategory(ctx context.Context, description string, categories []CategoryCandidate) (CategoryChoice, error) {
	if len(categories) == 0 {
		return CategoryChoice{}, errors.New("no categories available for classification")
	}

	lines := make([]string, 0, len(categories))
	for _, cat := range categories {
		catType := cat.Type
		if catType == "" {
			catType = "UNKNOWN"
		}
		lines = append(lines, fmt.Sprintf("- %s | %s | %s", cat.ID, cat.Name, catType))
	}

	prompt := fmt.Sprintf(`Kamu adalah AI untuk memilih kategori transaksi.

Deskripsi transaksi: "%s"

Daftar kategori (id, name, type):
%s

Pilih satu kategori yang paling cocok.
Jika tidak ada yang cocok, pilih kategori bernama "Lainnya" atau "Other" jika tersedia.
Return ONLY valid JSON (no markdown, no code blocks):
{
  "categoryId": "string",
  "categoryName": "string",
  "confidence": "high | medium | low"
}`, description, strings.Join(lines, "\n"))

	raw, err := executeWithRetry(ctx, 2, func() (string, error) {
		resp, err := c.client.Models.GenerateContent(ctx, ModelFlash, genai.Text(prompt), nil)
		if err != nil {
			return "", err
		}
		return resp.Text(), nil
	})
	if err != nil {
		return CategoryChoice{}, err
	}

	choice, err := ParseJSONResult[CategoryChoice](raw)
	if err != nil {
		return CategoryChoice{}, fmt.Errorf("classifyCategory: parse response: %w", err)
	}
	if choice.CategoryID == "" || choice.CategoryName == "" {
		return CategoryChoice{}, errors.New("invalid category classification response")
	}

	// The legacy default. "medium" keeps the result usable for the draft preview
	// while staying below the bar auto-save requires.
	if choice.Confidence == "" {
		choice.Confidence = ConfidenceMedium
	}

	// A model that invents an id would write transactions against a category
	// that does not exist, so the pick is validated against the offered set.
	for _, cat := range categories {
		if cat.ID == choice.CategoryID {
			return choice, nil
		}
	}
	return CategoryChoice{}, fmt.Errorf("classifyCategory: model returned unknown category id %q", choice.CategoryID)
}
