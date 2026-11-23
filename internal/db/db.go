package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mrand "math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var ErrNotFound = errors.New("record not found")

// PostStatus represents the lifecycle state of a post.
const (
	PostStatusIdea       = "idea"
	PostStatusDraft      = "draft"
	PostStatusScheduled  = "scheduled"
	PostStatusPublishing = "publishing"
	PostStatusSent       = "sent"
	PostStatusFailed     = "failed"
)

// Account represents a configured social media target channel.
type Account struct {
	ID              string          `json:"id"`
	Platform        string          `json:"platform"`
	Name            string          `json:"name"`
	Enabled         bool            `json:"enabled"`
	CredentialsJSON string          `json:"-"`
	Credentials     json.RawMessage `json:"credentials"`
	CreatedAt       string          `json:"createdAt"`
	UpdatedAt       string          `json:"updatedAt"`
}

// MediaItem stores metadata about an uploaded image attached to a post.
type MediaItem struct {
	ID         string `json:"id"`
	StoredName string `json:"storedName"`
	Filename   string `json:"filename"`
	MimeType   string `json:"mimeType"`
	SizeBytes  int64  `json:"sizeBytes"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	AltText    string `json:"altText"`
	URL        string `json:"url"`
}

// Post represents an idea, draft, scheduled post, or sent message across social channels.
type Post struct {
	ID                  string            `json:"id"`
	Status              string            `json:"status"`
	MasterText          string            `json:"masterText"`
	FirstComment        string            `json:"firstComment"`
	CustomizePerNetwork bool              `json:"customizePerNetwork"`
	CustomTexts         map[string]string `json:"customTexts"`
	Media               []MediaItem       `json:"media"`
	TargetAccountIDs    []string          `json:"targetAccountIds"`
	Tags                []string          `json:"tags"`
	ScheduledAt         string            `json:"scheduledAt"`
	Results             []PostResult      `json:"results"`
	CreatedAt           string            `json:"createdAt"`
	UpdatedAt           string            `json:"updatedAt"`
}

// PostResult captures the delivery outcome for one account on a post.
type PostResult struct {
	ID           string `json:"id"`
	PostID       string `json:"postId"`
	AccountID    string `json:"accountId"`
	Platform     string `json:"platform"`
	AccountName  string `json:"accountName"`
	TextSent     string `json:"textSent"`
	Status       string `json:"status"` // "success" | "error"
	ExternalID   string `json:"externalId"`
	ExternalURL  string `json:"externalUrl"`
	ErrorMessage string `json:"errorMessage"`
	PublishedAt  string `json:"publishedAt"`
}

// PostingSlot represents a recurring weekly time slot in the Buffer-style posting schedule.
type PostingSlot struct {
	ID        string `json:"id"`
	Weekday   int    `json:"weekday"`  // 0 (Sunday) .. 6 (Saturday)
	TimeHHMM  string `json:"timeHhmm"` // "09:00", "17:30"
	CreatedAt string `json:"createdAt"`
}

// HashtagGroup represents a reusable preset of hashtags in the Composer.
type HashtagGroup struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Hashtags string `json:"hashtags"`
}

// DisplayPreferences stores user-configured timezone and clock format.
type DisplayPreferences struct {
	Timezone   string `json:"timezone"`   // IANA timezone e.g. "Europe/Warsaw", "America/New_York", "UTC", or "Local"
	TimeFormat string `json:"timeFormat"` // "24h" or "12h"
}

// Store wraps SQLite database operations for Teepost.
type Store struct {
	sqlDB *sql.DB
}

// Open initializes the SQLite database, ensures parent directories exist, and runs migrations.
func Open(dbPath string) (*Store, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("create db directory: %w", err)
		}
	}

	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)

	store := &Store{sqlDB: sqlDB}
	if err := store.migrate(); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return store, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.sqlDB.Close()
}

// Ping verifies database connectivity for readiness probes.
func (s *Store) Ping(ctx context.Context) error {
	return s.sqlDB.PingContext(ctx)
}

func (s *Store) migrate() error {
	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA foreign_keys=ON;`,
		`PRAGMA busy_timeout=5000;`,
	}
	for _, p := range pragmas {
		if _, err := s.sqlDB.Exec(p); err != nil {
			return fmt.Errorf("exec pragma %q: %w", p, err)
		}
	}

	schema := `
	CREATE TABLE IF NOT EXISTS accounts (
		id TEXT PRIMARY KEY,
		platform TEXT NOT NULL,
		name TEXT NOT NULL,
		enabled INTEGER NOT NULL DEFAULT 1,
		credentials_json TEXT NOT NULL,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS posts (
		id TEXT PRIMARY KEY,
		status TEXT NOT NULL DEFAULT 'sent',
		master_text TEXT NOT NULL,
		first_comment TEXT NOT NULL DEFAULT '',
		customize_per_network INTEGER NOT NULL DEFAULT 0,
		custom_texts_json TEXT NOT NULL DEFAULT '{}',
		media_json TEXT NOT NULL DEFAULT '[]',
		target_account_ids_json TEXT NOT NULL DEFAULT '[]',
		tags_json TEXT NOT NULL DEFAULT '[]',
		scheduled_at TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS post_results (
		id TEXT PRIMARY KEY,
		post_id TEXT NOT NULL,
		account_id TEXT NOT NULL,
		platform TEXT NOT NULL,
		account_name TEXT NOT NULL,
		text_sent TEXT NOT NULL,
		status TEXT NOT NULL,
		external_id TEXT NOT NULL DEFAULT '',
		external_url TEXT NOT NULL DEFAULT '',
		error_message TEXT NOT NULL DEFAULT '',
		published_at TEXT NOT NULL,
		FOREIGN KEY(post_id) REFERENCES posts(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_posts_created_at ON posts(created_at DESC);
	CREATE INDEX IF NOT EXISTS idx_post_results_post_id ON post_results(post_id);

	CREATE TABLE IF NOT EXISTS settings (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);

	CREATE TABLE IF NOT EXISTS posting_slots (
		id TEXT PRIMARY KEY,
		weekday INTEGER NOT NULL,
		time_hhmm TEXT NOT NULL,
		created_at TEXT NOT NULL
	);
	`
	if _, err := s.sqlDB.Exec(schema); err != nil {
		return fmt.Errorf("run schema migrations: %w", err)
	}

	alterCols := []string{
		`ALTER TABLE posts ADD COLUMN status TEXT NOT NULL DEFAULT 'sent'`,
		`ALTER TABLE posts ADD COLUMN first_comment TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE posts ADD COLUMN target_account_ids_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE posts ADD COLUMN tags_json TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE posts ADD COLUMN scheduled_at TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE posts ADD COLUMN updated_at TEXT NOT NULL DEFAULT ''`,
	}
	for _, stmt := range alterCols {
		_, err := s.sqlDB.Exec(stmt)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("alter posts table (%s): %w", stmt, err)
		}
	}

	if _, err := s.sqlDB.Exec(`CREATE INDEX IF NOT EXISTS idx_posts_status_scheduled ON posts(status, scheduled_at)`); err != nil {
		return fmt.Errorf("create scheduled index: %w", err)
	}

	if _, err := s.GetSetting("slots_seeded"); errors.Is(err, ErrNotFound) {
		var count int
		_ = s.sqlDB.QueryRow(`SELECT COUNT(*) FROM posting_slots`).Scan(&count)
		if count == 0 {
			defaults := []PostingSlot{
				{Weekday: 1, TimeHHMM: "09:00"},
				{Weekday: 1, TimeHHMM: "17:00"},
				{Weekday: 2, TimeHHMM: "09:00"},
				{Weekday: 3, TimeHHMM: "09:00"},
				{Weekday: 3, TimeHHMM: "17:00"},
				{Weekday: 4, TimeHHMM: "09:00"},
				{Weekday: 5, TimeHHMM: "09:00"},
				{Weekday: 5, TimeHHMM: "17:00"},
			}
			if _, err := s.ReplacePostingSlots(defaults); err != nil {
				return fmt.Errorf("seed default posting slots: %w", err)
			}
		}
		_ = s.SetSetting("slots_seeded", "1")
	}

	return nil
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func newID() string {
	buf := make([]byte, 8)
	_, _ = rand.Read(buf)
	return hex.EncodeToString(buf)
}

