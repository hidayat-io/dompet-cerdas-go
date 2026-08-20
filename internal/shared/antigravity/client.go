package antigravity

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/mthidayat/dompet-cerdas-go/internal/domain"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/gemini"
	"github.com/mthidayat/dompet-cerdas-go/internal/shared/money"
)

const defaultModel = "gemini-3.6-flash-high"

// Client talks to an OpenAI-compatible endpoint (Antigravity proxy) that
// forwards to Gemini via OAuth. It mirrors the gemini.Client surface so the
// rest of the app can swap implementations.
type Client struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(baseURL, apiKey, model string) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, errors.New("antigravity api key is required")
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "http://127.0.0.1:3000/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if strings.TrimSpace(model) == "" {
		model = defaultModel
	}
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}, nil
}

// IsQuotaExceeded matches Antigravity / Gemini quota errors.
func IsQuotaExceeded(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "resource_exhausted") {
		return true
	}
	if strings.Contains(s, "spending cap") {
		return true
	}
	if strings.Contains(s, "quota") && strings.Contains(s, "exceeded") {
		return true
	}
	if strings.Contains(s, "429") && strings.Contains(s, "exceeded") {
		return true
	}
	return false
}

func (c *Client) chatCompletion(ctx context.Context, messages []map[string]interface{}, temperature *float32, maxTokens *int) (string, gemini.Usage, error) {
	body := map[string]interface{}{
		"model":    c.model,
		"messages": messages,
	}
	if temperature != nil {
		body["temperature"] = *temperature
	}
	if maxTokens != nil {
		body["max_tokens"] = *maxTokens
	}
	b, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", gemini.Usage{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", gemini.Usage{}, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", gemini.Usage{}, fmt.Errorf("antigravity %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", gemini.Usage{}, fmt.Errorf("antigravity: decode response: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", gemini.Usage{}, errors.New("antigravity: empty choices")
	}
	usage := gemini.Usage{
		PromptTokens:    out.Usage.PromptTokens,
		CandidateTokens: out.Usage.CompletionTokens,
		TotalTokens:     out.Usage.TotalTokens,
	}
	return cleanCodeBlocks(out.Choices[0].Message.Content), usage, nil
}

func cleanCodeBlocks(s string) string {
	s = strings.ReplaceAll(s, "```json", "")
	s = strings.ReplaceAll(s, "```", "")
	return strings.TrimSpace(s)
}

// AnalyzeReceiptVision sends a receipt image (already compressed) to the vision model.
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
  "confidenceScore": integer 0-100 (how confident you are in the extraction),
  "currency": "IDR",
  "notes": "label transaksi dalam Bahasa Indonesia, maksimal 6 kata, tanpa nominal (misal: 'Pembelian emas', 'Paket sei berdua di SeIndonesia')"
}

Rules:
1. Set "is_receipt" to false if the image does not look like a shopping receipt, invoice, or payment proof.
2. Extract ONLY the final total (Grand Total / Total Bayar / Total).
3. Ignore tax, service charge, subtotals individually.
4. If date not found, use today's date.
5. Suggest category based on merchant type and items.
6. Write "notes" as a short transaction label (max 6 words) describing WHAT was bought and WHERE, e.g. "Pembelian emas" or "Belanja mingguan di Indomaret". NEVER include the amount (it is stored separately) and NEVER copy on-screen confirmation wording such as "berhasil", "sukses", or "pembayaran diterima".
7. For poor quality images, mark confidence as "medium" or "low".
8. Set "confidenceScore" to a bare integer between 0 and 100, consistent with "confidence". Use a value above 90 only when the total amount and merchant are clearly legible and unambiguous.
9. Return only the JSON object, no other text.`

	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(imageBytes))
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []map[string]interface{}{
				{"type": "text", "text": prompt},
				{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
			},
		},
	}
	text, _, err := c.chatCompletion(ctx, messages, nil, nil)
	return text, err
}

func (c *Client) TranscribeAudio(ctx context.Context, audioBytes []byte, mimeType string) (string, error) {
	prompt := `Transkripsikan audio bahasa Indonesia ini menjadi teks singkat yang mudah diparse untuk catatan transaksi.
