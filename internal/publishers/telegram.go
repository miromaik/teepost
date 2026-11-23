package publishers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TelegramCredentials holds Bot API credentials.
type TelegramCredentials struct {
	BotToken string `json:"botToken"`
	ChatID   string `json:"chatId"`
}

// TelegramPublisher sends plain text, single photo, or media group messages via the Telegram Bot API.
type TelegramPublisher struct {
	creds  TelegramCredentials
	client *http.Client
}

// NewTelegramPublisher parses and validates Telegram credentials.
func NewTelegramPublisher(credentialsJSON string) (*TelegramPublisher, error) {
	var creds TelegramCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &creds); err != nil {
		return nil, fmt.Errorf("invalid Telegram credentials JSON: %w", err)
	}
	creds.BotToken = strings.TrimSpace(creds.BotToken)
	creds.ChatID = strings.TrimSpace(creds.ChatID)
	if creds.BotToken == "" || creds.ChatID == "" {
		return nil, errors.New("Telegram credentials require botToken and chatId")
	}
	return &TelegramPublisher{
		creds:  creds,
		client: &http.Client{Timeout: 35 * time.Second},
	}, nil
}

func (p *TelegramPublisher) apiURL(method string) string {
	return fmt.Sprintf("https://api.telegram.org/bot%s/%s", p.creds.BotToken, method)
}

// TestConnection validates the bot token via getMe and verifies access to the target chat via getChat.
func (p *TelegramPublisher) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.apiURL("getMe"), nil)
	if err != nil {
		return err
	}
	if _, err := doJSONOrError(p.client, req, "Telegram getMe"); err != nil {
		return err
	}

	chatEndpoint := p.apiURL("getChat") + "?chat_id=" + url.QueryEscape(p.creds.ChatID)
	chatReq, err := http.NewRequestWithContext(ctx, http.MethodGet, chatEndpoint, nil)
	if err != nil {
		return err
	}
	_, err = doJSONOrError(p.client, chatReq, "Telegram getChat")
	return err
}

// Publish sends text-only (sendMessage), single image (sendPhoto), or 2-4 images (sendMediaGroup).
func (p *TelegramPublisher) Publish(ctx context.Context, payload PostPayload) (PublishResult, error) {
	var res PublishResult
	var err error
	switch len(payload.Media) {
	case 0:
		res, err = p.sendMessage(ctx, payload.Text, 0)
	case 1:
		res, err = p.sendPhoto(ctx, payload.Text, payload.Media[0])
	default:
		res, err = p.sendMediaGroup(ctx, payload.Text, payload.Media)
	}
	if err != nil {
		return PublishResult{}, err
	}

	if fc := strings.TrimSpace(payload.FirstComment); fc != "" {
		replyID, _ := strconv.ParseInt(res.ExternalID, 10, 64)
		if _, fcErr := p.sendMessage(ctx, fc, replyID); fcErr != nil {
			return res, fcErr
		}
	}
	return res, nil
}

func (p *TelegramPublisher) sendMessage(ctx context.Context, text string, replyToMessageID int64) (PublishResult, error) {
	// Intentionally omit parse_mode so punctuation never triggers Markdown escaping errors.
	payloadMap := map[string]any{
		"chat_id": p.creds.ChatID,
		"text":    text,
	}
	if replyToMessageID > 0 {
		payloadMap["reply_to_message_id"] = replyToMessageID
	}
	bodyBytes, err := json.Marshal(payloadMap)
	if err != nil {
		return PublishResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL("sendMessage"), bytes.NewReader(bodyBytes))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	respBytes, err := doJSONOrError(p.client, req, "Telegram sendMessage")
	if err != nil {
		return PublishResult{}, err
	}
	return parseTelegramSingleMessage(respBytes)
}

func (p *TelegramPublisher) sendPhoto(ctx context.Context, caption string, item MediaAttachment) (PublishResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	_ = writer.WriteField("chat_id", p.creds.ChatID)
	if caption != "" {
		_ = writer.WriteField("caption", caption)
	}

	filename := item.Filename
	if filename == "" {
		filename = "photo.jpg"
	}
	part, err := writer.CreateFormFile("photo", filename)
	if err != nil {
		return PublishResult{}, err
	}
	if _, err := part.Write(item.Data); err != nil {
		return PublishResult{}, err
	}
	if err := writer.Close(); err != nil {
		return PublishResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL("sendPhoto"), &buf)
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	respBytes, err := doJSONOrError(p.client, req, "Telegram sendPhoto")
	if err != nil {
		return PublishResult{}, err
	}
	return parseTelegramSingleMessage(respBytes)
}

func (p *TelegramPublisher) sendMediaGroup(ctx context.Context, caption string, items []MediaAttachment) (PublishResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	_ = writer.WriteField("chat_id", p.creds.ChatID)

	mediaEntries := make([]map[string]any, 0, 4)
	for i, item := range items {
		if i >= 4 {
			break
		}
		fieldName := fmt.Sprintf("file%d", i)
		entry := map[string]any{
			"type":  "photo",
			"media": "attach://" + fieldName,
		}
		if i == 0 && caption != "" {
			entry["caption"] = caption
		}
		mediaEntries = append(mediaEntries, entry)

		filename := item.Filename
		if filename == "" {
			filename = fmt.Sprintf("photo_%d.jpg", i)
		}
		part, err := writer.CreateFormFile(fieldName, filename)
		if err != nil {
			return PublishResult{}, err
		}
		if _, err := part.Write(item.Data); err != nil {
			return PublishResult{}, err
		}
	}

	mediaJSON, err := json.Marshal(mediaEntries)
	if err != nil {
		return PublishResult{}, err
	}
	_ = writer.WriteField("media", string(mediaJSON))

	if err := writer.Close(); err != nil {
		return PublishResult{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.apiURL("sendMediaGroup"), &buf)
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	respBytes, err := doJSONOrError(p.client, req, "Telegram sendMediaGroup")
	if err != nil {
		return PublishResult{}, err
	}

	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      []struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				Username string `json:"username"`
			} `json:"chat"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return PublishResult{}, fmt.Errorf("parse Telegram sendMediaGroup response: %w", err)
	}
	if !parsed.OK {
		return PublishResult{}, fmt.Errorf("Telegram sendMediaGroup error: %s", parsed.Description)
	}
	if len(parsed.Result) == 0 {
		return PublishResult{}, nil
	}
	first := parsed.Result[0]
	return buildTelegramResult(first.MessageID, first.Chat.Username), nil
}

func parseTelegramSingleMessage(respBytes []byte) (PublishResult, error) {
	var parsed struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
		Result      struct {
			MessageID int64 `json:"message_id"`
			Chat      struct {
				Username string `json:"username"`
			} `json:"chat"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return PublishResult{}, fmt.Errorf("parse Telegram response: %w", err)
	}
	if !parsed.OK {
		return PublishResult{}, fmt.Errorf("Telegram API error: %s", parsed.Description)
	}
	return buildTelegramResult(parsed.Result.MessageID, parsed.Result.Chat.Username), nil
}

func buildTelegramResult(messageID int64, username string) PublishResult {
	idStr := ""
	if messageID > 0 {
		idStr = strconv.FormatInt(messageID, 10)
	}
	var externalURL string
	if username != "" && messageID > 0 {
		externalURL = fmt.Sprintf("https://t.me/%s/%d", username, messageID)
	}
	return PublishResult{
		ExternalID:  idStr,
		ExternalURL: externalURL,
	}
}