// ListAccounts returns all accounts ordered by creation date ascending.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.sqlDB.Query(`
		SELECT id, platform, name, enabled, credentials_json, created_at, updated_at
		FROM accounts
		ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list accounts: %w", err)
	}
	defer rows.Close()

	accounts := make([]Account, 0)
	for rows.Next() {
		var a Account
		var enabledInt int
		if err := rows.Scan(&a.ID, &a.Platform, &a.Name, &enabledInt, &a.CredentialsJSON, &a.CreatedAt, &a.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan account: %w", err)
		}
		a.Enabled = enabledInt == 1
		a.Credentials = json.RawMessage(a.CredentialsJSON)
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate accounts: %w", err)
	}
	return accounts, nil
}

// GetAccount fetches a single account by ID.
func (s *Store) GetAccount(id string) (Account, error) {
	var a Account
	var enabledInt int
	err := s.sqlDB.QueryRow(`
		SELECT id, platform, name, enabled, credentials_json, created_at, updated_at
		FROM accounts
		WHERE id = ?
	`, id).Scan(&a.ID, &a.Platform, &a.Name, &enabledInt, &a.CredentialsJSON, &a.CreatedAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, fmt.Errorf("get account: %w", err)
	}
	a.Enabled = enabledInt == 1
	a.Credentials = json.RawMessage(a.CredentialsJSON)
	return a, nil
}