Aturan:
- Return teks biasa saja, tanpa markdown, tanpa penjelasan tambahan.
- Jika ada beberapa transaksi, pisahkan dengan koma.
- Pertahankan nominal agar mudah dipahami, misalnya: 25rb, 10 ribu, 2,5 juta.
- Jika audio terlalu tidak jelas untuk ditranskripsikan, return string kosong.`
	// Antigravity proxy only handles image_url as inlineData, so we send audio as data URL with image_url type
	// It will be forwarded as inlineData with audio mime, which Gemini understands.
	dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(audioBytes))
	messages := []map[string]interface{}{
		{
			"role": "user",
			"content": []map[string]interface{}{
				{"type": "text", "text": prompt},
				{"type": "image_url", "image_url": map[string]string{"url": dataURL}},
			},
		},
	}
	text, _, err := c.chatCompletion(ctx, messages, nil, nil)
	return cleanCodeBlocks(text), err
}

func (c *Client) GenerateInsights(ctx context.Context, prompt string, systemInstruction string) (string, gemini.Usage, error) {
	temp := float32(0.7)
	messages := []map[string]interface{}{}
	if strings.TrimSpace(systemInstruction) != "" {
		messages = append(messages, map[string]interface{}{"role": "system", "content": systemInstruction})
	}
	messages = append(messages, map[string]interface{}{"role": "user", "content": prompt})
	return c.chatCompletion(ctx, messages, &temp, nil)
}

// Category helpers mirrored from gemini
var (
	leakedCurrencyRegex = regexp.MustCompile(`(?i)[\s.,;]?rp[\s.,]?\d[\d.,]*(?:k|rb|ribu|jt|juta|m|milyar|miliar)?\.?|[\s.,;]?idr\s*\d[\d.,]*\.?`)
	indonesianDateRegex = regexp.MustCompile(`(?i)(?:pada\s+tanggal\s+|tgl\s+|tanggal\s+)\d{1,2}\s+(?:jan(?:uary)?|feb(?:ruary)?|mar(?:ch)?|apr(?:il)?|mei|jun(?:e)?|jul(?:y)?|agu(?:stus)?|sep(?:tember)?|okt(?:ober)?|nov(?:ember)?|des(?:ember)?)\s+\d{4}\.?(?:\s+\d{2}:\d{2})?`)
)

func cleanDescription(s string) string {
	s = leakedCurrencyRegex.ReplaceAllString(s, " ")
	s = indonesianDateRegex.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return strings.TrimRight(s, ".,;:")
}

func (c *Client) ClassifyCategory(ctx context.Context, description string, categories []gemini.CategoryCandidate) (gemini.CategoryChoice, error) {
	if len(categories) == 0 {
		return gemini.CategoryChoice{}, errors.New("no categories available for classification")
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

	messages := []map[string]interface{}{{"role": "user", "content": prompt}}
	raw, _, err := c.chatCompletion(ctx, messages, nil, nil)
	if err != nil {
		return gemini.CategoryChoice{}, err
	}
	choice, err := gemini.ParseJSONResult[gemini.CategoryChoice](cleanCodeBlocks(raw))
	if err != nil {
		return gemini.CategoryChoice{}, fmt.Errorf("classifyCategory: parse response: %w", err)
	}
	if choice.CategoryID == "" || choice.CategoryName == "" {
		return gemini.CategoryChoice{}, errors.New("invalid category classification response")
	}
	if choice.Confidence == "" {
		choice.Confidence = "medium"
	}
	for _, cat := range categories {
		if cat.ID == choice.CategoryID {
			return choice, nil
		}
	}
	return gemini.CategoryChoice{}, fmt.Errorf("classifyCategory: model returned unknown category id %q", choice.CategoryID)
}

// ParseTransaction for free-form text
func (c *Client) ParseTransaction(ctx context.Context, message string) (*domain.HybridTransactionParseResult, error) {
	prompt := fmt.Sprintf(`Kamu adalah parser transaksi keuangan berbahasa Indonesia.

Pesan pengguna:
%q

