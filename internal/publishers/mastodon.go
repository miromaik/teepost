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

// MastodonCredentials holds the Mastodon instance base URL and OAuth access token.
type MastodonCredentials struct {
	InstanceURL string `json:"instanceUrl"`
	AccessToken string `json:"accessToken"`
}

// MastodonPublisher uploads media via /api/v2/media and publishes statuses via /api/v1/statuses.
type MastodonPublisher struct {
	creds  MastodonCredentials
	client *http.Client
}

// NewMastodonPublisher parses and validates Mastodon credentials.
func NewMastodonPublisher(credentialsJSON string) (*MastodonPublisher, error) {
	var creds MastodonCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &creds); err != nil {
		return nil, fmt.Errorf("invalid Mastodon credentials JSON: %w", err)
	}
	creds.InstanceURL = strings.TrimRight(strings.TrimSpace(creds.InstanceURL), "/")
	creds.AccessToken = strings.TrimSpace(creds.AccessToken)
	if creds.InstanceURL == "" || creds.AccessToken == "" {
		return nil, errors.New("Mastodon credentials require instanceUrl and accessToken")
	}
	return &MastodonPublisher{
		creds:  creds,
		client: &http.Client{Timeout: 35 * time.Second},
	}, nil
}

// TestConnection verifies credentials via GET /api/v1/accounts/verify_credentials.
func (p *MastodonPublisher) TestConnection(ctx context.Context) error {
	endpoint := p.creds.InstanceURL + "/api/v1/accounts/verify_credentials"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.creds.AccessToken)
	_, err = doJSONOrError(p.client, req, "Mastodon verify_credentials")
	return err
}

// Publish uploads up to 4 images via /api/v2/media and creates the status via /api/v1/statuses.
func (p *MastodonPublisher) Publish(ctx context.Context, payload PostPayload) (PublishResult, error) {
	mediaIDs := make([]string, 0, len(payload.Media))
	for i, item := range payload.Media {
		if i >= 4 {
			break
		}
		mediaID, err := p.uploadMedia(ctx, item)
		if err != nil {
			return PublishResult{}, err
		}
		mediaIDs = append(mediaIDs, mediaID)
	}

	statusObj := map[string]any{
		"status": payload.Text,
	}
	if len(mediaIDs) > 0 {
		statusObj["media_ids"] = mediaIDs
	}

	rawBody, err := json.Marshal(statusObj)
	if err != nil {
		return PublishResult{}, err
	}

	endpoint := p.creds.InstanceURL + "/api/v1/statuses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.creds.AccessToken)

	respBytes, err := doJSONOrError(p.client, req, "Mastodon create status")
	if err != nil {
		return PublishResult{}, err
	}

	var parsed struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return PublishResult{}, fmt.Errorf("parse Mastodon status response: %w", err)
	}

	if fc := strings.TrimSpace(payload.FirstComment); fc != "" && parsed.ID != "" {
		replyBody, err := json.Marshal(map[string]any{
			"status":         fc,
			"in_reply_to_id": parsed.ID,
		})
		if err == nil {
			replyReq, rErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(replyBody))
			if rErr == nil {
				replyReq.Header.Set("Content-Type", "application/json")
				replyReq.Header.Set("Authorization", "Bearer "+p.creds.AccessToken)
				if _, fErr := doJSONOrError(p.client, replyReq, "Mastodon first comment"); fErr != nil {
					return PublishResult{ExternalID: parsed.ID, ExternalURL: parsed.URL}, fErr
				}
			}
		}
	}

	return PublishResult{
		ExternalID:  parsed.ID,
		ExternalURL: parsed.URL,
	}, nil
}

func (p *MastodonPublisher) uploadMedia(ctx context.Context, item MediaAttachment) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	filename := item.Filename
	if filename == "" {
		filename = "image.jpg"
	}
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return "", err
	}
	if _, err := part.Write(item.Data); err != nil {
		return "", err
	}
	if strings.TrimSpace(item.AltText) != "" {
		_ = writer.WriteField("description", item.AltText)
	}
	if err := writer.Close(); err != nil {
		return "", err
	}

	endpoint := p.creds.InstanceURL + "/api/v2/media"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+p.creds.AccessToken)

	respBytes, err := doJSONOrError(p.client, req, "Mastodon upload media")
	if err != nil {
		return "", err
	}

	var parsed struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("parse Mastodon media response: %w", err)
	}
	if parsed.ID == "" {
		return "", errors.New("Mastodon media upload returned empty id")
	}
	return parsed.ID, nil
}
