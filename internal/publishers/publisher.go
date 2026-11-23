package publishers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MediaAttachment represents an image attached to a social post.
type MediaAttachment struct {
	Filename string
	MimeType string
	Data     []byte
	AltText  string
	Width    int
	Height   int
}

// PostPayload contains the text, optional first comment reply, and media attachments to publish.
type PostPayload struct {
	Text         string
	FirstComment string
	Media        []MediaAttachment
}

// PublishResult contains the remote post ID and public URL when provided by the network.
type PublishResult struct {
	ExternalID  string `json:"externalId"`
	ExternalURL string `json:"externalUrl"`
}

// Publisher defines the contract every social media integration implements.
type Publisher interface {
	Publish(ctx context.Context, payload PostPayload) (PublishResult, error)
	TestConnection(ctx context.Context) error
}

// NewPublisher instantiates the appropriate Publisher for a given platform and JSON credentials.
func NewPublisher(platform string, credentialsJSON string) (Publisher, error) {
	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "x":
		return NewXPublisher(credentialsJSON)
	case "bluesky":
		return NewBlueskyPublisher(credentialsJSON)
	case "telegram":
		return NewTelegramPublisher(credentialsJSON)
	case "discord":
		return NewDiscordPublisher(credentialsJSON)
	case "mastodon":
		return NewMastodonPublisher(credentialsJSON)
	default:
		return nil, fmt.Errorf("unsupported platform %q", platform)
	}
}

func readResponseSnippet(r io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(r, 2048))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func formatHTTPError(prefix string, status int, body string) error {
	if body == "" {
		return fmt.Errorf("%s: HTTP %d", prefix, status)
	}
	return fmt.Errorf("%s: HTTP %d: %s", prefix, status, body)
}

func doJSONOrError(client *http.Client, req *http.Request, prefix string) ([]byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", prefix, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("%s: read response: %w", prefix, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, formatHTTPError(prefix, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}
