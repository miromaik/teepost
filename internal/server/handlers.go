package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"teepost/internal/db"
	"teepost/internal/publishers"
)

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

type accountInput struct {
	Platform    string          `json:"platform"`
	Name        string          `json:"name"`
	Enabled     bool            `json:"enabled"`
	Credentials json.RawMessage `json:"credentials"`
}

func validateAccountInput(in accountInput) error {
	switch strings.ToLower(strings.TrimSpace(in.Platform)) {
	case "x", "bluesky", "telegram", "discord", "mastodon":
	default:
		return fmt.Errorf("unsupported platform %q", in.Platform)
	}
	if strings.TrimSpace(in.Name) == "" {
		return errors.New("account name is required")
	}
	if len(in.Credentials) == 0 {
		return errors.New("credentials are required")
	}
	if _, err := publishers.NewPublisher(in.Platform, string(in.Credentials)); err != nil {
		return err
	}
	return nil
}

var secretCredentialKeys = map[string][]string{
	"x":        {"apiKey", "apiSecret", "accessToken", "accessTokenSecret"},
	"bluesky":  {"appPassword"},
	"telegram": {"botToken"},
	"discord":  {"webhookUrl"},
	"mastodon": {"accessToken"},
}

func secretKeysForPlatform(platform string) []string {
	return secretCredentialKeys[strings.ToLower(strings.TrimSpace(platform))]
}

func sanitizeAccount(acc db.Account) db.Account {
	creds := map[string]string{}
	if acc.CredentialsJSON != "" {
		_ = json.Unmarshal([]byte(acc.CredentialsJSON), &creds)
	}
	for _, key := range secretKeysForPlatform(acc.Platform) {
		delete(creds, key)
	}
	raw, err := json.Marshal(creds)
	if err != nil {
		raw = []byte("{}")
	}
	acc.Credentials = raw
	acc.CredentialsJSON = ""
	return acc
}

func sanitizeAccounts(accounts []db.Account) []db.Account {
	sanitized := make([]db.Account, len(accounts))
	for i, acc := range accounts {
		sanitized[i] = sanitizeAccount(acc)
	}
	return sanitized
}

