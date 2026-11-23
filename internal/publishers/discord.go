package publishers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"strings"
	"time"
)

// DiscordCredentials holds the Discord webhook URL.
type DiscordCredentials struct {
	WebhookURL string `json:"webhookUrl"`
}

// DiscordPublisher sends messages and image attachments to a Discord webhook.
type DiscordPublisher struct {
	creds  DiscordCredentials
	client *http.Client
}

// NewDiscordPublisher validates the Discord webhook URL.
func NewDiscordPublisher(credentialsJSON string) (*DiscordPublisher, error) {
	var creds DiscordCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &creds); err != nil {
		return nil, fmt.Errorf("invalid Discord credentials JSON: %w", err)
	}
	creds.WebhookURL = strings.TrimSpace(creds.WebhookURL)
	if creds.WebhookURL == "" {
		return nil, errors.New("Discord credentials require webhookUrl")
	}
	return &DiscordPublisher{
		creds:  creds,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// TestConnection checks that the Discord webhook URL resolves and returns webhook metadata.
func (p *DiscordPublisher) TestConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.creds.WebhookURL, nil)
	if err != nil {
		return err
	}
	_, err = doJSONOrError(p.client, req, "Discord webhook check")
	return err
}

// Publish sends multipart/form-data with payload_json and files[0..3] to the Discord webhook.
func (p *DiscordPublisher) Publish(ctx context.Context, payload PostPayload) (PublishResult, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	payloadObj := map[string]any{
		"content": payload.Text,
	}

	if len(payload.Media) > 0 {
		attachments := make([]map[string]any, 0, 4)
		for i, item := range payload.Media {
			if i >= 4 {
				break
			}
			filename := item.Filename
			if filename == "" {
				filename = fmt.Sprintf("image_%d.jpg", i)
			}
			att := map[string]any{
				"id":       i,
				"filename": filename,
			}
			if strings.TrimSpace(item.AltText) != "" {
				att["description"] = item.AltText
			}
			attachments = append(attachments, att)

			part, err := writer.CreateFormFile(fmt.Sprintf("files[%d]", i), filename)
			if err != nil {
				return PublishResult{}, err
			}
			if _, err := part.Write(item.Data); err != nil {
				return PublishResult{}, err
			}
		}
		payloadObj["attachments"] = attachments
	}

	payloadJSON, err := json.Marshal(payloadObj)
	if err != nil {
		return PublishResult{}, err
	}
	if err := writer.WriteField("payload_json", string(payloadJSON)); err != nil {
		return PublishResult{}, err
	}
	if err := writer.Close(); err != nil {
		return PublishResult{}, err
	}

	endpoint := p.creds.WebhookURL
	if strings.Contains(endpoint, "?") {
		endpoint += "&wait=true"
	} else {
		endpoint += "?wait=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	respBytes, err := doJSONOrError(p.client, req, "Discord webhook publish")
	if err != nil {
		return PublishResult{}, err
	}

	var parsed struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(respBytes, &parsed)

	if fc := strings.TrimSpace(payload.FirstComment); fc != "" {
		followBody, fErr := json.Marshal(map[string]any{"content": fc})
		if fErr == nil {
			followReq, rErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(followBody))
			if rErr == nil {
				followReq.Header.Set("Content-Type", "application/json")
				if _, cErr := doJSONOrError(p.client, followReq, "Discord first comment"); cErr != nil {
					return PublishResult{}, cErr
				}
			}
		}
	}

	return PublishResult{
		ExternalID: parsed.ID,
	}, nil
}
