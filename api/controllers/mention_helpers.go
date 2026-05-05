package controllers

import (
	"context"
	"regexp"
	"strings"

	"github.com/jmoiron/sqlx"
)

// mentionTokenRE matches anything shaped like `@handle` with a
// letter/digit/underscore body. Used to extract candidate usernames
// from a comment body before we validate them against the users table.
var mentionTokenRE = regexp.MustCompile(`(?i)(?:^|[^A-Za-z0-9_])@([A-Za-z0-9_]+)`)

// resolveMentionedUserIDs returns the set of user ids whose username
// is @-mentioned in body, excluding excludeUID (the commenter — they
// don't self-notify). Usernames are matched case-insensitively to
// mirror the read-path regex in loadMentionActivity. Silent on error:
// the SSE publish is best-effort and must not block the write path.
func resolveMentionedUserIDs(ctx context.Context, db *sqlx.DB, body string, excludeUID uint) []uint {
	if body == "" {
		return nil
	}
	matches := mentionTokenRE.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		lc := strings.ToLower(m[1])
		if _, ok := seen[lc]; ok {
			continue
		}
		seen[lc] = struct{}{}
		names = append(names, lc)
	}
	if len(names) == 0 {
		return nil
	}

	query, args, err := sqlx.In(
		"SELECT id FROM users WHERE LOWER(username) IN (?) AND id <> ?",
		names, excludeUID,
	)
	if err != nil {
		return nil
	}
	var ids []uint
	if err := sqlx.SelectContext(ctx, db, &ids, db.Rebind(query), args...); err != nil {
		return nil
	}
	return ids
}