func mergeAccountCredentials(platform, existingJSON string, incoming json.RawMessage) (json.RawMessage, error) {
	merged := map[string]string{}
	if existingJSON != "" {
		if err := json.Unmarshal([]byte(existingJSON), &merged); err != nil {
			return nil, fmt.Errorf("decode existing credentials: %w", err)
		}
	}

	incomingMap := map[string]string{}
	if len(incoming) > 0 {
		if err := json.Unmarshal(incoming, &incomingMap); err != nil {
			return nil, fmt.Errorf("decode credentials: %w", err)
		}
	}

	secretKeys := make(map[string]bool, len(secretKeysForPlatform(platform)))
	for _, key := range secretKeysForPlatform(platform) {
		secretKeys[key] = true
	}
	for key, value := range incomingMap {
		if secretKeys[key] && strings.TrimSpace(value) == "" {
			continue
		}
		merged[key] = value
	}

	raw, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("encode credentials: %w", err)
	}
	return raw, nil
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	accounts, err := s.store.ListAccounts()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeAccounts(accounts))
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var in accountInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	in.Platform = strings.ToLower(strings.TrimSpace(in.Platform))
	in.Name = strings.TrimSpace(in.Name)

	if err := validateAccountInput(in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := s.store.CreateAccount(db.Account{
		ID:              generateID(),
		Platform:        in.Platform,
		Name:            in.Name,
		Enabled:         in.Enabled,
		CredentialsJSON: string(in.Credentials),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, sanitizeAccount(created))
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in accountInput
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	in.Platform = strings.ToLower(strings.TrimSpace(in.Platform))
	in.Name = strings.TrimSpace(in.Name)

	existing, err := s.store.GetAccount(id)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	existingCredentials := existing.CredentialsJSON
	if existing.Platform != in.Platform {
		existingCredentials = ""
	}
	mergedCredentials, err := mergeAccountCredentials(in.Platform, existingCredentials, in.Credentials)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	in.Credentials = mergedCredentials

	if err := validateAccountInput(in); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := s.store.UpdateAccount(db.Account{
		ID:              id,
		Platform:        in.Platform,
		Name:            in.Name,
		Enabled:         in.Enabled,
		CredentialsJSON: string(in.Credentials),
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sanitizeAccount(updated))
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.DeleteAccount(id)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

func (s *Server) handleTestSavedAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	acc, err := s.store.GetAccount(id)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pub, err := publishers.NewPublisher(acc.Platform, acc.CredentialsJSON)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	start := time.Now()
	if err := pub.TestConnection(ctx); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"error":     err.Error(),
			"latencyMs": time.Since(start).Milliseconds(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": time.Since(start).Milliseconds(),
	})
}

func (s *Server) handleTestUnsavedCredentials(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Platform    string          `json:"platform"`
		Credentials json.RawMessage `json:"credentials"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	pub, err := publishers.NewPublisher(in.Platform, string(in.Credentials))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	start := time.Now()
	if err := pub.TestConnection(ctx); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        false,
			"error":     err.Error(),
			"latencyMs": time.Since(start).Milliseconds(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": time.Since(start).Milliseconds(),
	})
}

func (s *Server) handleUploadMedia(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	if err := r.ParseMultipartForm(20 << 20); err != nil {
		writeJSONError(w, http.StatusBadRequest, "failed to parse multipart upload")
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "missing 'file' form field")
		return
	}
	defer file.Close()

	stored, err := s.media.SaveUploadedFile(file, header.Filename)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, stored)
}

func (s *Server) handleServeMedia(w http.ResponseWriter, r *http.Request) {
	filename := r.PathValue("filename")
	data, mimeType, err := s.media.ReadMediaBytes(filename)
	if errors.Is(err, os.ErrNotExist) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type publishRequest struct {
	PostID              string            `json:"postId"`
	AccountIDs          []string          `json:"accountIds"`
	MasterText          string            `json:"masterText"`
	FirstComment        string            `json:"firstComment"`
	CustomizePerNetwork bool              `json:"customizePerNetwork"`
	CustomTexts         map[string]string `json:"customTexts"`
	Media               []db.MediaItem    `json:"media"`
	Tags                []string          `json:"tags"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	tzName, _ := time.Now().Zone()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"time":     time.Now().UTC().Format(time.RFC3339),
		"timezone": tzName,
	})
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.store.Ping(ctx); err != nil {
		writeJSONError(w, http.StatusServiceUnavailable, "database not ready")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	tzName, offsetSec := time.Now().Zone()
	writeJSON(w, http.StatusOK, map[string]any{
		"version":    "0.1.0-alpha",
		"timezone":   fmt.Sprintf("%s (UTC%+d)", tzName, offsetSec/3600),
		"serverTime": time.Now().Format(time.RFC3339),
		"dbPath":     s.cfg.DBPath,
		"mediaDir":   s.cfg.MediaDir,
	})
}

func (s *Server) loadMediaAttachments(items []db.MediaItem) ([]publishers.MediaAttachment, error) {
	if len(items) > 4 {
		items = items[:4]
	}
	attachments := make([]publishers.MediaAttachment, 0, len(items))
	for _, item := range items {
		data, detectedMime, err := s.media.ReadMediaBytes(item.StoredName)
		if err != nil {
			return nil, fmt.Errorf("load media %q: %w", item.Filename, err)
		}
		mimeType := item.MimeType
		if mimeType == "" {
			mimeType = detectedMime
		}
		attachments = append(attachments, publishers.MediaAttachment{
			Filename: item.Filename,
			MimeType: mimeType,
			Data:     data,
			AltText:  item.AltText,
			Width:    item.Width,
			Height:   item.Height,
		})
	}
	return attachments, nil
}

func publishToAccount(ctx context.Context, postID string, acc db.Account, text string, firstComment string, attachments []publishers.MediaAttachment) db.PostResult {
	result := db.PostResult{
		ID:          generateID(),
		PostID:      postID,
		AccountID:   acc.ID,
		Platform:    acc.Platform,
		AccountName: acc.Name,
		TextSent:    text,
		PublishedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}

	pub, err := publishers.NewPublisher(acc.Platform, acc.CredentialsJSON)
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		return result
	}

	pubRes, err := pub.Publish(ctx, publishers.PostPayload{
		Text:         text,
		FirstComment: firstComment,
		Media:        attachments,
	})
	if err != nil {
		result.Status = "error"
		result.ErrorMessage = err.Error()
		result.ExternalID = pubRes.ExternalID
		result.ExternalURL = pubRes.ExternalURL
		return result
	}

	result.Status = "success"
	result.ExternalID = pubRes.ExternalID
	result.ExternalURL = pubRes.ExternalURL
	return result
}

