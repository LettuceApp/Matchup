// Package share implements the "link preview" experience: short URLs,
// Open Graph metadata rendering, and dynamically generated preview
// images for matchups and brackets.
//
// Flow:
//
//	Human paste  →  /m/{short_id}  →  302 to /users/{u}/matchup/{id}?ref=…
//	Bot paste    →  /m/{short_id}  →  HTML with <meta og:*> tags
//	Crawler      →  /og/m/{id}.png →  rendered PNG (cached in Redis)
package share

import (
	"net/http"
	"strings"
)

// crawlerUserAgents is the set of lowercased User-Agent substrings
// belonging to known link-preview crawlers — 2026 vintage. We match on
// substring, not prefix, because many bots append identifying strings
// at various positions. Any substring match means "treat as bot".
//
// Keep this list sorted for readability only; order doesn't affect
// correctness. Add freely — false positives just serve slightly leaner
// pages, which is harmless.
var crawlerUserAgents = []string{
	"applebot",             // iMessage / Spotlight / App Search
	"bluesky",              // Bluesky link preview
	"discordbot",           // Discord
	"embedly",              // Embedly (generic embed service)
	"facebookexternalhit",  // Facebook, Instagram, WhatsApp
	"facebot",              // Facebook new
	"iframely",             // Iframely (generic embed service)
	"line-poker",           // LINE messenger
	"linkedinbot",          // LinkedIn
	"mastodon",             // Mastodon link preview
	"pinterest",            // Pinterest
	"redditbot",            // Reddit
	"skypeuripreview",      // Skype
	"slackbot",             // Slack (all variants)
	"snapchat",             // Snapchat
	"telegrambot",          // Telegram
	"threads",              // Threads (Meta)
	"twitterbot",           // X / Twitter
	"viberbot",             // Viber
	"vkshare",              // VK
	"whatsapp",             // WhatsApp
}

// IsCrawler reports whether the request is from a known link-preview
// crawler. Also honors a `?_meta=1` query escape hatch — useful for
// manual testing and for Slack's re-crawl-after-30-seconds retry that
// sometimes strips the UA.
func IsCrawler(r *http.Request) bool {
	if r.URL.Query().Get("_meta") == "1" {
		return true
	}
	ua := strings.ToLower(r.Header.Get("User-Agent"))
	if ua == "" {
		return false
	}
	for _, needle := range crawlerUserAgents {
		if strings.Contains(ua, needle) {
			return true
		}
	}
	return false
}