// CreateAccount inserts a new account.
func (s *Store) CreateAccount(a Account) (Account, error) {
	now := nowISO()
	if a.CreatedAt == "" {
		a.CreatedAt = now
	}
	a.UpdatedAt = now
	if a.CredentialsJSON == "" && len(a.Credentials) > 0 {
		a.CredentialsJSON = string(a.Credentials)
	}
	if a.CredentialsJSON == "" {
		a.CredentialsJSON = "{}"
	}
	a.Credentials = json.RawMessage(a.CredentialsJSON)

	_, err := s.sqlDB.Exec(`
		INSERT INTO accounts (id, platform, name, enabled, credentials_json, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, a.ID, a.Platform, a.Name, boolToInt(a.Enabled), a.CredentialsJSON, a.CreatedAt, a.UpdatedAt)
	if err != nil {
		return Account{}, fmt.Errorf("insert account: %w", err)
	}
	return a, nil
}

// UpdateAccount updates an existing account.
func (s *Store) UpdateAccount(a Account) (Account, error) {
	a.UpdatedAt = nowISO()
	if a.CredentialsJSON == "" && len(a.Credentials) > 0 {
		a.CredentialsJSON = string(a.Credentials)
	}
	if a.CredentialsJSON == "" {
		a.CredentialsJSON = "{}"
	}
	a.Credentials = json.RawMessage(a.CredentialsJSON)

	res, err := s.sqlDB.Exec(`
		UPDATE accounts
		SET platform = ?, name = ?, enabled = ?, credentials_json = ?, updated_at = ?
		WHERE id = ?
	`, a.Platform, a.Name, boolToInt(a.Enabled), a.CredentialsJSON, a.UpdatedAt, a.ID)
	if err != nil {
		return Account{}, fmt.Errorf("update account: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return Account{}, ErrNotFound
	}
	return s.GetAccount(a.ID)
}

// DeleteAccount removes an account by ID.
func (s *Store) DeleteAccount(id string) error {
	res, err := s.sqlDB.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete account: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

func normalizePostFields(p *Post) {
	now := nowISO()
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	if p.UpdatedAt == "" {
		p.UpdatedAt = now
	}
	if p.Status == "" {
		p.Status = PostStatusSent
	}
	if p.CustomTexts == nil {
		p.CustomTexts = map[string]string{}
	}
	if p.Media == nil {
		p.Media = []MediaItem{}
	}
	if p.TargetAccountIDs == nil {
		p.TargetAccountIDs = []string{}
	}
	if p.Tags == nil {
		p.Tags = []string{}
	}
}

// CreatePostWithResults persists a post and its per-account delivery results in a transaction.
func (s *Store) CreatePostWithResults(p Post) (Post, error) {
	normalizePostFields(&p)

	customTextsBytes, err := json.Marshal(p.CustomTexts)
	if err != nil {
		return Post{}, fmt.Errorf("marshal custom texts: %w", err)
	}
	mediaBytes, err := json.Marshal(p.Media)
	if err != nil {
		return Post{}, fmt.Errorf("marshal media: %w", err)
	}
	targetsBytes, err := json.Marshal(p.TargetAccountIDs)
	if err != nil {
		return Post{}, fmt.Errorf("marshal target accounts: %w", err)
	}
	tagsBytes, err := json.Marshal(p.Tags)
	if err != nil {
		return Post{}, fmt.Errorf("marshal tags: %w", err)
	}

	tx, err := s.sqlDB.Begin()
	if err != nil {
		return Post{}, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	_, err = tx.Exec(`
		INSERT INTO posts (
			id, status, master_text, first_comment, customize_per_network, custom_texts_json,
			media_json, target_account_ids_json, tags_json, scheduled_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.Status, p.MasterText, p.FirstComment, boolToInt(p.CustomizePerNetwork), string(customTextsBytes),
		string(mediaBytes), string(targetsBytes), string(tagsBytes), p.ScheduledAt, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return Post{}, fmt.Errorf("insert post: %w", err)
	}

	for _, r := range p.Results {
		if r.PublishedAt == "" {
			r.PublishedAt = nowISO()
		}
		_, err = tx.Exec(`
			INSERT INTO post_results (
				id, post_id, account_id, platform, account_name, text_sent,
				status, external_id, external_url, error_message, published_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, r.ID, p.ID, r.AccountID, r.Platform, r.AccountName, r.TextSent,
			r.Status, r.ExternalID, r.ExternalURL, r.ErrorMessage, r.PublishedAt)
		if err != nil {
			return Post{}, fmt.Errorf("insert post result: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Post{}, fmt.Errorf("commit post tx: %w", err)
	}
	return s.GetPost(p.ID)
}

// SavePost creates or updates a draft, idea, or scheduled post without publishing it immediately.
func (s *Store) SavePost(p Post) (Post, error) {
	if p.ID == "" {
		p.ID = newID()
	}
	normalizePostFields(&p)
	p.UpdatedAt = nowISO()

	customTextsBytes, err := json.Marshal(p.CustomTexts)
	if err != nil {
		return Post{}, fmt.Errorf("marshal custom texts: %w", err)
	}
	mediaBytes, err := json.Marshal(p.Media)
	if err != nil {
		return Post{}, fmt.Errorf("marshal media: %w", err)
	}
	targetsBytes, err := json.Marshal(p.TargetAccountIDs)
	if err != nil {
		return Post{}, fmt.Errorf("marshal target accounts: %w", err)
	}
	tagsBytes, err := json.Marshal(p.Tags)
	if err != nil {
		return Post{}, fmt.Errorf("marshal tags: %w", err)
	}

	_, err = s.sqlDB.Exec(`
		INSERT INTO posts (
			id, status, master_text, first_comment, customize_per_network, custom_texts_json,
			media_json, target_account_ids_json, tags_json, scheduled_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			status = excluded.status,
			master_text = excluded.master_text,
			first_comment = excluded.first_comment,
			customize_per_network = excluded.customize_per_network,
			custom_texts_json = excluded.custom_texts_json,
			media_json = excluded.media_json,
			target_account_ids_json = excluded.target_account_ids_json,
			tags_json = excluded.tags_json,
			scheduled_at = excluded.scheduled_at,
			updated_at = excluded.updated_at
	`, p.ID, p.Status, p.MasterText, p.FirstComment, boolToInt(p.CustomizePerNetwork), string(customTextsBytes),
		string(mediaBytes), string(targetsBytes), string(tagsBytes), p.ScheduledAt, p.CreatedAt, p.UpdatedAt)
	if err != nil {
		return Post{}, fmt.Errorf("save post: %w", err)
	}

	return s.GetPost(p.ID)
}

// ClaimDueScheduledPosts atomically marks scheduled posts whose scheduledAt <= nowUTC as publishing.
func (s *Store) ClaimDueScheduledPosts(nowUTC time.Time) ([]Post, error) {
	tx, err := s.sqlDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin claim tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`
		SELECT id, status, master_text, first_comment, customize_per_network, custom_texts_json,
			media_json, target_account_ids_json, tags_json, scheduled_at, created_at, updated_at
		FROM posts
		WHERE status = ? AND scheduled_at != ''
		ORDER BY scheduled_at ASC
	`, PostStatusScheduled)
	if err != nil {
		return nil, fmt.Errorf("query scheduled posts: %w", err)
	}

	due := make([]Post, 0)
	for rows.Next() {
		p, err := scanPostRow(rows)
		if err != nil {
			_ = rows.Close()
			return nil, err
		}
		schedTime, parseErr := time.Parse(time.RFC3339Nano, p.ScheduledAt)
		if parseErr != nil {
			schedTime, parseErr = time.Parse(time.RFC3339, p.ScheduledAt)
		}
		if parseErr != nil {
			continue
		}
		if !schedTime.After(nowUTC) {
			due = append(due, p)
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}

	nowStr := nowISO()
	for i := range due {
		if _, err := tx.Exec(`UPDATE posts SET status = ?, updated_at = ? WHERE id = ?`, PostStatusPublishing, nowStr, due[i].ID); err != nil {
			return nil, fmt.Errorf("claim scheduled post %s: %w", due[i].ID, err)
		}
		due[i].Status = PostStatusPublishing
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit claim tx: %w", err)
	}
	return due, nil
}

// FinalizePublishedPost updates a post's status and replaces its delivery results.
func (s *Store) FinalizePublishedPost(postID string, status string, results []PostResult) (Post, error) {
	tx, err := s.sqlDB.Begin()
	if err != nil {
		return Post{}, fmt.Errorf("begin finalize tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	nowStr := nowISO()
	res, err := tx.Exec(`UPDATE posts SET status = ?, updated_at = ? WHERE id = ?`, status, nowStr, postID)
	if err != nil {
		return Post{}, fmt.Errorf("update post status: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return Post{}, ErrNotFound
	}

	if _, err := tx.Exec(`DELETE FROM post_results WHERE post_id = ?`, postID); err != nil {
		return Post{}, fmt.Errorf("clear old post results: %w", err)
	}

	for _, r := range results {
		if r.PublishedAt == "" {
			r.PublishedAt = nowStr
		}
		_, err = tx.Exec(`
			INSERT INTO post_results (
				id, post_id, account_id, platform, account_name, text_sent,
				status, external_id, external_url, error_message, published_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, r.ID, postID, r.AccountID, r.Platform, r.AccountName, r.TextSent,
			r.Status, r.ExternalID, r.ExternalURL, r.ErrorMessage, r.PublishedAt)
		if err != nil {
			return Post{}, fmt.Errorf("insert finalized post result: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Post{}, fmt.Errorf("commit finalize tx: %w", err)
	}
	return s.GetPost(postID)
}

// UpdatePostResult updates or inserts a result row for a given post and account.
func (s *Store) UpdatePostResult(r PostResult) error {
	if r.PublishedAt == "" {
		r.PublishedAt = nowISO()
	}
	res, err := s.sqlDB.Exec(`
		UPDATE post_results
		SET status = ?, external_id = ?, external_url = ?, error_message = ?, published_at = ?, text_sent = ?
		WHERE post_id = ? AND account_id = ?
	`, r.Status, r.ExternalID, r.ExternalURL, r.ErrorMessage, r.PublishedAt, r.TextSent, r.PostID, r.AccountID)
	if err != nil {
		return fmt.Errorf("update post result: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		_, err = s.sqlDB.Exec(`
			INSERT INTO post_results (
				id, post_id, account_id, platform, account_name, text_sent,
				status, external_id, external_url, error_message, published_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, r.ID, r.PostID, r.AccountID, r.Platform, r.AccountName, r.TextSent,
			r.Status, r.ExternalID, r.ExternalURL, r.ErrorMessage, r.PublishedAt)
		if err != nil {
			return fmt.Errorf("insert post result on retry: %w", err)
		}
	}
	if r.Status == "success" {
		_, _ = s.sqlDB.Exec(`UPDATE posts SET status = ?, updated_at = ? WHERE id = ?`, PostStatusSent, nowISO(), r.PostID)
	}
	return nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanPostRow(scanner rowScanner) (Post, error) {
	var p Post
	var customizeInt int
	var customTextsJSON, mediaJSON, targetsJSON, tagsJSON string

	err := scanner.Scan(
		&p.ID, &p.Status, &p.MasterText, &p.FirstComment, &customizeInt, &customTextsJSON,
		&mediaJSON, &targetsJSON, &tagsJSON, &p.ScheduledAt, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return Post{}, err
	}

	p.CustomizePerNetwork = customizeInt == 1
	p.CustomTexts = map[string]string{}
	_ = json.Unmarshal([]byte(customTextsJSON), &p.CustomTexts)
	p.Media = []MediaItem{}
	_ = json.Unmarshal([]byte(mediaJSON), &p.Media)
	p.TargetAccountIDs = []string{}
	_ = json.Unmarshal([]byte(targetsJSON), &p.TargetAccountIDs)
	p.Tags = []string{}
	_ = json.Unmarshal([]byte(tagsJSON), &p.Tags)
	p.Results = make([]PostResult, 0)
	if p.UpdatedAt == "" {
		p.UpdatedAt = p.CreatedAt
	}
	return p, nil
}

// GetPost returns a single post with its results.
func (s *Store) GetPost(id string) (Post, error) {
	row := s.sqlDB.QueryRow(`
		SELECT id, status, master_text, first_comment, customize_per_network, custom_texts_json,
			media_json, target_account_ids_json, tags_json, scheduled_at, created_at, updated_at
		FROM posts
		WHERE id = ?
	`, id)
	p, err := scanPostRow(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Post{}, ErrNotFound
	}
	if err != nil {
		return Post{}, fmt.Errorf("get post: %w", err)
	}

	results, err := s.listPostResults(p.ID)
	if err != nil {
		return Post{}, err
	}
	p.Results = results
	return p, nil
}

func (s *Store) listPostResults(postID string) ([]PostResult, error) {
	rows, err := s.sqlDB.Query(`
		SELECT id, post_id, account_id, platform, account_name, text_sent,
			status, external_id, external_url, error_message, published_at
		FROM post_results
		WHERE post_id = ?
		ORDER BY rowid ASC
	`, postID)
	if err != nil {
		return nil, fmt.Errorf("list post results: %w", err)
	}
	defer rows.Close()

	results := make([]PostResult, 0)
	for rows.Next() {
		var r PostResult
		if err := rows.Scan(
			&r.ID, &r.PostID, &r.AccountID, &r.Platform, &r.AccountName, &r.TextSent,
			&r.Status, &r.ExternalID, &r.ExternalURL, &r.ErrorMessage, &r.PublishedAt,
		); err != nil {
			return nil, fmt.Errorf("scan post result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate post results: %w", err)
	}
	return results, nil
}

// ListPosts returns posts in reverse chronological order with their results.
func (s *Store) ListPosts(limit int) ([]Post, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.sqlDB.Query(`
		SELECT id, status, master_text, first_comment, customize_per_network, custom_texts_json,
			media_json, target_account_ids_json, tags_json, scheduled_at, created_at, updated_at
		FROM posts
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("list posts: %w", err)
	}
	defer rows.Close()

	posts := make([]Post, 0)
	for rows.Next() {
		p, err := scanPostRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan post: %w", err)
		}
		posts = append(posts, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posts: %w", err)
	}

	for i := range posts {
		results, err := s.listPostResults(posts[i].ID)
		if err != nil {
			return nil, err
		}
		posts[i].Results = results
	}
	return posts, nil
}

// DeletePost removes a post and its cascaded post_results.
func (s *Store) DeletePost(id string) error {
	res, err := s.sqlDB.Exec(`DELETE FROM posts WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete post: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSetting returns a setting value by key, or ErrNotFound if not set.
func (s *Store) GetSetting(key string) (string, error) {
	var val string
	err := s.sqlDB.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&val)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get setting %q: %w", key, err)
	}
	return val, nil
}

// SetSetting inserts or updates a setting key/value pair.
func (s *Store) SetSetting(key, value string) error {
	_, err := s.sqlDB.Exec(`
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at
	`, key, value, nowISO())
	if err != nil {
		return fmt.Errorf("set setting %q: %w", key, err)
	}
	return nil
}

// DeleteSetting removes a setting key from the database.
func (s *Store) DeleteSetting(key string) error {
	_, err := s.sqlDB.Exec(`DELETE FROM settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("delete setting %q: %w", key, err)
	}
	return nil
}

// IsQueuePaused reports whether background queue publishing is paused.
func (s *Store) IsQueuePaused() bool {
	val, err := s.GetSetting("queue_paused")
	if err != nil {
		return false
	}
	return val == "1"
}

// SetQueuePaused enables or disables queue pausing.
func (s *Store) SetQueuePaused(paused bool) error {
	if paused {
		return s.SetSetting("queue_paused", "1")
	}
	return s.SetSetting("queue_paused", "0")
}

// ReorderScheduledPost swaps a scheduled post's timestamp with its adjacent neighbor ("up" or "down").
func (s *Store) ReorderScheduledPost(postID string, direction string) error {
	rows, err := s.sqlDB.Query(`
		SELECT id, scheduled_at FROM posts
		WHERE status = ? AND scheduled_at != ''
		ORDER BY scheduled_at ASC
	`, PostStatusScheduled)
	if err != nil {
		return fmt.Errorf("query scheduled order: %w", err)
	}

	type item struct {
		id          string
		scheduledAt string
	}
	items := make([]item, 0)
	for rows.Next() {
		var it item
		if err := rows.Scan(&it.id, &it.scheduledAt); err != nil {
			_ = rows.Close()
			return err
		}
		items = append(items, it)
	}
	if err := rows.Close(); err != nil {
		return err
	}

	idx := -1
	for i, it := range items {
		if it.id == postID {
			idx = i
			break
		}
	}
	if idx == -1 {
		return ErrNotFound
	}

	swapIdx := idx - 1
	if direction == "down" {
		swapIdx = idx + 1
	}
	if swapIdx < 0 || swapIdx >= len(items) {
		return nil
	}

	tx, err := s.sqlDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	nowStr := nowISO()
	if _, err := tx.Exec(`UPDATE posts SET scheduled_at = ?, updated_at = ? WHERE id = ?`, items[swapIdx].scheduledAt, nowStr, items[idx].id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE posts SET scheduled_at = ?, updated_at = ? WHERE id = ?`, items[idx].scheduledAt, nowStr, items[swapIdx].id); err != nil {
		return err
	}
	return tx.Commit()
}

// ShuffleScheduledPosts randomly redistributes the scheduled times across all queued posts.
func (s *Store) ShuffleScheduledPosts() error {
	rows, err := s.sqlDB.Query(`
		SELECT id, scheduled_at FROM posts
		WHERE status = ? AND scheduled_at != ''
		ORDER BY scheduled_at ASC
	`, PostStatusScheduled)
	if err != nil {
		return fmt.Errorf("query scheduled shuffle: %w", err)
	}

	ids := make([]string, 0)
	times := make([]string, 0)
	for rows.Next() {
		var id, sched string
		if err := rows.Scan(&id, &sched); err != nil {
			_ = rows.Close()
			return err
		}
		ids = append(ids, id)
		times = append(times, sched)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(ids) < 2 {
		return nil
	}

	mrand.Shuffle(len(times), func(i, j int) {
		times[i], times[j] = times[j], times[i]
	})

	tx, err := s.sqlDB.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	nowStr := nowISO()
	for i, id := range ids {
		if _, err := tx.Exec(`UPDATE posts SET scheduled_at = ?, updated_at = ? WHERE id = ?`, times[i], nowStr, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ListHashtagGroups returns saved hashtag group presets, seeding defaults on first access.
func (s *Store) ListHashtagGroups() ([]HashtagGroup, error) {
	raw, err := s.GetSetting("hashtag_groups")
	if errors.Is(err, ErrNotFound) {
		defaults := []HashtagGroup{
			{ID: newID(), Name: "Launch", Hashtags: "#launch #product #release"},
			{ID: newID(), Name: "Tech", Hashtags: "#opensource #selfhosted #golang"},
			{ID: newID(), Name: "Update", Hashtags: "#changelog #news #community"},
		}
		return s.SaveHashtagGroups(defaults)
	}
	if err != nil {
		return nil, err
	}
	var groups []HashtagGroup
	if err := json.Unmarshal([]byte(raw), &groups); err != nil {
		return []HashtagGroup{}, nil
	}
	return groups, nil
}

// SaveHashtagGroups persists the complete list of hashtag groups.
func (s *Store) SaveHashtagGroups(groups []HashtagGroup) ([]HashtagGroup, error) {
	cleaned := make([]HashtagGroup, 0, len(groups))
	for _, g := range groups {
		name := strings.TrimSpace(g.Name)
		tags := strings.TrimSpace(g.Hashtags)
		if name == "" || tags == "" {
			continue
		}
		id := g.ID
		if id == "" {
			id = newID()
		}
		cleaned = append(cleaned, HashtagGroup{
			ID:       id,
			Name:     name,
			Hashtags: tags,
		})
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	if err := s.SetSetting("hashtag_groups", string(data)); err != nil {
		return nil, err
	}
	return cleaned, nil
}

// GetDisplayPreferences returns user-configured timezone and clock format (defaults: Local, 24h).
func (s *Store) GetDisplayPreferences() DisplayPreferences {
	tz, err := s.GetSetting("pref_timezone")
	if err != nil || strings.TrimSpace(tz) == "" {
		tz = "Local"
	}
	tf, err := s.GetSetting("pref_time_format")
	if err != nil || (tf != "12h" && tf != "24h") {
		tf = "24h"
	}
	return DisplayPreferences{
		Timezone:   tz,
		TimeFormat: tf,
	}
}

// SaveDisplayPreferences validates and persists timezone and clock format.
func (s *Store) SaveDisplayPreferences(prefs DisplayPreferences) (DisplayPreferences, error) {
	tz := strings.TrimSpace(prefs.Timezone)
	if tz == "" {
		tz = "Local"
	}
	if tz != "Local" {
		if _, err := time.LoadLocation(tz); err != nil {
			return DisplayPreferences{}, fmt.Errorf("invalid timezone %q", tz)
		}
	}
	tf := prefs.TimeFormat
	if tf != "12h" && tf != "24h" {
		tf = "24h"
	}
	if err := s.SetSetting("pref_timezone", tz); err != nil {
		return DisplayPreferences{}, err
	}
	if err := s.SetSetting("pref_time_format", tf); err != nil {
		return DisplayPreferences{}, err
	}
	return DisplayPreferences{Timezone: tz, TimeFormat: tf}, nil
}

// ResolveLocation returns the *time.Location matching user preferences (or time.Local).
func (s *Store) ResolveLocation() *time.Location {
	prefs := s.GetDisplayPreferences()
	if prefs.Timezone == "" || prefs.Timezone == "Local" {
		return time.Local
	}
	loc, err := time.LoadLocation(prefs.Timezone)
	if err != nil {
		return time.Local
	}
	return loc
}

// ListPostingSlots returns all recurring weekly posting schedule slots sorted by weekday and time.
func (s *Store) ListPostingSlots() ([]PostingSlot, error) {
	rows, err := s.sqlDB.Query(`
		SELECT id, weekday, time_hhmm, created_at
		FROM posting_slots
		ORDER BY weekday ASC, time_hhmm ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list posting slots: %w", err)
	}
	defer rows.Close()

	slots := make([]PostingSlot, 0)
	for rows.Next() {
		var slot PostingSlot
		if err := rows.Scan(&slot.ID, &slot.Weekday, &slot.TimeHHMM, &slot.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan posting slot: %w", err)
		}
		slots = append(slots, slot)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate posting slots: %w", err)
	}
	return slots, nil
}

// ReplacePostingSlots replaces the entire weekly queue schedule with the provided slots.
func (s *Store) ReplacePostingSlots(slots []PostingSlot) ([]PostingSlot, error) {
	tx, err := s.sqlDB.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin replace slots tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM posting_slots`); err != nil {
		return nil, fmt.Errorf("clear posting slots: %w", err)
	}

	nowStr := nowISO()
	seen := make(map[string]bool)
	for _, slot := range slots {
		if slot.Weekday < 0 || slot.Weekday > 6 {
			continue
		}
		if _, _, ok := parseHHMM(slot.TimeHHMM); !ok {
			continue
		}
		key := fmt.Sprintf("%d:%s", slot.Weekday, slot.TimeHHMM)
		if seen[key] {
			continue
		}
		seen[key] = true
		id := slot.ID
		if id == "" {
			id = newID()
		}
		if _, err := tx.Exec(`
			INSERT INTO posting_slots (id, weekday, time_hhmm, created_at)
			VALUES (?, ?, ?, ?)
		`, id, slot.Weekday, slot.TimeHHMM, nowStr); err != nil {
			return nil, fmt.Errorf("insert posting slot: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit replace slots tx: %w", err)
	}
	return s.ListPostingSlots()
}

func parseHHMM(hhmm string) (int, int, bool) {
	parts := strings.Split(strings.TrimSpace(hhmm), ":")
	if len(parts) != 2 {
		return 0, 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, 0, false
	}
	return h, m, true
}

// NextAvailableQueueTime computes the next available UTC timestamp based on weekly posting slots.
func (s *Store) NextAvailableQueueTime(now time.Time) (time.Time, error) {
	slots, err := s.ListPostingSlots()
	if err != nil {
		return time.Time{}, err
	}

	rows, err := s.sqlDB.Query(`SELECT scheduled_at FROM posts WHERE status = ? AND scheduled_at != ''`, PostStatusScheduled)
	if err != nil {
		return time.Time{}, fmt.Errorf("list scheduled times: %w", err)
	}
	defer rows.Close()

	occupied := make([]time.Time, 0)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		t, pErr := time.Parse(time.RFC3339Nano, raw)
		if pErr != nil {
			t, pErr = time.Parse(time.RFC3339, raw)
		}
		if pErr == nil {
			occupied = append(occupied, t.UTC())
		}
	}

	isFree := func(candidate time.Time) bool {
		cUTC := candidate.UTC()
		for _, occ := range occupied {
			diff := cUTC.Sub(occ)
			if diff < 0 {
				diff = -diff
			}
			if diff < 5*time.Minute {
				return false
			}
		}
		return true
	}

	loc := s.ResolveLocation()
	localNow := now.In(loc)

	if len(slots) == 0 {
		candidate := localNow.Add(2 * time.Hour).Truncate(15 * time.Minute).UTC()
		for !isFree(candidate) {
			candidate = candidate.Add(1 * time.Hour)
		}
		return candidate, nil
	}

	candidates := make([]time.Time, 0, 64)
	for dayOffset := 0; dayOffset < 90; dayOffset++ {
		day := localNow.AddDate(0, 0, dayOffset)
		wd := int(day.Weekday())
		for _, slot := range slots {
			if slot.Weekday != wd {
				continue
			}
			h, m, ok := parseHHMM(slot.TimeHHMM)
			if !ok {
				continue
			}
			cand := time.Date(day.Year(), day.Month(), day.Day(), h, m, 0, 0, loc)
			if cand.After(localNow.Add(1 * time.Minute)) {
				candidates = append(candidates, cand.UTC())
			}
		}
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool {
				return candidates[i].Before(candidates[j])
			})
			for _, c := range candidates {
				if isFree(c) {
					return c, nil
				}
			}
			candidates = candidates[:0]
		}
	}

	return localNow.Add(24 * time.Hour).UTC(), nil
}