func systemFailureResult(postID, text, message string) db.PostResult {
	return db.PostResult{
		ID:           generateID(),
		PostID:       postID,
		AccountID:    "system",
		Platform:     "system",
		AccountName:  "System",
		TextSent:     text,
		Status:       "error",
		ErrorMessage: message,
		PublishedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
}

func (s *Server) executePublish(ctx context.Context, postID string, targetAccounts []db.Account, req publishRequest, attachments []publishers.MediaAttachment) (db.Post, error) {
	results := make([]db.PostResult, len(targetAccounts))

	pubCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	var wg sync.WaitGroup
	for i, acc := range targetAccounts {
		wg.Add(1)
		go func(idx int, account db.Account) {
			defer wg.Done()
			text := req.MasterText
			if req.CustomizePerNetwork {
				if custom, ok := req.CustomTexts[account.ID]; ok {
					text = custom
				}
			}
			results[idx] = publishToAccount(pubCtx, postID, account, text, req.FirstComment, attachments)
		}(i, acc)
	}
	wg.Wait()

	finalStatus := db.PostStatusSent
	successCount := 0
	for _, r := range results {
		if r.Status == "success" {
			successCount++
		}
	}
	if successCount == 0 {
		finalStatus = db.PostStatusFailed
	}

	targetIDs := make([]string, len(targetAccounts))
	for i, acc := range targetAccounts {
		targetIDs[i] = acc.ID
	}

	if req.PostID != "" {
		if _, err := s.store.GetPost(req.PostID); err == nil {
			saved, err := s.store.SavePost(db.Post{
				ID:                  req.PostID,
				Status:              finalStatus,
				MasterText:          req.MasterText,
				FirstComment:        req.FirstComment,
				CustomizePerNetwork: req.CustomizePerNetwork,
				CustomTexts:         req.CustomTexts,
				Media:               req.Media,
				TargetAccountIDs:    targetIDs,
				Tags:                req.Tags,
				ScheduledAt:         "",
			})
			if err != nil {
				return db.Post{}, err
			}
			return s.store.FinalizePublishedPost(saved.ID, finalStatus, results)
		}
	}

	nowStr := time.Now().UTC().Format(time.RFC3339Nano)
	return s.store.CreatePostWithResults(db.Post{
		ID:                  postID,
		Status:              finalStatus,
		MasterText:          req.MasterText,
		FirstComment:        req.FirstComment,
		CustomizePerNetwork: req.CustomizePerNetwork,
		CustomTexts:         req.CustomTexts,
		Media:               req.Media,
		TargetAccountIDs:    targetIDs,
		Tags:                req.Tags,
		Results:             results,
		CreatedAt:           nowStr,
		UpdatedAt:           nowStr,
	})
}

func (s *Server) handlePublish(w http.ResponseWriter, r *http.Request) {
	var req publishRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if len(req.AccountIDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "select at least one account")
		return
	}
	if strings.TrimSpace(req.MasterText) == "" && len(req.Media) == 0 {
		writeJSONError(w, http.StatusBadRequest, "post text or media is required")
		return
	}

	attachments, err := s.loadMediaAttachments(req.Media)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	targetAccounts := make([]db.Account, 0, len(req.AccountIDs))
	seen := make(map[string]bool)
	for _, accID := range req.AccountIDs {
		if seen[accID] {
			continue
		}
		seen[accID] = true
		acc, err := s.store.GetAccount(accID)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("account %q not found", accID))
			return
		}
		if !acc.Enabled {
			writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("account %q is disabled", acc.Name))
			return
		}
		targetAccounts = append(targetAccounts, acc)
	}

	postID := req.PostID
	if postID == "" {
		postID = generateID()
	} else {
		existing, getErr := s.store.GetPost(postID)
		switch {
		case errors.Is(getErr, db.ErrNotFound):
			// caller-provided id for a brand new post; continue
		case getErr != nil:
			writeJSONError(w, http.StatusInternalServerError, getErr.Error())
			return
		default:
			if existing.Status == db.PostStatusSent || existing.Status == db.PostStatusPublishing {
				writeJSONError(w, http.StatusConflict, "post is already being published or was sent")
				return
			}
			claimed, claimErr := s.store.ClaimPostForPublishing(postID)
			if claimErr != nil {
				writeJSONError(w, http.StatusInternalServerError, claimErr.Error())
				return
			}
			if !claimed {
				writeJSONError(w, http.StatusConflict, "post is already being published or was sent")
				return
			}
		}
	}

	savedPost, err := s.executePublish(r.Context(), postID, targetAccounts, req, attachments)
	if err != nil {
		_, _ = s.store.FinalizePublishedPost(postID, db.PostStatusFailed, []db.PostResult{
			systemFailureResult(postID, req.MasterText, err.Error()),
		})
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, savedPost)
}

