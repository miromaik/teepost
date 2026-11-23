package tests

import (
	"testing"

	"teepost/internal/publishers"
)

func TestParseFacets(t *testing.T) {
	cases := []struct {
		name     string
		text     string
		expected []string
	}{
		{
			name:     "multibyte UTF-8 offsets and trailing punctuation",
			text:     "Café 🚀 check https://example.com/test?a=1. #Teepost #GoLang!",
			expected: []string{"https://example.com/test?a=1", "#Teepost", "#GoLang"},
		},
		{
			name:     "ignores URL fragments and pure numeric tags",
			text:     "See https://example.com/docs#section and #123 plus #Release2026",
			expected: []string{"https://example.com/docs#section", "#Release2026"},
		},
		{
			name:     "empty text returns zero facets",
			text:     "",
			expected: []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			facets := publishers.ParseFacets(tc.text)
			if len(facets) != len(tc.expected) {
				t.Fatalf("expected %d facets, got %d: %#v", len(tc.expected), len(facets), facets)
			}
			for i, exp := range tc.expected {
				idx := facets[i]["index"].(map[string]int)
				got := tc.text[idx["byteStart"]:idx["byteEnd"]]
				if got != exp {
					t.Errorf("facet[%d] slice %q != expected %q", i, got, exp)
				}
			}
		})
	}
}

func TestNewPublisherValidation(t *testing.T) {
	cases := []struct {
		name      string
		platform  string
		credsJSON string
		wantErr   bool
	}{
		{
			name:      "valid discord webhook",
			platform:  "discord",
			credsJSON: `{"webhookUrl":"https://discord.com/api/webhooks/1/abc"}`,
			wantErr:   false,
		},
		{
			name:      "valid bluesky credentials",
			platform:  "bluesky",
			credsJSON: `{"identifier":"alice.bsky.social","appPassword":"abcd-efgh-ijkl-mnop"}`,
			wantErr:   false,
		},
		{
			name:      "valid telegram credentials",
			platform:  "telegram",
			credsJSON: `{"botToken":"123:ABC","chatId":"@channel"}`,
			wantErr:   false,
		},
		{
			name:      "valid mastodon credentials",
			platform:  "mastodon",
			credsJSON: `{"instanceUrl":"https://mastodon.social","accessToken":"tok"}`,
			wantErr:   false,
		},
		{
			name:      "valid x credentials",
			platform:  "x",
			credsJSON: `{"apiKey":"a","apiSecret":"b","accessToken":"c","accessTokenSecret":"d"}`,
			wantErr:   false,
		},
		{
			name:      "missing discord webhook",
			platform:  "discord",
			credsJSON: `{}`,
			wantErr:   true,
		},
		{
			name:      "unsupported platform",
			platform:  "threads",
			credsJSON: `{}`,
			wantErr:   true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pub, err := publishers.NewPublisher(tc.platform, tc.credsJSON)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if pub == nil {
				t.Fatalf("expected non-nil publisher")
			}
		})
	}
}