Ekstrak hanya transaksi yang benar-benar tertulis di pesan.
- Dukung satu atau beberapa transaksi.
- Nominal bisa ditulis seperti 6000, 6k, 25rb, 10 ribu, 1,5 juta, atau Rp 50.000.
- Jangan anggap ukuran produk seperti 1.5L atau 600ml sebagai nominal.
- Description harus berupa nama/keterangan transaksi, tanpa nominal.
- category_hint boleh dikosongkan jika tidak yakin.
- sourceText harus berisi potongan pesan asal untuk item tersebut.
- Jika nominal atau transaksi tidak jelas, kembalikan items kosong dan isi clarificationNeeded.

Return ONLY valid JSON tanpa markdown:
{
  "items": [
    {"amount": 6000, "description": "air minum", "category_hint": "Food", "sourceText": "6k air minum"}
  ],
  "confidence": "high | medium | low",
  "clarificationNeeded": ""
}`, message)

	messages := []map[string]interface{}{{"role": "user", "content": prompt}}
	raw, _, err := c.chatCompletion(ctx, messages, nil, nil)
	if err != nil {
		return nil, err
	}
	// Reuse gemini's parsing logic via helper
	return parseTransactionResponse(raw, message)
}

// Helpers duplicated from gemini to avoid private access
type transactionAmount struct {
	Int    *int64 `json:"-"`
	String string `json:"-"`
}

func (a *transactionAmount) UnmarshalJSON(data []byte) error {
	var number int64
	if err := json.Unmarshal(data, &number); err == nil {
		a.Int = &number
		return nil
	}
	return json.Unmarshal(data, &a.String)
}

type transactionParseItem struct {
	Amount       transactionAmount `json:"amount"`
	Description  string            `json:"description"`
	CategoryHint string            `json:"category_hint"`
	SourceText   string            `json:"sourceText"`
}

type transactionParseResponse struct {
	Items               []transactionParseItem `json:"items"`
	Confidence          string                 `json:"confidence"`
	ClarificationNeeded string                 `json:"clarificationNeeded"`
}

func parseTransactionResponse(raw, message string) (*domain.HybridTransactionParseResult, error) {
	// Use gemini's generic parser then normalize
	resp, err := gemini.ParseJSONResult[transactionParseResponse](cleanCodeBlocks(raw))
	if err != nil {
		return nil, fmt.Errorf("parseTransaction: parse response: %w", err)
	}
	if len(resp.Items) == 0 {
		return nil, errors.New("parseTransaction: no transaction items")
	}
	if len(resp.Items) > 20 {
		return nil, errors.New("parseTransaction: too many transaction items")
	}
	items := make([]domain.ParsedTransactionDraft, 0, len(resp.Items))
	for _, item := range resp.Items {
		amount, err := parseTransactionAmount(item.Amount)
		if err != nil {
			return nil, fmt.Errorf("parseTransaction: invalid amount: %w", err)
		}
		description := cleanDescription(item.Description)
		if amount <= 0 || description == "" {
			return nil, errors.New("parseTransaction: invalid transaction item")
		}
		sourceText := strings.TrimSpace(item.SourceText)
		if sourceText == "" {
			sourceText = message
		}
		items = append(items, domain.ParsedTransactionDraft{
			Amount:       amount,
			Description:  description,
			CategoryHint: strings.TrimSpace(item.CategoryHint),
			SourceText:   sourceText,
		})
	}
	confidence := domain.ConfidenceMedium
	switch resp.Confidence {
	case "high":
		confidence = domain.ConfidenceHigh
	case "low":
		confidence = domain.ConfidenceLow
	}
	return &domain.HybridTransactionParseResult{
		Items:               items,
		UsedAI:              true,
		Confidence:          confidence,
		ClarificationNeeded: strings.TrimSpace(resp.ClarificationNeeded),
	}, nil
}

func parseTransactionAmount(amount transactionAmount) (int64, error) {
	if amount.Int != nil {
		return *amount.Int, nil
	}
	if strings.TrimSpace(amount.String) == "" {
		return 0, errors.New("missing amount")
	}
	parsed, ok := money.ParseAmount(amount.String)
	if !ok {
		return 0, fmt.Errorf("cannot parse %q", amount.String)
	}
	return parsed, nil
}
