package botapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a minimal Telegram Bot API HTTP client.
type Client struct {
	token      string
	httpClient *http.Client
	baseURL    string
}

// New creates a bot client.
func New(token string) *Client {
	return &Client{
		token: token,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		baseURL: "https://api.telegram.org",
	}
}

// SendMessage sends a text message. parseMode may be "Markdown" or empty.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text, parseMode string) error {
	if c == nil || c.token == "" {
		return fmt.Errorf("telegram bot token not configured")
	}
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	return c.post(ctx, "sendMessage", payload, nil)
}

// AnswerCallbackQuery answers an inline keyboard callback.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID, text string) error {
	payload := map[string]interface{}{
		"callback_query_id": callbackQueryID,
	}
	if text != "" {
		payload["text"] = text
	}
	return c.post(ctx, "answerCallbackQuery", payload, nil)
}

// EditMessageText edits an existing message.
func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text, parseMode string, replyMarkup interface{}) error {
	payload := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if replyMarkup != nil {
		payload["reply_markup"] = replyMarkup
	}
	return c.post(ctx, "editMessageText", payload, nil)
}

// GetFilePath resolves a Telegram file_id to a downloadable path.
func (c *Client) GetFilePath(ctx context.Context, fileID string) (string, error) {
	var result struct {
		FilePath string `json:"file_path"`
	}
	if err := c.post(ctx, "getFile", map[string]interface{}{"file_id": fileID}, &result); err != nil {
		return "", err
	}
	return result.FilePath, nil
}

// DownloadFile downloads a file from Telegram servers.
func (c *Client) DownloadFile(ctx context.Context, filePath string) ([]byte, error) {
	url := fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, c.token, filePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("telegram download %d: %s", resp.StatusCode, string(body))
	}
	return io.ReadAll(resp.Body)
}

// SendMessageWithKeyboard sends a message with an inline keyboard.
func (c *Client) SendMessageWithKeyboard(ctx context.Context, chatID int64, text, parseMode string, keyboard interface{}) (int, error) {
	payload := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	if keyboard != nil {
		payload["reply_markup"] = keyboard
	}
	var result struct {
		MessageID int `json:"message_id"`
	}
	if err := c.post(ctx, "sendMessage", payload, &result); err != nil {
		return 0, err
	}
	return result.MessageID, nil
}

func (c *Client) post(ctx context.Context, method string, payload map[string]interface{}, out interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}

	var envelope struct {
		OK          bool            `json:"ok"`
		Description string          `json:"description"`
		Result      json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("telegram %s: decode: %w", method, err)
	}
	if !envelope.OK {
		return fmt.Errorf("telegram %s: %s", method, envelope.Description)
	}
	if out != nil && len(envelope.Result) > 0 {
		return json.Unmarshal(envelope.Result, out)
	}
	return nil
}