type savePostRequest struct {
	ID                  string            `json:"id"`
	Mode                string            `json:"mode"` // "draft" | "idea" | "schedule" | "queue"
	AccountIDs          []string          `json:"accountIds"`
	MasterText          string            `json:"masterText"`
	FirstComment        string            `json:"firstComment"`
	CustomizePerNetwork bool              `json:"customizePerNetwork"`
	CustomTexts         map[string]string `json:"customTexts"`
	Media               []db.MediaItem    `json:"media"`
	Tags                []string          `json:"tags"`
	ScheduledAt         string            `json:"scheduledAt"`
}

func (s *Server) handleSavePost(w http.ResponseWriter, r *http.Request) {
	var req savePostRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if strings.TrimSpace(req.MasterText) == "" && len(req.Media) == 0 {
		writeJSONError(w, http.StatusBadRequest, "post text or media is required")
		return
	}

	var status string
	var scheduledAt string

	switch strings.ToLower(strings.TrimSpace(req.Mode)) {
	case "idea":
		status = db.PostStatusIdea
		scheduledAt = ""
	case "draft", "":
		status = db.PostStatusDraft
		scheduledAt = ""
	case "schedule":
		if len(req.AccountIDs) == 0 {
			writeJSONError(w, http.StatusBadRequest, "select at least one channel before scheduling")
			return
		}
		parsed, err := time.Parse(time.RFC3339Nano, req.ScheduledAt)
		if err != nil {
			parsed, err = time.Parse(time.RFC3339, req.ScheduledAt)
		}
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "valid scheduledAt timestamp (RFC3339) is required")
			return
		}
		status = db.PostStatusScheduled
		scheduledAt = parsed.UTC().Format(time.RFC3339)
	case "queue":
		if len(req.AccountIDs) == 0 {
			writeJSONError(w, http.StatusBadRequest, "select at least one channel before adding to queue")
			return
		}
		nextTime, err := s.store.NextAvailableQueueTime(time.Now())
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		status = db.PostStatusScheduled
		scheduledAt = nextTime.UTC().Format(time.RFC3339)
	default:
		writeJSONError(w, http.StatusBadRequest, fmt.Sprintf("unsupported save mode %q", req.Mode))
		return
	}

	postID := req.ID
	if postID == "" {
		postID = generateID()
	}

	saved, err := s.store.SavePost(db.Post{
		ID:                  postID,
		Status:              status,
		MasterText:          req.MasterText,
		FirstComment:        req.FirstComment,
		CustomizePerNetwork: req.CustomizePerNetwork,
		CustomTexts:         req.CustomTexts,
		Media:               req.Media,
		TargetAccountIDs:    req.AccountIDs,
		Tags:                req.Tags,
		ScheduledAt:         scheduledAt,
	})
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handlePublishSavedPostNow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	post, err := s.store.GetPost(id)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	switch post.Status {
	case db.PostStatusDraft, db.PostStatusIdea, db.PostStatusScheduled:
	default:
		writeJSONError(w, http.StatusConflict, "post is already being published or was sent")
		return
	}

	accountIDs := post.TargetAccountIDs
	if len(accountIDs) == 0 {
		accounts, err := s.store.ListAccounts()
		if err == nil {
			for _, a := range accounts {
				if a.Enabled {
					accountIDs = append(accountIDs, a.ID)
				}
			}
		}
	}
	if len(accountIDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no target channels configured for this post")
		return
	}

	targetAccounts := make([]db.Account, 0, len(accountIDs))
	targetIDs := make([]string, 0, len(accountIDs))
	seen := make(map[string]bool, len(accountIDs))
	for _, accID := range accountIDs {
		if seen[accID] {
			continue
		}
		seen[accID] = true
		acc, err := s.store.GetAccount(accID)
		if err != nil {
			continue
		}
		if !acc.Enabled {
			continue
		}
		targetAccounts = append(targetAccounts, acc)
		targetIDs = append(targetIDs, acc.ID)
	}
	if len(targetAccounts) == 0 {
		writeJSONError(w, http.StatusBadRequest, "no enabled target channels available for this post")
		return
	}

	attachments, err := s.loadMediaAttachments(post.Media)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	claimed, err := s.store.ClaimPostForPublishing(post.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !claimed {
		writeJSONError(w, http.StatusConflict, "post is already being published or was sent")
		return
	}

	req := publishRequest{
		PostID:              post.ID,
		AccountIDs:          targetIDs,
		MasterText:          post.MasterText,
		FirstComment:        post.FirstComment,
		CustomizePerNetwork: post.CustomizePerNetwork,
		CustomTexts:         post.CustomTexts,
		Media:               post.Media,
		Tags:                post.Tags,
	}
	updated, err := s.executePublish(r.Context(), post.ID, targetAccounts, req, attachments)
	if err != nil {
		_, _ = s.store.FinalizePublishedPost(post.ID, db.PostStatusFailed, []db.PostResult{
			systemFailureResult(post.ID, post.MasterText, err.Error()),
		})
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) handleGetScheduleSlots(w http.ResponseWriter, r *http.Request) {
	slots, err := s.store.ListPostingSlots()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nextSlot, err := s.store.NextAvailableQueueTime(time.Now())
	nextSlotStr := ""
	if err == nil {
		nextSlotStr = nextSlot.UTC().Format(time.RFC3339)
	}
	tzName, _ := time.Now().Zone()
	writeJSON(w, http.StatusOK, map[string]any{
		"slots":    slots,
		"nextSlot": nextSlotStr,
		"timezone": tzName,
		"paused":   s.store.IsQueuePaused(),
	})
}

func (s *Server) handleUpdateScheduleSlots(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Slots []db.PostingSlot `json:"slots"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	updated, err := s.store.ReplacePostingSlots(in.Slots)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	nextSlot, err := s.store.NextAvailableQueueTime(time.Now())
	nextSlotStr := ""
	if err == nil {
		nextSlotStr = nextSlot.UTC().Format(time.RFC3339)
	}
	tzName, _ := time.Now().Zone()
	writeJSON(w, http.StatusOK, map[string]any{
		"slots":    updated,
		"nextSlot": nextSlotStr,
		"timezone": tzName,
		"paused":   s.store.IsQueuePaused(),
	})
}

func (s *Server) handleSetQueuePaused(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Paused bool `json:"paused"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if err := s.store.SetQueuePaused(in.Paused); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"paused": s.store.IsQueuePaused()})
}

