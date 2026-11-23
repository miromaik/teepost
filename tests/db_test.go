package tests

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"teepost/internal/db"
)

func openTestStore(t *testing.T) *db.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open test store: %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("close test store: %v", err)
		}
	})
	return store
}

func TestAccountCRUDAndPing(t *testing.T) {
	store := openTestStore(t)

	if err := store.Ping(context.Background()); err != nil {
		t.Fatalf("ping failed: %v", err)
	}

	created, err := store.CreateAccount(db.Account{
		ID:              "acc-1",
		Platform:        "discord",
		Name:            "#announcements",
		Enabled:         true,
		CredentialsJSON: `{"webhookUrl":"https://discord.com/api/webhooks/1/x"}`,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if created.ID != "acc-1" || !created.Enabled {
		t.Fatalf("unexpected created account: %#v", created)
	}

	list, err := store.ListAccounts()
	if err != nil || len(list) != 1 {
		t.Fatalf("expected 1 account, got %d (err=%v)", len(list), err)
	}

	created.Enabled = false
	created.Name = "#updates"
	updated, err := store.UpdateAccount(created)
	if err != nil {
		t.Fatalf("update account: %v", err)
	}
	if updated.Enabled || updated.Name != "#updates" {
		t.Fatalf("unexpected updated account: %#v", updated)
	}

	if err := store.DeleteAccount("acc-1"); err != nil {
		t.Fatalf("delete account: %v", err)
	}
}

func TestScheduledPostLifecycleAndQueueSlots(t *testing.T) {
	store := openTestStore(t)

	fixedNow := time.Date(2026, 11, 23, 8, 0, 0, 0, time.UTC)

	// Default slots should be seeded on initial migration.
	slots, err := store.ListPostingSlots()
	if err != nil || len(slots) == 0 {
		t.Fatalf("expected seeded posting slots, got %d (err=%v)", len(slots), err)
	}

	nextSlot, err := store.NextAvailableQueueTime(fixedNow)
	if err != nil {
		t.Fatalf("next available queue time: %v", err)
	}
	if !nextSlot.After(fixedNow) {
		t.Fatalf("expected next slot (%v) to be after fixedNow (%v)", nextSlot, fixedNow)
	}

	// Save a due scheduled post and verify ClaimDueScheduledPosts claims it atomically.
	dueAt := fixedNow.Add(-5 * time.Minute).Format(time.RFC3339)
	saved, err := store.SavePost(db.Post{
		ID:               "post-due",
		Status:           db.PostStatusScheduled,
		MasterText:       "Scheduled launch post",
		TargetAccountIDs: []string{"acc-1"},
		Tags:             []string{"Launch"},
		ScheduledAt:      dueAt,
	})
	if err != nil {
		t.Fatalf("save scheduled post: %v", err)
	}
	if saved.Status != db.PostStatusScheduled {
		t.Fatalf("expected scheduled status, got %q", saved.Status)
	}

	claimed, err := store.ClaimDueScheduledPosts(fixedNow)
	if err != nil {
		t.Fatalf("claim due posts: %v", err)
	}
	if len(claimed) != 1 || claimed[0].ID != "post-due" {
		t.Fatalf("expected 1 claimed post, got %#v", claimed)
	}

	// Second claim at the same timestamp must return 0 since status is now publishing.
	claimedAgain, err := store.ClaimDueScheduledPosts(fixedNow)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(claimedAgain) != 0 {
		t.Fatalf("expected 0 claimed posts on second pass, got %d", len(claimedAgain))
	}

	finalized, err := store.FinalizePublishedPost("post-due", db.PostStatusSent, []db.PostResult{
		{
			ID:          "res-1",
			AccountID:   "acc-1",
			Platform:    "discord",
			AccountName: "#announcements",
			TextSent:    "Scheduled launch post",
			Status:      "success",
		},
	})
	if err != nil {
		t.Fatalf("finalize post: %v", err)
	}
	if finalized.Status != db.PostStatusSent || len(finalized.Results) != 1 {
		t.Fatalf("unexpected finalized post: %#v", finalized)
	}
}

func TestQueuePauseReorderShuffleAndHashtagGroups(t *testing.T) {
	store := openTestStore(t)

	if store.IsQueuePaused() {
		t.Fatalf("expected queue to be active by default")
	}
	if err := store.SetQueuePaused(true); err != nil {
		t.Fatalf("pause queue: %v", err)
	}
	if !store.IsQueuePaused() {
		t.Fatalf("expected queue to be paused")
	}

	t1 := "2026-11-24T09:00:00Z"
	t2 := "2026-11-24T17:00:00Z"
	_, err := store.SavePost(db.Post{
		ID:           "q1",
		Status:       db.PostStatusScheduled,
		MasterText:   "First",
		FirstComment: "Link in bio",
		ScheduledAt:  t1,
	})
	if err != nil {
		t.Fatalf("save q1: %v", err)
	}
	_, err = store.SavePost(db.Post{
		ID:          "q2",
		Status:      db.PostStatusScheduled,
		MasterText:  "Second",
		ScheduledAt: t2,
	})
	if err != nil {
		t.Fatalf("save q2: %v", err)
	}

	if err := store.ReorderScheduledPost("q2", "up"); err != nil {
		t.Fatalf("reorder q2 up: %v", err)
	}
	p2, err := store.GetPost("q2")
	if err != nil || p2.ScheduledAt != t1 {
		t.Fatalf("expected q2 scheduledAt=%s, got %q (err=%v)", t1, p2.ScheduledAt, err)
	}

	if err := store.ShuffleScheduledPosts(); err != nil {
		t.Fatalf("shuffle scheduled posts: %v", err)
	}

	groups, err := store.ListHashtagGroups()
	if err != nil || len(groups) < 3 {
		t.Fatalf("expected seeded hashtag groups, got %d (err=%v)", len(groups), err)
	}
}
