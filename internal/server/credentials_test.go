package server

import (
	"encoding/json"
	"testing"

	"teepost/internal/db"
)

func TestSanitizeAccountRemovesSecrets(t *testing.T) {
	acc := db.Account{
		Platform:        "bluesky",
		CredentialsJSON: `{"identifier":"alice.bsky.social","appPassword":"supersecret","pdsUrl":"https://bsky.social"}`,
	}

	got := sanitizeAccount(acc)

	var creds map[string]string
	if err := json.Unmarshal(got.Credentials, &creds); err != nil {
		t.Fatalf("decode sanitized credentials: %v", err)
	}
	if _, ok := creds["appPassword"]; ok {
		t.Fatal("appPassword must not be exposed")
	}
	if creds["identifier"] != "alice.bsky.social" {
		t.Fatalf("public identifier lost: %q", creds["identifier"])
	}
}

func TestSanitizeAccountStripsEveryPlatformSecret(t *testing.T) {
	cases := []struct {
		platform string
		raw      string
		secret   string
	}{
		{"x", `{"apiKey":"a","apiSecret":"b","accessToken":"c","accessTokenSecret":"d"}`, "apiKey"},
		{"telegram", `{"botToken":"a","chatId":"@c"}`, "botToken"},
		{"discord", `{"webhookUrl":"https://discord.com/api/webhooks/1/tok"}`, "webhookUrl"},
		{"mastodon", `{"instanceUrl":"https://mastodon.social","accessToken":"a"}`, "accessToken"},
	}

	for _, tc := range cases {
		t.Run(tc.platform, func(t *testing.T) {
			got := sanitizeAccount(db.Account{Platform: tc.platform, CredentialsJSON: tc.raw})
			var creds map[string]string
			if err := json.Unmarshal(got.Credentials, &creds); err != nil {
				t.Fatalf("decode sanitized credentials: %v", err)
			}
			if _, ok := creds[tc.secret]; ok {
				t.Fatalf("%s must not be exposed", tc.secret)
			}
		})
	}
}

func TestMergeAccountCredentialsKeepsBlankSecretsAndUpdatesFields(t *testing.T) {
	existing := `{"identifier":"alice.bsky.social","appPassword":"old-secret","pdsUrl":"https://bsky.social"}`
	incoming := json.RawMessage(`{"identifier":"alice.bsky.social","appPassword":"","pdsUrl":""}`)

	merged, err := mergeAccountCredentials("bluesky", existing, incoming)
	if err != nil {
		t.Fatalf("merge credentials: %v", err)
	}

	var creds map[string]string
	if err := json.Unmarshal(merged, &creds); err != nil {
		t.Fatalf("decode merged credentials: %v", err)
	}
	if creds["appPassword"] != "old-secret" {
		t.Fatalf("blank secret must keep existing value, got %q", creds["appPassword"])
	}
	if creds["pdsUrl"] != "" {
		t.Fatalf("blank non-secret field must clear value, got %q", creds["pdsUrl"])
	}
}

func TestMergeAccountCredentialsReplacesProvidedSecret(t *testing.T) {
	existing := `{"botToken":"old","chatId":"@old"}`
	incoming := json.RawMessage(`{"botToken":"new","chatId":"@new"}`)

	merged, err := mergeAccountCredentials("telegram", existing, incoming)
	if err != nil {
		t.Fatalf("merge credentials: %v", err)
	}

	var creds map[string]string
	if err := json.Unmarshal(merged, &creds); err != nil {
		t.Fatalf("decode merged credentials: %v", err)
	}
	if creds["botToken"] != "new" {
		t.Fatalf("provided secret must replace existing, got %q", creds["botToken"])
	}
	if creds["chatId"] != "@new" {
		t.Fatalf("public field must update, got %q", creds["chatId"])
	}
}
