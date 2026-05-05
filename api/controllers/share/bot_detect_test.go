package share

import (
	"net/http/httptest"
	"testing"
)

func TestIsCrawler(t *testing.T) {
	cases := []struct {
		name string
		ua   string
		want bool
	}{
		// Bots we claim to support — each one is a regression guard
		// against someone accidentally pruning the crawlerUserAgents list.
		{"twitterbot", "Twitterbot/1.0", true},
		{"facebookexternalhit", "facebookexternalhit/1.1 (+http://www.facebook.com/externalhit_uatext.php)", true},
		{"facebot", "Facebot", true},
		{"slackbot-linkexpanding", "Slackbot-LinkExpanding 1.0 (+https://api.slack.com/robots)", true},
		{"slackbot-generic", "Slackbot 1.0 (+https://api.slack.com/robots)", true},
		{"discordbot", "Mozilla/5.0 (compatible; Discordbot/2.0; +https://discordapp.com)", true},
		{"telegrambot", "TelegramBot (like TwitterBot)", true},
		{"whatsapp", "WhatsApp/2.2148.8 N", true},
		{"linkedinbot", "LinkedInBot/1.0 (compatible; Mozilla/5.0; Jakarta Commons-HttpClient/3.1)", true},
		{"applebot", "Applebot/0.1 (+http://www.apple.com/go/applebot)", true},
		{"redditbot", "RedditBot/1.0", true},

		// Case-insensitive — UAs come in mixed case in the wild.
		{"uppercase twitterbot", "TWITTERBOT/1.0", true},

		// Ordinary browsers — must NOT trigger.
		{"chrome", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36", false},
		{"firefox", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10.15; rv:121.0) Gecko/20100101 Firefox/121.0", false},
		{"safari-iphone", "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15", false},

		// Edge cases.
		{"empty ua", "", false},
		{"random string", "curl/7.0.0", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/m/abcd1234", nil)
			if tc.ua != "" {
				req.Header.Set("User-Agent", tc.ua)
			}
			if got := IsCrawler(req); got != tc.want {
				t.Errorf("IsCrawler(%q) = %v, want %v", tc.ua, got, tc.want)
			}
		})
	}
}

func TestIsCrawler_MetaEscapeHatch(t *testing.T) {
	// `?_meta=1` always forces bot-mode regardless of UA — used for
	// manual debugging ("what would Twitter see?") and for platforms
	// like Slack that strip UA on their re-crawl pass.
	req := httptest.NewRequest("GET", "/m/abcd1234?_meta=1", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Chrome)")
	if !IsCrawler(req) {
		t.Error("expected ?_meta=1 to force crawler mode")
	}
}
