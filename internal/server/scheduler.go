package server

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"teepost/internal/db"
)

// StartScheduler starts a background loop that publishes scheduled posts when their scheduledAt time is reached.
func (s *Server) StartScheduler(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	// Posts left in publishing by a crash have no lease; fail them with a diagnostic result.
	if recovered, err := s.store.RecoverStalePublishingPosts(time.Now().UTC().Add(-2 * time.Minute)); err != nil {
		slog.Error("recover stale publishing posts", "error", err)
	} else if recovered > 0 {
		slog.Warn("recovered stale publishing posts", "count", recovered)
	}

	// Run once shortly after startup in case posts became due while offline.
	s.processDueScheduledPosts(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processDueScheduledPosts(ctx)
		}
	}
}

func (s *Server) processDueScheduledPosts(ctx context.Context) {
	if s.store.IsQueuePaused() {
		return
	}
	duePosts, err := s.store.ClaimDueScheduledPosts(time.Now().UTC())
	if err != nil {
		slog.Error("scheduler claim error", "error", err)
		return
	}

	for _, post := range duePosts {
		s.publishClaimedPost(ctx, post)
	}
}

func (s *Server) publishClaimedPost(ctx context.Context, post db.Post) {
	targetIDs := post.TargetAccountIDs
	if len(targetIDs) == 0 {
		accounts, err := s.store.ListAccounts()
		if err == nil {
			for _, acc := range accounts {
				if acc.Enabled {
					targetIDs = append(targetIDs, acc.ID)
				}
			}
		}
	}

	targetAccounts := make([]db.Account, 0, len(targetIDs))
	seen := make(map[string]bool)
	for _, accID := range targetIDs {
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
	}

	if len(targetAccounts) == 0 {
		_, _ = s.store.FinalizePublishedPost(post.ID, db.PostStatusFailed, []db.PostResult{
			{
				ID:           generateID(),
				PostID:       post.ID,
				AccountID:    "none",
				Platform:     "discord",
				AccountName:  "No target channel",
				TextSent:     post.MasterText,
				Status:       "error",
				ErrorMessage: "No valid target channels configured for this scheduled post.",
				PublishedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			},
		})
		return
	}

	attachments, err := s.loadMediaAttachments(post.Media)
	if err != nil {
		results := make([]db.PostResult, len(targetAccounts))
		for i, acc := range targetAccounts {
			results[i] = db.PostResult{
				ID:           generateID(),
				PostID:       post.ID,
				AccountID:    acc.ID,
				Platform:     acc.Platform,
				AccountName:  acc.Name,
				TextSent:     post.MasterText,
				Status:       "error",
				ErrorMessage: err.Error(),
				PublishedAt:  time.Now().UTC().Format(time.RFC3339Nano),
			}
		}
		_, _ = s.store.FinalizePublishedPost(post.ID, db.PostStatusFailed, results)
		return
	}

	pubCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	results := make([]db.PostResult, len(targetAccounts))
	var wg sync.WaitGroup
	for i, acc := range targetAccounts {
		wg.Add(1)
		go func(idx int, account db.Account) {
			defer wg.Done()
			text := post.MasterText
			if post.CustomizePerNetwork {
				if custom, ok := post.CustomTexts[account.ID]; ok {
					text = custom
				}
			}
			results[idx] = publishToAccount(pubCtx, post.ID, account, text, post.FirstComment, attachments)
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

	if _, err := s.store.FinalizePublishedPost(post.ID, finalStatus, results); err != nil {
		slog.Error("scheduler finalize post error", "postId", post.ID, "error", err)
	} else {
		slog.Info("scheduler published post", "postId", post.ID, "succeeded", successCount, "total", len(results))
	}
}