func (s *Server) handleReorderQueue(w http.ResponseWriter, r *http.Request) {
	var in struct {
		PostID    string `json:"postId"`
		Direction string `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	if err := s.store.ReorderScheduledPost(in.PostID, in.Direction); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	posts, err := s.store.ListPosts(500)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *Server) handleShuffleQueue(w http.ResponseWriter, r *http.Request) {
	if err := s.store.ShuffleScheduledPosts(); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	posts, err := s.store.ListPosts(500)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *Server) handleListHashtagGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := s.store.ListHashtagGroups()
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, groups)
}

func (s *Server) handleSaveHashtagGroups(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Groups []db.HashtagGroup `json:"groups"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	saved, err := s.store.SaveHashtagGroups(in.Groups)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleGetPreferences(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.store.GetDisplayPreferences())
}

func (s *Server) handleSavePreferences(w http.ResponseWriter, r *http.Request) {
	var in db.DisplayPreferences
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}
	saved, err := s.store.SaveDisplayPreferences(in)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, saved)
}

func (s *Server) handleRetryPostAccount(w http.ResponseWriter, r *http.Request) {
	postID := r.PathValue("postId")
	accountID := r.PathValue("accountId")

	post, err := s.store.GetPost(postID)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	acc, err := s.store.GetAccount(accountID)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "account not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !acc.Enabled {
		writeJSONError(w, http.StatusBadRequest, "account is disabled")
		return
	}

	for _, existing := range post.Results {
		if existing.AccountID != accountID {
			continue
		}
		if strings.TrimSpace(existing.ExternalID) != "" || strings.TrimSpace(existing.ExternalURL) != "" {
			writeJSONError(w, http.StatusConflict, "the main post already exists on this platform; retrying would duplicate it")
			return
		}
	}

	attachments, err := s.loadMediaAttachments(post.Media)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	text := post.MasterText
	if post.CustomizePerNetwork {
		if custom, ok := post.CustomTexts[accountID]; ok {
			text = custom
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 45*time.Second)
	defer cancel()

	res := publishToAccount(ctx, post.ID, acc, text, post.FirstComment, attachments)
	if err := s.store.UpdatePostResult(res); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	updatedPost, err := s.store.GetPost(post.ID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updatedPost)
}

func (s *Server) handleListPosts(w http.ResponseWriter, r *http.Request) {
	posts, err := s.store.ListPosts(500)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, posts)
}

func (s *Server) handleDeletePost(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.store.DeletePost(id)
	if errors.Is(err, db.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "post not found")
		return
	}
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
