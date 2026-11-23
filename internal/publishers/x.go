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

	"github.com/dghubble/oauth1"
)

// XCredentials holds OAuth 1.0a credentials for the X (Twitter) API.
type XCredentials struct {
	APIKey            string `json:"apiKey"`
	APISecret         string `json:"apiSecret"`
	AccessToken       string `json:"accessToken"`
	AccessTokenSecret string `json:"accessTokenSecret"`
}

// XPublisher publishes posts and images to X using OAuth 1.0a HMAC-SHA1.
type XPublisher struct {
	creds XCredentials
}

// NewXPublisher parses X credentials and validates required fields.
func NewXPublisher(credentialsJSON string) (*XPublisher, error) {
	var creds XCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &creds); err != nil {
		return nil, fmt.Errorf("invalid X credentials JSON: %w", err)
	}
	creds.APIKey = strings.TrimSpace(creds.APIKey)
	creds.APISecret = strings.TrimSpace(creds.APISecret)
	creds.AccessToken = strings.TrimSpace(creds.AccessToken)
	creds.AccessTokenSecret = strings.TrimSpace(creds.AccessTokenSecret)

	if creds.APIKey == "" || creds.APISecret == "" || creds.AccessToken == "" || creds.AccessTokenSecret == "" {
		return nil, errors.New("X credentials require apiKey, apiSecret, accessToken, and accessTokenSecret")
	}
	return &XPublisher{creds: creds}, nil
}

func (p *XPublisher) httpClient(ctx context.Context) *http.Client {
	cfg := oauth1.NewConfig(p.creds.APIKey, p.creds.APISecret)
	token := oauth1.NewToken(p.creds.AccessToken, p.creds.AccessTokenSecret)
	client := cfg.Client(ctx, token)
	client.Timeout = 30 * time.Second
	return client
}

// TestConnection verifies X OAuth 1.0a credentials via GET /2/users/me.
func (p *XPublisher) TestConnection(ctx context.Context) error {
	client := p.httpClient(ctx)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.twitter.com/2/users/me", nil)
	if err != nil {
		return err
	}
	_, err = doJSONOrError(client, req, "X verify credentials")
	return err
}

// Publish uploads any attached images to v1.1 media/upload.json and creates a tweet via v2/tweets.
func (p *XPublisher) Publish(ctx context.Context, payload PostPayload) (PublishResult, error) {
	client := p.httpClient(ctx)

	mediaIDs := make([]string, 0, len(payload.Media))
	for i, item := range payload.Media {
		if i >= 4 {
			break
		}
		mediaID, err := p.uploadMedia(ctx, client, item)
		if err != nil {
			return PublishResult{}, err
		}
		if strings.TrimSpace(item.AltText) != "" {
			if err := p.setAltText(ctx, client, mediaID, item.AltText); err != nil {
				return PublishResult{}, err
			}
		}
		mediaIDs = append(mediaIDs, mediaID)
	}

	bodyPayload := map[string]any{
		"text": payload.Text,
	}
	if len(mediaIDs) > 0 {
		bodyPayload["media"] = map[string]any{
			"media_ids": mediaIDs,
		}
	}

	rawBody, err := json.Marshal(bodyPayload)
	if err != nil {
		return PublishResult{}, fmt.Errorf("marshal X tweet payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitter.com/2/tweets", bytes.NewReader(rawBody))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	respBytes, err := doJSONOrError(client, req, "X create tweet")
	if err != nil {
		return PublishResult{}, err
	}

	var parsed struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return PublishResult{}, fmt.Errorf("parse X tweet response: %w", err)
	}

	var externalURL string
	if parsed.Data.ID != "" {
		externalURL = "https://x.com/i/web/status/" + parsed.Data.ID
	}

	if fc := strings.TrimSpace(payload.FirstComment); fc != "" && parsed.Data.ID != "" {
		replyBody, err := json.Marshal(map[string]any{
			"text": fc,
			"reply": map[string]string{
				"in_reply_to_tweet_id": parsed.Data.ID,
			},
		})
		if err == nil {
			replyReq, rErr := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twitter.com/2/tweets", bytes.NewReader(replyBody))
			if rErr == nil {
				replyReq.Header.Set("Content-Type", "application/json")
				if _, fErr := doJSONOrError(client, replyReq, "X first comment"); fErr != nil {
					return PublishResult{ExternalID: parsed.Data.ID, ExternalURL: externalURL}, fErr
				}
			}
		}
	}

	return PublishResult{
		ExternalID:  parsed.Data.ID,
		ExternalURL: externalURL,
	}, nil
}

func (p *XPublisher) uploadMedia(ctx context.Context, client *http.Client, item MediaAttachment) (string, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	filename := item.Filename
	if filename == "" {
		filename = "image.jpg"
	}
	part, err := writer.CreateFormFile("media", filename)
	if err != nil {
		return "", fmt.Errorf("create X media multipart: %w", err)
	}
	if _, err := part.Write(item.Data); err != nil {
		return "", fmt.Errorf("write X media bytes: %w", err)
	}
	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close X media multipart: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://upload.twitter.com/1.1/media/upload.json", &buf)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	respBytes, err := doJSONOrError(client, req, "X media upload")
	if err != nil {
		return "", err
	}

	var parsed struct {
		MediaIDString string `json:"media_id_string"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return "", fmt.Errorf("parse X media upload response: %w", err)
	}
	if parsed.MediaIDString == "" {
		return "", errors.New("X media upload returned empty media_id_string")
	}
	return parsed.MediaIDString, nil
}

func (p *XPublisher) setAltText(ctx context.Context, client *http.Client, mediaID, altText string) error {
	metaBody, err := json.Marshal(map[string]any{
		"media_id": mediaID,
		"alt_text": map[string]string{
			"text": altText,
		},
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://upload.twitter.com/1.1/media/metadata/create.json", bytes.NewReader(metaBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	_, err = doJSONOrError(client, req, "X media alt text")
	return err
}
