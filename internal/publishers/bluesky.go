package publishers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"
)

// BlueskyCredentials holds AT Protocol credentials.
type BlueskyCredentials struct {
	Identifier  string `json:"identifier"`
	AppPassword string `json:"appPassword"`
	PDSURL      string `json:"pdsUrl"`
}

// BlueskyPublisher publishes posts with UTF-8 facets and image blobs to Bluesky.
type BlueskyPublisher struct {
	creds  BlueskyCredentials
	client *http.Client
}

type bskySession struct {
	AccessJwt string `json:"accessJwt"`
	DID       string `json:"did"`
	Handle    string `json:"handle"`
}

// NewBlueskyPublisher validates Bluesky credentials and sets default PDS URL.
func NewBlueskyPublisher(credentialsJSON string) (*BlueskyPublisher, error) {
	var creds BlueskyCredentials
	if err := json.Unmarshal([]byte(credentialsJSON), &creds); err != nil {
		return nil, fmt.Errorf("invalid Bluesky credentials JSON: %w", err)
	}
	creds.Identifier = strings.TrimSpace(creds.Identifier)
	creds.AppPassword = strings.TrimSpace(creds.AppPassword)
	creds.PDSURL = strings.TrimRight(strings.TrimSpace(creds.PDSURL), "/")
	if creds.PDSURL == "" {
		creds.PDSURL = "https://bsky.social"
	}
	if creds.Identifier == "" || creds.AppPassword == "" {
		return nil, errors.New("Bluesky credentials require identifier (handle or DID) and appPassword")
	}
	return &BlueskyPublisher{
		creds:  creds,
		client: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (p *BlueskyPublisher) createSession(ctx context.Context) (bskySession, error) {
	payload, err := json.Marshal(map[string]string{
		"identifier": p.creds.Identifier,
		"password":   p.creds.AppPassword,
	})
	if err != nil {
		return bskySession{}, err
	}

	endpoint := p.creds.PDSURL + "/xrpc/com.atproto.server.createSession"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return bskySession{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	respBytes, err := doJSONOrError(p.client, req, "Bluesky createSession")
	if err != nil {
		return bskySession{}, err
	}

	var session bskySession
	if err := json.Unmarshal(respBytes, &session); err != nil {
		return bskySession{}, fmt.Errorf("parse Bluesky session: %w", err)
	}
	if session.AccessJwt == "" || session.DID == "" {
		return bskySession{}, errors.New("Bluesky createSession returned incomplete session")
	}
	return session, nil
}

// TestConnection verifies Bluesky credentials by creating a session.
func (p *BlueskyPublisher) TestConnection(ctx context.Context) error {
	_, err := p.createSession(ctx)
	return err
}

// Publish authenticates, uploads image blobs, builds facets, and creates an app.bsky.feed.post record.
func (p *BlueskyPublisher) Publish(ctx context.Context, payload PostPayload) (PublishResult, error) {
	session, err := p.createSession(ctx)
	if err != nil {
		return PublishResult{}, err
	}

	record := map[string]any{
		"$type":     "app.bsky.feed.post",
		"text":      payload.Text,
		"createdAt": time.Now().UTC().Format(time.RFC3339),
	}

	facets := ParseFacets(payload.Text)
	if len(facets) > 0 {
		record["facets"] = facets
	}

	if len(payload.Media) > 0 {
		images := make([]map[string]any, 0, len(payload.Media))
		for i, item := range payload.Media {
			if i >= 4 {
				break
			}
			blob, err := p.uploadBlob(ctx, session.AccessJwt, item)
			if err != nil {
				return PublishResult{}, err
			}
			imgEntry := map[string]any{
				"alt":   item.AltText,
				"image": blob,
			}
			if item.Width > 0 && item.Height > 0 {
				imgEntry["aspectRatio"] = map[string]int{
					"width":  item.Width,
					"height": item.Height,
				}
			}
			images = append(images, imgEntry)
		}
		record["embed"] = map[string]any{
			"$type":  "app.bsky.embed.images",
			"images": images,
		}
	}

	createBody, err := json.Marshal(map[string]any{
		"repo":       session.DID,
		"collection": "app.bsky.feed.post",
		"record":     record,
	})
	if err != nil {
		return PublishResult{}, err
	}

	endpoint := p.creds.PDSURL + "/xrpc/com.atproto.repo.createRecord"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(createBody))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+session.AccessJwt)

	respBytes, err := doJSONOrError(p.client, req, "Bluesky createRecord")
	if err != nil {
		return PublishResult{}, err
	}

	var parsed struct {
		URI string `json:"uri"`
		CID string `json:"cid"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return PublishResult{}, fmt.Errorf("parse Bluesky createRecord response: %w", err)
	}

	var externalURL string
	if parsed.URI != "" {
		parts := strings.Split(parsed.URI, "/")
		rkey := parts[len(parts)-1]
		handleOrDID := session.Handle
		if handleOrDID == "" {
			handleOrDID = session.DID
		}
		externalURL = fmt.Sprintf("https://bsky.app/profile/%s/post/%s", handleOrDID, rkey)
	}

	if fc := strings.TrimSpace(payload.FirstComment); fc != "" && parsed.URI != "" && parsed.CID != "" {
		replyRec := map[string]any{
			"$type":     "app.bsky.feed.post",
			"text":      fc,
			"createdAt": time.Now().UTC().Format(time.RFC3339),
			"reply": map[string]any{
				"root":   map[string]string{"uri": parsed.URI, "cid": parsed.CID},
				"parent": map[string]string{"uri": parsed.URI, "cid": parsed.CID},
			},
		}
		if fcFacets := ParseFacets(fc); len(fcFacets) > 0 {
			replyRec["facets"] = fcFacets
		}
		replyBody, mErr := json.Marshal(map[string]any{
			"repo":       session.DID,
			"collection": "app.bsky.feed.post",
			"record":     replyRec,
		})
		if mErr == nil {
			replyReq, rErr := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(replyBody))
			if rErr == nil {
				replyReq.Header.Set("Content-Type", "application/json")
				replyReq.Header.Set("Authorization", "Bearer "+session.AccessJwt)
				if _, fErr := doJSONOrError(p.client, replyReq, "Bluesky first comment"); fErr != nil {
					return PublishResult{ExternalID: parsed.URI, ExternalURL: externalURL}, fErr
				}
			}
		}
	}

	return PublishResult{
		ExternalID:  parsed.URI,
		ExternalURL: externalURL,
	}, nil
}

func (p *BlueskyPublisher) uploadBlob(ctx context.Context, accessJwt string, item MediaAttachment) (json.RawMessage, error) {
	mimeType := item.MimeType
	if mimeType == "" {
		mimeType = "image/jpeg"
	}

	endpoint := p.creds.PDSURL + "/xrpc/com.atproto.repo.uploadBlob"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(item.Data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mimeType)
	req.Header.Set("Authorization", "Bearer "+accessJwt)

	respBytes, err := doJSONOrError(p.client, req, "Bluesky uploadBlob")
	if err != nil {
		return nil, err
	}

	var parsed struct {
		Blob json.RawMessage `json:"blob"`
	}
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("parse Bluesky uploadBlob response: %w", err)
	}
	if len(parsed.Blob) == 0 {
		return nil, errors.New("Bluesky uploadBlob returned empty blob")
	}
	return parsed.Blob, nil
}

var (
	urlRegex = regexp.MustCompile(`https?://[^\s\)\]\}>"']+`)
	tagRegex = regexp.MustCompile(`(?:^|[\s\(\[\{])(#[^\s#.,;:!?\)\]\}>"']+)`)
)

type byteRange struct {
	start int
	end   int
}

// ParseFacets extracts clickable links and hashtags using exact UTF-8 byte offsets for Bluesky AT Protocol.
func ParseFacets(text string) []map[string]any {
	facets := make([]map[string]any, 0)
	occupied := make([]byteRange, 0)

	for _, match := range urlRegex.FindAllStringIndex(text, -1) {
		start, end := match[0], match[1]
		raw := text[start:end]
		trimmed := strings.TrimRight(raw, ".,;:!?")
		end = start + len(trimmed)
		if end <= start {
			continue
		}
		occupied = append(occupied, byteRange{start: start, end: end})
		facets = append(facets, map[string]any{
			"index": map[string]int{
				"byteStart": start,
				"byteEnd":   end,
			},
			"features": []map[string]string{
				{
					"$type": "app.bsky.richtext.facet#link",
					"uri":   trimmed,
				},
			},
		})
	}

	for _, submatch := range tagRegex.FindAllStringSubmatchIndex(text, -1) {
		// Group 1 is the `#tag` portion without leading whitespace/bracket.
		if len(submatch) < 4 {
			continue
		}
		start, end := submatch[2], submatch[3]
		raw := text[start:end]
		trimmed := strings.TrimRight(raw, ".,;:!?")
		end = start + len(trimmed)
		if len(trimmed) <= 1 {
			continue
		}
		tagValue := strings.TrimPrefix(trimmed, "#")
		if tagValue == "" || isAllDigits(tagValue) {
			continue
		}
		if overlaps(start, end, occupied) {
			continue
		}
		facets = append(facets, map[string]any{
			"index": map[string]int{
				"byteStart": start,
				"byteEnd":   end,
			},
			"features": []map[string]string{
				{
					"$type": "app.bsky.richtext.facet#tag",
					"tag":   tagValue,
				},
			},
		})
	}

	return facets
}

func overlaps(start, end int, ranges []byteRange) bool {
	for _, r := range ranges {
		if start < r.end && end > r.start {
			return true
		}
	}
	return false
}

func isAllDigits(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
