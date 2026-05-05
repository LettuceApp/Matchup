// Compact "time since" formatter used across the app (HomeCard post
// header, MatchupPage overline, ActivityFeed timestamps). Intentionally
// minimal — no month/year fallbacks yet because every surface that
// renders timestamps prefers to stay short.
//
// Returns '' for missing input so callers can render conditionally
// (`{timeAgo && <span>{timeAgo}</span>}`).
export function relativeTime(dateStr) {
  if (!dateStr) return '';
  const diff = Date.now() - new Date(dateStr).getTime();
  const mins = Math.floor(diff / 60000);
  if (mins < 60) return `${mins || 1}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24) return `${hrs}h ago`;
  const days = Math.floor(hrs / 24);
  return `${days}d ago`;
}
