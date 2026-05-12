package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"Matchup/cache"
	activityv1 "Matchup/gen/activity/v1"
	"Matchup/gen/activity/v1/activityv1connect"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// ActivityHandler implements activityv1connect.ActivityServiceHandler.
//
// The Activity feed is computed per request from the existing write
// tables — there is no `notifications` table. Every kind below maps
// to a single indexed SQL query; the handler fans them out in
// parallel goroutines via sync.WaitGroup, merges the streams by
// occurred_at DESC, and returns the top N.
type ActivityHandler struct {
	DB     *sqlx.DB
	ReadDB *sqlx.DB
}

var _ activityv1connect.ActivityServiceHandler = (*ActivityHandler)(nil)

// Feed-kind constants. Frontend branches on these strings; keep them
// stable across proto revisions or you'll break the client copy map.
const (
	kindVoteCast             = "vote_cast"
	kindVoteWin              = "vote_win"
	kindVoteLoss             = "vote_loss"
	kindBracketProgress      = "bracket_progress"
	kindMatchupVoteReceived  = "matchup_vote_received"
	kindLikeReceived         = "like_received"
	kindCommentReceived      = "comment_received"
	kindNewFollower          = "new_follower"
	kindMentionReceived      = "mention_received"
	kindMatchupCompleted     = "matchup_completed"
	kindBracketCompleted     = "bracket_completed"
	kindMilestoneReached     = "milestone_reached"
	kindFollowedUserPosted   = "followed_user_posted"
)

const (
	defaultActivityLimit = 50
	perKindQueryLimit    = 50
)

// kindCategory maps each ActivityItem kind to the user-visible category
// that owns it. The mapping lives here (not in the settings handler)
// because the activity fan-out is the only read path that cares. See
// migration 017 for the default values.
var kindCategory = map[string]string{
	kindMentionReceived:     "mention",
	kindLikeReceived:        "engagement",
	kindCommentReceived:     "engagement",
	kindMatchupVoteReceived: "engagement",
	kindMilestoneReached:    "milestone",
	"matchup_closing_soon":  "prompt",
	"tie_needs_resolution":  "prompt",
	kindNewFollower:         "social",
	kindVoteCast:            "social",
	kindVoteWin:             "social",
	kindVoteLoss:            "social",
	kindBracketProgress:     "social",
	kindMatchupCompleted:    "social",
	kindBracketCompleted:    "social",
	kindFollowedUserPosted:  "social",
}

// GetUserActivity returns the merged activity feed for the given user.
// Accepts either a username or a public UUID in `req.Msg.UserId`
// (mirrors GetUserLikes). Pagination cursor is RFC3339 — pass the
// `occurred_at` of the last item you saw to get the next page.
func (h *ActivityHandler) GetUserActivity(ctx context.Context, req *connect.Request[activityv1.GetUserActivityRequest]) (*connect.Response[activityv1.GetUserActivityResponse], error) {
	db := dbForRead(ctx, h.DB, h.ReadDB)

	user, err := resolveUserByIdentifier(db, req.Msg.GetUserId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, errors.New("user not found"))
	}

	// Activity is owner-only. A non-owner caller (signed in OR
	// anonymous) sees PermissionDenied — same response shape as the
	// frontend's owner-only tab gate, so neither leaks "this user
	// exists, you just can't see them." httpctx.CurrentUserID
	// returns the canonical internal uint set by the auth middleware;
	// resolveUserByIdentifier populated `user.ID` from the request's
	// username-or-public-id, so a direct uint compare is the
	// strictest match.
	viewerID, ok := httpctx.CurrentUserID(ctx)
	if !ok || viewerID == 0 {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("activity is private"))
	}
	if viewerID != user.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, errors.New("activity is private"))
	}

	// Cursor: RFC3339 timestamp. Items with occurred_at strictly older
	// than this are returned. Zero-value means "from the top".
	var before time.Time
	if raw := req.Msg.GetBefore(); raw != "" {
		t, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid before cursor: %w", perr))
		}
		before = t
	}

	limit := int(req.Msg.GetLimit())
	if limit <= 0 {
		limit = defaultActivityLimit
	}
	if limit > 200 {
		limit = 200
	}

	// Fan out — each helper produces zero or more ActivityItem rows.
	// Errors from one helper do NOT fail the whole request; a degraded
	// feed is better than a blank one. We log the failure inline via
	// the mutex-guarded errs slice for later aggregation.
	var (
		mu    sync.Mutex
		items []*activityv1.ActivityItem
		errs  []string
		wg    sync.WaitGroup
	)

	collect := func(name string, fn func() ([]*activityv1.ActivityItem, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := fn()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				errs = append(errs, fmt.Sprintf("%s: %v", name, err))
				return
			}
			items = append(items, got...)
		}()
	}

	// `user.Username` is the matchup/bracket author for every "on-you"
	// event (those rows reference this user's own content), so we
	// pass it straight through to the receive-path helpers instead of
	// re-JOINing `users` just to get our own row back.
	viewerUsername := user.Username

	collect("vote_cast", func() ([]*activityv1.ActivityItem, error) {
		return loadVoteCastActivity(ctx, db, user.ID, before)
	})
	collect("vote_outcome", func() ([]*activityv1.ActivityItem, error) {
		return loadVoteOutcomeActivity(ctx, db, user.ID, before)
	})
	collect("bracket_progress", func() ([]*activityv1.ActivityItem, error) {
		return loadBracketProgressActivity(ctx, db, user.ID, before)
	})
	collect("matchup_vote_received", func() ([]*activityv1.ActivityItem, error) {
		return loadMatchupVotesReceivedActivity(ctx, db, user.ID, viewerUsername, before)
	})
	collect("like_received", func() ([]*activityv1.ActivityItem, error) {
		return loadLikesReceivedActivity(ctx, db, user.ID, viewerUsername, before)
	})
	collect("comment_received", func() ([]*activityv1.ActivityItem, error) {
		return loadCommentsReceivedActivity(ctx, db, user.ID, viewerUsername, before)
	})
	collect("new_follower", func() ([]*activityv1.ActivityItem, error) {
		return loadNewFollowerActivity(ctx, db, user.ID, before)
	})
	collect("mention_received", func() ([]*activityv1.ActivityItem, error) {
		return loadMentionActivity(ctx, db, user.ID, viewerUsername, before)
	})
	collect("authored_completed", func() ([]*activityv1.ActivityItem, error) {
		return loadAuthoredContentCompletedActivity(ctx, db, user.ID, viewerUsername, before)
	})
	collect("notifications", func() ([]*activityv1.ActivityItem, error) {
		return loadNotificationsActivity(ctx, db, user.ID, viewerUsername, before)
	})
	collect("followed_user_posted", func() ([]*activityv1.ActivityItem, error) {
		return loadFollowedUserPostedActivity(ctx, db, user.ID, before)
	})

	wg.Wait()

	// Filter out items whose category the user has opted out of. Done
	// post-fan-out so every helper stays kind-agnostic — the category
	// map in this file is the single source of truth for
	// kind→category mapping.
	prefs := decodeNotificationPrefs(user.NotificationPrefs)
	items = filterItemsByPrefs(items, prefs)

	// Hide activity involving blocked or muted users. Same post-fan-out
	// strategy — the 14 kind-specific SQL queries don't need to grow
	// their own NOT IN clauses; one actor_username check after the
	// merge covers every kind uniformly. The cost is a fetch of the
	// viewer's blocked+muted set per request; for the vast majority of
	// viewers (zero blocks, zero mutes) that's one indexed lookup.
	hidden, _ := hiddenUsernamesForViewer(ctx, h.DB, user.ID)
	if len(hidden) > 0 {
		items = filterItemsByHiddenUsernames(items, hidden)
	}

	// Sort by occurred_at DESC, stable fallback on id so the order is
	// deterministic when two events share a timestamp.
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].OccurredAt == items[j].OccurredAt {
			return items[i].Id > items[j].Id
		}
		return items[i].OccurredAt > items[j].OccurredAt
	})

	// Cap at the requested limit + emit cursor for the next page.
	var nextBefore *string
	if len(items) > limit {
		items = items[:limit]
		last := items[len(items)-1].OccurredAt
		nextBefore = &last
	}

	resp := &activityv1.GetUserActivityResponse{
		Items:      items,
		NextBefore: nextBefore,
	}
	return connect.NewResponse(resp), nil
}

// MarkActivityRead stamps `notifications.read_at = NOW()` on every
// unread row owned by the viewer with `occurred_at <= before`. Empty
// cursor means "everything through now." Only affects persisted
// notifications (milestone_reached, closing_soon, tie_needs_resolution);
// derived kinds have nothing to stamp.
//
// Auth: requires a logged-in user; other users' rows are untouched
// because of the `user_id = $1` predicate.
func (h *ActivityHandler) MarkActivityRead(ctx context.Context, req *connect.Request[activityv1.MarkActivityReadRequest]) (*connect.Response[activityv1.MarkActivityReadResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	// Default cursor = NOW(). If the client passes an RFC3339 string
	// we honour it — useful for "mark everything through this
	// pagination cursor" semantics.
	before := time.Now().UTC()
	if raw := req.Msg.GetBefore(); raw != "" {
		t, perr := time.Parse(time.RFC3339, raw)
		if perr != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid before cursor: %w", perr))
		}
		before = t
	}

	result, err := h.DB.ExecContext(ctx,
		`UPDATE public.notifications
		   SET read_at = NOW()
		 WHERE user_id = $1
		   AND read_at IS NULL
		   AND occurred_at <= $2`,
		uid, before,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	n, _ := result.RowsAffected()

	resp := connect.NewResponse(&activityv1.MarkActivityReadResponse{
		MarkedCount: int32(n),
	})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// SubscribePush upserts a push subscription for the caller's browser
// or mobile device. `platform` discriminates the delivery channel; the
// validator enforces platform-specific field requirements so native
// rows can't accidentally store garbage p256dh/auth values.
//
// Re-subscribing from the same endpoint is an UPSERT — e.g. a user
// who re-installs the app and registers a new token, or a shared
// browser where a different user signs in. The `ON CONFLICT` clause
// rewrites user_id + keys + platform to the most recent subscribe.
func (h *ActivityHandler) SubscribePush(ctx context.Context, req *connect.Request[activityv1.SubscribePushRequest]) (*connect.Response[activityv1.SubscribePushResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	m := req.Msg

	// Default platform to "web" when the client omits it — lets the
	// pre-rename web bundle keep working during the rollout window.
	platform := m.GetPlatform()
	if platform == "" {
		platform = "web"
	}

	if m.GetEndpoint() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint is required"))
	}

	// Per-platform field requirements. Web needs the encryption
	// triple; native platforms use only the device token (endpoint)
	// and must NOT send p256dh/auth (stops clients from shipping
	// nonsense that'd dead-letter in the dispatcher).
	var p256dh, authKey *string
	switch platform {
	case "web":
		if m.GetP256DhKey() == "" || m.GetAuthKey() == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("p256dh_key and auth_key are required for web"))
		}
		p256 := m.GetP256DhKey()
		auth := m.GetAuthKey()
		p256dh = &p256
		authKey = &auth
	case "ios", "android":
		if m.GetP256DhKey() != "" || m.GetAuthKey() != "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("p256dh_key/auth_key must be empty for native platforms"))
		}
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("unsupported platform"))
	}

	var ua *string
	if raw := m.GetUserAgent(); raw != "" {
		ua = &raw
	}

	// Upsert keyed on the unique `endpoint` index. If the endpoint
	// already exists we update the user_id too — covers shared-device
	// / different-account cases — and refresh the platform in case
	// the row was somehow created with the wrong one.
	var id int64
	if err := h.DB.QueryRowContext(ctx, `
		INSERT INTO public.push_subscriptions
			(user_id, platform, endpoint, p256dh_key, auth_key, user_agent)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (endpoint) DO UPDATE SET
			user_id = EXCLUDED.user_id,
			platform = EXCLUDED.platform,
			p256dh_key = EXCLUDED.p256dh_key,
			auth_key = EXCLUDED.auth_key,
			user_agent = EXCLUDED.user_agent
		RETURNING id`,
		uid, platform, m.GetEndpoint(), p256dh, authKey, ua,
	).Scan(&id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&activityv1.SubscribePushResponse{Id: id})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// UnsubscribePush deletes the caller's subscription row for the given
// endpoint. Scoped to the caller's own rows (a user can't unregister
// someone else's device by guessing endpoints).
func (h *ActivityHandler) UnsubscribePush(ctx context.Context, req *connect.Request[activityv1.UnsubscribePushRequest]) (*connect.Response[activityv1.UnsubscribePushResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	endpoint := req.Msg.GetEndpoint()
	if endpoint == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("endpoint required"))
	}

	result, err := h.DB.ExecContext(ctx,
		`DELETE FROM public.push_subscriptions WHERE user_id = $1 AND endpoint = $2`,
		uid, endpoint,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	n, _ := result.RowsAffected()

	resp := connect.NewResponse(&activityv1.UnsubscribePushResponse{Removed: n > 0})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// GetPushConfig returns platform-specific config the client needs to
// subscribe. Today that's the VAPID public key for Web Push; native
// clients are pre-configured with APNS/FCM credentials at build time
// and ignore the vapid_public_key field. Auth-required so the
// settings UI only surfaces for signed-in users.
func (h *ActivityHandler) GetPushConfig(ctx context.Context, _ *connect.Request[activityv1.GetPushConfigRequest]) (*connect.Response[activityv1.GetPushConfigResponse], error) {
	if _, ok := httpctx.CurrentUserID(ctx); !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}
	return connect.NewResponse(&activityv1.GetPushConfigResponse{
		VapidPublicKey: cache.VAPIDPublicKey(),
	}), nil
}

// filterItemsByPrefs drops items whose category is turned off in prefs.
// Items whose kind is not in the category map pass through untouched —
// a safer default when a future kind is added server-side before the
// kindCategory map is updated. A forgotten map entry surfaces items
// the user might not want, but that's better than silent suppression.
func filterItemsByPrefs(items []*activityv1.ActivityItem, prefs map[string]bool) []*activityv1.ActivityItem {
	if len(items) == 0 {
		return items
	}
	out := items[:0]
	for _, it := range items {
		cat, ok := kindCategory[it.Kind]
		if !ok {
			out = append(out, it)
			continue
		}
		if enabled, ok := prefs[cat]; !ok || enabled {
			out = append(out, it)
		}
	}
	return out
}

// filterItemsByHiddenUsernames drops items whose actor_username is in
// the hidden (blocked + muted) set. Case-insensitive match against the
// lowercase usernames returned by hiddenUsernamesForViewer.
//
// Items without an actor (vote_cast, matchup_completed — events about
// the viewer's own content) pass through untouched. Those don't
// involve another user so there's no-one to hide.
func filterItemsByHiddenUsernames(items []*activityv1.ActivityItem, hidden map[string]bool) []*activityv1.ActivityItem {
	if len(items) == 0 || len(hidden) == 0 {
		return items
	}
	out := items[:0]
	for _, it := range items {
		if it.ActorUsername != nil && hidden[strings.ToLower(*it.ActorUsername)] {
			continue
		}
		out = append(out, it)
	}
	return out
}

// ----- per-kind SQL helpers ---------------------------------------

// beforeClause returns the WHERE-fragment + arg for the cursor, or
// empty string + nil if before is the zero time. Keeps each helper
// from having to conditionally-concat SQL.
func beforeClause(before time.Time, col string) (string, interface{}) {
	if before.IsZero() {
		return "", nil
	}
	return fmt.Sprintf(" AND %s < $2", col), before.UTC()
}

// loadVoteCastActivity — every matchup this user has voted on.
func loadVoteCastActivity(ctx context.Context, db *sqlx.DB, userID uint, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		ID             int64     `db:"id"`
		CreatedAt      time.Time `db:"created_at"`
		MatchupID      string    `db:"matchup_public_id"`
		Title          string    `db:"title"`
		Item           string    `db:"item"`
		ItemPublic     string    `db:"item_public_id"`
		AuthorUsername string    `db:"author_username"`
	}
	cursor, arg := beforeClause(before, "mv.created_at")
	// Joins the matchup's author onto the row so the frontend can build
	// the canonical SPA link (`/users/{username}/matchup/{id}`). Without
	// this the activity Link fell through to the SPA catch-all.
	query := `SELECT mv.id,
		         mv.created_at,
		         m.public_id AS matchup_public_id,
		         m.title,
		         mi.item,
		         mi.public_id AS item_public_id,
		         u.username AS author_username
		  FROM matchup_votes mv
		  JOIN matchups m        ON m.public_id  = mv.matchup_public_id
		  JOIN matchup_items mi  ON mi.public_id = mv.matchup_item_public_id
		  JOIN users u           ON u.id = m.author_id
		  WHERE mv.user_id = $1` + cursor + `
		  ORDER BY mv.created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	var err error
	if arg != nil {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID, arg)
	} else {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, &activityv1.ActivityItem{
			Id:           fmt.Sprintf("%s:%d", kindVoteCast, r.ID),
			Kind:         kindVoteCast,
			OccurredAt:   rfc3339(r.CreatedAt),
			SubjectType:  "matchup",
			SubjectId:    r.MatchupID,
			SubjectTitle: r.Title,
			Payload: map[string]string{
				"voted_item":      r.Item,
				"voted_item_id":   r.ItemPublic,
				"author_username": r.AuthorUsername,
			},
		})
	}
	return out, nil
}

// loadVoteOutcomeActivity — completed matchups the user voted in,
// split into win / loss based on whether their picked item matches
// matchups.winner_item_id.
func loadVoteOutcomeActivity(ctx context.Context, db *sqlx.DB, userID uint, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		MatchupID      uint      `db:"matchup_id"`
		MatchupPID     string    `db:"matchup_public_id"`
		Title          string    `db:"title"`
		UpdatedAt      time.Time `db:"updated_at"`
		VotedItem      string    `db:"voted_item"`
		VotedItemPID   string    `db:"voted_item_public_id"`
		WinnerItem     string    `db:"winner_item"`
		WinnerItemID   uint      `db:"winner_item_id"`
		VotedItemID    uint      `db:"voted_item_id"`
		AuthorUsername string    `db:"author_username"`
	}
	cursor, arg := beforeClause(before, "m.updated_at")
	query := `SELECT m.id              AS matchup_id,
		         m.public_id       AS matchup_public_id,
		         m.title,
		         m.updated_at,
		         voted.item        AS voted_item,
		         voted.public_id   AS voted_item_public_id,
		         voted.id          AS voted_item_id,
		         winner.item       AS winner_item,
		         m.winner_item_id  AS winner_item_id,
		         u.username        AS author_username
		  FROM matchup_votes mv
		  JOIN matchups m            ON m.public_id  = mv.matchup_public_id
		  JOIN matchup_items voted   ON voted.public_id = mv.matchup_item_public_id
		  JOIN matchup_items winner  ON winner.id    = m.winner_item_id
		  JOIN users u               ON u.id = m.author_id
		  WHERE mv.user_id = $1
		    AND m.status = 'completed'
		    AND m.winner_item_id IS NOT NULL` + cursor + `
		  ORDER BY m.updated_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	var err error
	if arg != nil {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID, arg)
	} else {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		kind := kindVoteLoss
		if r.VotedItemID == r.WinnerItemID {
			kind = kindVoteWin
		}
		out = append(out, &activityv1.ActivityItem{
			Id:           fmt.Sprintf("%s:%d", kind, r.MatchupID),
			Kind:         kind,
			OccurredAt:   rfc3339(r.UpdatedAt),
			SubjectType:  "matchup",
			SubjectId:    r.MatchupPID,
			SubjectTitle: r.Title,
			Payload: map[string]string{
				"voted_item":      r.VotedItem,
				"winner_item":     r.WinnerItem,
				"author_username": r.AuthorUsername,
			},
		})
	}
	return out, nil
}

// loadBracketProgressActivity — brackets the user voted in, surfacing
// the current round + status. Rolls `advance` and `complete` into one
// kind; the frontend reads payload.status to differentiate copy.
func loadBracketProgressActivity(ctx context.Context, db *sqlx.DB, userID uint, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		BracketID    uint      `db:"bracket_id"`
		BracketPID   string    `db:"bracket_public_id"`
		Title        string    `db:"title"`
		UpdatedAt    time.Time `db:"updated_at"`
		CurrentRound int       `db:"current_round"`
		Status       string    `db:"status"`
	}
	cursor, arg := beforeClause(before, "b.updated_at")
	query := `SELECT b.id             AS bracket_id,
		         b.public_id      AS bracket_public_id,
		         b.title,
		         b.updated_at,
		         b.current_round,
		         b.status
		  FROM brackets b
		  WHERE b.id IN (
		    SELECT DISTINCT m.bracket_id FROM matchup_votes mv
		    JOIN matchups m ON m.public_id = mv.matchup_public_id
		    WHERE mv.user_id = $1 AND m.bracket_id IS NOT NULL
		  )` + cursor + `
		  ORDER BY b.updated_at DESC LIMIT 20`

	var rows []row
	var err error
	if arg != nil {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID, arg)
	} else {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, &activityv1.ActivityItem{
			Id:           fmt.Sprintf("%s:%d", kindBracketProgress, r.BracketID),
			Kind:         kindBracketProgress,
			OccurredAt:   rfc3339(r.UpdatedAt),
			SubjectType:  "bracket",
			SubjectId:    r.BracketPID,
			SubjectTitle: r.Title,
			Payload: map[string]string{
				"round":  strconv.Itoa(r.CurrentRound),
				"status": r.Status,
			},
		})
	}
	return out, nil
}

// loadMatchupVotesReceivedActivity — votes landed on matchups authored
// by this user. Owner self-votes AND anonymous votes are both kept
// because they're all real engagement; only the author's own user_id
// is excluded. `viewerUsername` is the matchup author (== viewer) and
// is echoed into the payload so the frontend can build the deep link.
func loadMatchupVotesReceivedActivity(ctx context.Context, db *sqlx.DB, userID uint, viewerUsername string, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		VoteID     int64     `db:"vote_id"`
		CreatedAt  time.Time `db:"created_at"`
		MatchupPID string    `db:"matchup_public_id"`
		Title      string    `db:"title"`
		Item       string    `db:"item"`
	}
	cursor, arg := beforeClause(before, "mv.created_at")
	query := `SELECT mv.id            AS vote_id,
		         mv.created_at,
		         m.public_id      AS matchup_public_id,
		         m.title,
		         mi.item
		  FROM matchup_votes mv
		  JOIN matchups m       ON m.public_id  = mv.matchup_public_id
		  JOIN matchup_items mi ON mi.public_id = mv.matchup_item_public_id
		  WHERE m.author_id = $1
		    AND (mv.user_id IS NULL OR mv.user_id <> $1)` + cursor + `
		  ORDER BY mv.created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	var err error
	if arg != nil {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID, arg)
	} else {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		out = append(out, &activityv1.ActivityItem{
			Id:           fmt.Sprintf("%s:%d", kindMatchupVoteReceived, r.VoteID),
			Kind:         kindMatchupVoteReceived,
			OccurredAt:   rfc3339(r.CreatedAt),
			SubjectType:  "matchup",
			SubjectId:    r.MatchupPID,
			SubjectTitle: r.Title,
			Payload: map[string]string{
				"item":            r.Item,
				"author_username": viewerUsername,
			},
		})
	}
	return out, nil
}

// loadLikesReceivedActivity — likes on matchups OR brackets this user
// authored. UNION ALL across the two like tables keeps the response
// unified; the frontend reads subject_type to route the link.
// `viewerUsername` threads the matchup/bracket author (always == viewer
// for received events) into the payload so the frontend can build the
// canonical SPA link for matchup subjects.
func loadLikesReceivedActivity(ctx context.Context, db *sqlx.DB, userID uint, viewerUsername string, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		LikeID      int64     `db:"like_id"`
		CreatedAt   time.Time `db:"created_at"`
		SubjectType string    `db:"subject_type"`
		SubjectPID  string    `db:"subject_pid"`
		Title       string    `db:"title"`
		ActorID     string    `db:"actor_id"`
		ActorName   string    `db:"actor_username"`
	}
	// Cursor goes inside each UNION branch so the post-UNION LIMIT is
	// stable; Postgres disallows ORDER BY on the legs themselves when
	// the outer ORDER BY references a positional column.
	cursorLikes, cursorBracketLikes := "", ""
	args := []interface{}{userID}
	if !before.IsZero() {
		cursorLikes = " AND l.created_at < $2"
		cursorBracketLikes = " AND bl.created_at < $2"
		args = append(args, before.UTC())
	}
	query := `SELECT * FROM ((
		  SELECT l.id           AS like_id,
		         l.created_at,
		         'matchup'      AS subject_type,
		         m.public_id    AS subject_pid,
		         m.title        AS title,
		         u.public_id    AS actor_id,
		         u.username     AS actor_username
		  FROM likes l
		  JOIN matchups m ON m.id = l.matchup_id
		  JOIN users u    ON u.id = l.user_id
		  WHERE m.author_id = $1 AND l.user_id <> $1` + cursorLikes + `
		) UNION ALL (
		  SELECT bl.id          AS like_id,
		         bl.created_at,
		         'bracket'      AS subject_type,
		         b.public_id    AS subject_pid,
		         b.title        AS title,
		         u.public_id    AS actor_id,
		         u.username     AS actor_username
		  FROM bracket_likes bl
		  JOIN brackets b ON b.id = bl.bracket_id
		  JOIN users u    ON u.id = bl.user_id
		  WHERE b.author_id = $1 AND bl.user_id <> $1` + cursorBracketLikes + `
		)) t ORDER BY created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		actor := r.ActorID
		name := r.ActorName
		out = append(out, &activityv1.ActivityItem{
			Id:             fmt.Sprintf("%s:%s:%d", kindLikeReceived, r.SubjectType, r.LikeID),
			Kind:           kindLikeReceived,
			OccurredAt:     rfc3339(r.CreatedAt),
			ActorId:        &actor,
			ActorUsername:  &name,
			SubjectType:    r.SubjectType,
			SubjectId:      r.SubjectPID,
			SubjectTitle:   r.Title,
			Payload: map[string]string{
				"author_username": viewerUsername,
			},
		})
	}
	return out, nil
}

// loadCommentsReceivedActivity — comments on matchups + bracket_comments
// on brackets this user authored. See loadLikesReceivedActivity for
// the `viewerUsername` rationale.
func loadCommentsReceivedActivity(ctx context.Context, db *sqlx.DB, userID uint, viewerUsername string, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		CommentID   int64     `db:"comment_id"`
		CreatedAt   time.Time `db:"created_at"`
		SubjectType string    `db:"subject_type"`
		SubjectPID  string    `db:"subject_pid"`
		Title       string    `db:"title"`
		ActorID     string    `db:"actor_id"`
		ActorName   string    `db:"actor_username"`
		Body        string    `db:"body"`
	}
	cursorC, cursorBC := "", ""
	args := []interface{}{userID}
	if !before.IsZero() {
		cursorC = " AND c.created_at < $2"
		cursorBC = " AND bc.created_at < $2"
		args = append(args, before.UTC())
	}
	query := `SELECT * FROM ((
		  SELECT c.id           AS comment_id,
		         c.created_at,
		         'matchup'      AS subject_type,
		         m.public_id    AS subject_pid,
		         m.title        AS title,
		         u.public_id    AS actor_id,
		         u.username     AS actor_username,
		         c.body
		  FROM comments c
		  JOIN matchups m ON m.id = c.matchup_id
		  JOIN users u    ON u.id = c.user_id
		  WHERE m.author_id = $1 AND c.user_id <> $1` + cursorC + `
		) UNION ALL (
		  SELECT bc.id          AS comment_id,
		         bc.created_at,
		         'bracket'      AS subject_type,
		         b.public_id    AS subject_pid,
		         b.title        AS title,
		         u.public_id    AS actor_id,
		         u.username     AS actor_username,
		         bc.body
		  FROM bracket_comments bc
		  JOIN brackets b ON b.id = bc.bracket_id
		  JOIN users u    ON u.id = bc.user_id
		  WHERE b.author_id = $1 AND bc.user_id <> $1` + cursorBC + `
		)) t ORDER BY created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		actor := r.ActorID
		name := r.ActorName
		snippet := r.Body
		if len(snippet) > 120 {
			snippet = snippet[:120] + "…"
		}
		out = append(out, &activityv1.ActivityItem{
			Id:            fmt.Sprintf("%s:%s:%d", kindCommentReceived, r.SubjectType, r.CommentID),
			Kind:          kindCommentReceived,
			OccurredAt:    rfc3339(r.CreatedAt),
			ActorId:       &actor,
			ActorUsername: &name,
			SubjectType:   r.SubjectType,
			SubjectId:     r.SubjectPID,
			SubjectTitle:  r.Title,
			Payload: map[string]string{
				"body":            snippet,
				"author_username": viewerUsername,
			},
		})
	}
	return out, nil
}

// loadNewFollowerActivity — someone followed you.
func loadNewFollowerActivity(ctx context.Context, db *sqlx.DB, userID uint, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		FollowID   int64     `db:"follow_id"`
		CreatedAt  time.Time `db:"created_at"`
		ActorID    string    `db:"actor_id"`
		ActorName  string    `db:"actor_username"`
	}
	cursor, arg := beforeClause(before, "f.created_at")
	query := `SELECT f.id         AS follow_id,
		         f.created_at,
		         u.public_id  AS actor_id,
		         u.username   AS actor_username
		  FROM follows f
		  JOIN users u ON u.id = f.follower_id
		  WHERE f.followed_id = $1` + cursor + `
		  ORDER BY f.created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	var err error
	if arg != nil {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID, arg)
	} else {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID)
	}
	if err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		actor := r.ActorID
		name := r.ActorName
		out = append(out, &activityv1.ActivityItem{
			Id:            fmt.Sprintf("%s:%d", kindNewFollower, r.FollowID),
			Kind:          kindNewFollower,
			OccurredAt:    rfc3339(r.CreatedAt),
			ActorId:       &actor,
			ActorUsername: &name,
			SubjectType:   "user",
			SubjectId:     r.ActorID,
			SubjectTitle:  r.ActorName,
		})
	}
	return out, nil
}

// loadMentionActivity — comments on matchups OR bracket_comments whose
// body contains `@viewerUsername`. We narrow candidates in SQL with an
// ILIKE (trivial index, no substring-index needed for this volume) and
// post-check in Go with a word-boundary regex so `@alice` doesn't match
// `@alice123`. Self-mentions are excluded (user_id <> $1).
//
// viewerUsername doubles as the author of the matchup/bracket for link
// payload purposes — mentions surface regardless of who authored the
// parent subject, so we source the author from the SQL join rather than
// threading it through. The payload carries `author_username` of the
// subject (for matchup link construction) and `actor_username` of the
// commenter (already on the top-level ActivityItem).
func loadMentionActivity(ctx context.Context, db *sqlx.DB, userID uint, viewerUsername string, before time.Time) ([]*activityv1.ActivityItem, error) {
	// Viewer's own username is required — no username, no mentions.
	if viewerUsername == "" {
		return nil, nil
	}

	type row struct {
		CommentID      int64     `db:"comment_id"`
		CreatedAt      time.Time `db:"created_at"`
		SubjectType    string    `db:"subject_type"`
		SubjectPID     string    `db:"subject_pid"`
		Title          string    `db:"title"`
		ActorID        string    `db:"actor_id"`
		ActorName      string    `db:"actor_username"`
		AuthorUsername string    `db:"author_username"`
		Body           string    `db:"body"`
	}

	// Cursor goes inside each UNION branch so the post-UNION LIMIT is
	// stable; matches the idiom in loadLikesReceived/loadCommentsReceived.
	cursorC, cursorBC := "", ""
	args := []interface{}{userID, viewerUsername}
	if !before.IsZero() {
		cursorC = " AND c.created_at < $3"
		cursorBC = " AND bc.created_at < $3"
		args = append(args, before.UTC())
	}

	// ILIKE narrows the set; the Go-side regex below confirms a real
	// word-boundary `@username`. We JOIN users on the actor so we only
	// keep comments authored by a real signed-in user (no anon comments
	// at the moment, but this future-proofs if that changes) and so we
	// can populate actor_id/actor_username on the output item.
	query := `SELECT * FROM ((
		  SELECT c.id            AS comment_id,
		         c.created_at,
		         'matchup'       AS subject_type,
		         m.public_id     AS subject_pid,
		         m.title         AS title,
		         u.public_id     AS actor_id,
		         u.username      AS actor_username,
		         ma.username     AS author_username,
		         c.body
		  FROM comments c
		  JOIN matchups m ON m.id = c.matchup_id
		  JOIN users u    ON u.id = c.user_id
		  JOIN users ma   ON ma.id = m.author_id
		  WHERE c.user_id <> $1
		    AND c.body ILIKE '%@' || $2 || '%'` + cursorC + `
		) UNION ALL (
		  SELECT bc.id           AS comment_id,
		         bc.created_at,
		         'bracket'       AS subject_type,
		         b.public_id     AS subject_pid,
		         b.title         AS title,
		         u.public_id     AS actor_id,
		         u.username      AS actor_username,
		         ba.username     AS author_username,
		         bc.body
		  FROM bracket_comments bc
		  JOIN brackets b ON b.id = bc.bracket_id
		  JOIN users u    ON u.id = bc.user_id
		  JOIN users ba   ON ba.id = b.author_id
		  WHERE bc.user_id <> $1
		    AND bc.body ILIKE '%@' || $2 || '%'` + cursorBC + `
		)) t ORDER BY created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, err
	}

	// Word-boundary check: `@alice` matches; `@alice123` does not. The
	// trailing \b is crucial — without it `@alice1` would pass.
	mentionRE, err := regexp.Compile(`(?i)(^|[^A-Za-z0-9_])@` + regexp.QuoteMeta(viewerUsername) + `($|[^A-Za-z0-9_])`)
	if err != nil {
		return nil, fmt.Errorf("compile mention regex: %w", err)
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		if !mentionRE.MatchString(r.Body) {
			continue
		}
		actor := r.ActorID
		name := r.ActorName
		snippet := r.Body
		if len(snippet) > 120 {
			snippet = snippet[:120] + "…"
		}
		out = append(out, &activityv1.ActivityItem{
			Id:            fmt.Sprintf("%s:%s:%d", kindMentionReceived, r.SubjectType, r.CommentID),
			Kind:          kindMentionReceived,
			OccurredAt:    rfc3339(r.CreatedAt),
			ActorId:       &actor,
			ActorUsername: &name,
			SubjectType:   r.SubjectType,
			SubjectId:     r.SubjectPID,
			SubjectTitle:  r.Title,
			Payload: map[string]string{
				"body":            snippet,
				"author_username": r.AuthorUsername,
			},
		})
	}
	return out, nil
}

// loadAuthoredContentCompletedActivity — matchups + brackets the viewer
// authored that have reached status='completed'. Surfaces as
// matchup_completed / bracket_completed. Overlaps conceptually with the
// per-vote vote_win/vote_loss feed (if the author voted), but this is
// about the content CLOSING from the creator's POV — different verb,
// different icon. The two can and should coexist on a busy day.
//
// viewerUsername flows into payload.author_username so the matchup
// branch builds the canonical SPA link (/users/{author}/matchup/{id}).
// For brackets we don't need it (SPA route is /brackets/{id}) but pass
// it along for symmetry in case the route evolves.
func loadAuthoredContentCompletedActivity(ctx context.Context, db *sqlx.DB, userID uint, viewerUsername string, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		InternalID  uint      `db:"internal_id"`
		PublicID    string    `db:"public_id"`
		Title       string    `db:"title"`
		UpdatedAt   time.Time `db:"updated_at"`
		Kind        string    `db:"kind"`
		WinnerItem  *string   `db:"winner_item"` // nullable — only matchups populate it
	}

	// Cursor goes inside each UNION branch; same idiom as the other
	// receive-path helpers.
	cursorM, cursorB := "", ""
	args := []interface{}{userID}
	if !before.IsZero() {
		cursorM = " AND m.updated_at < $2"
		cursorB = " AND b.updated_at < $2"
		args = append(args, before.UTC())
	}

	// LEFT JOIN on winner so un-resolved matchups (no winner declared
	// for some reason) still appear — handles the edge case gracefully.
	query := `SELECT * FROM ((
		  SELECT m.id              AS internal_id,
		         m.public_id       AS public_id,
		         m.title           AS title,
		         m.updated_at      AS updated_at,
		         'matchup_completed' AS kind,
		         winner.item       AS winner_item
		  FROM matchups m
		  LEFT JOIN matchup_items winner ON winner.id = m.winner_item_id
		  WHERE m.author_id = $1 AND m.status = 'completed'` + cursorM + `
		) UNION ALL (
		  SELECT b.id              AS internal_id,
		         b.public_id       AS public_id,
		         b.title           AS title,
		         b.updated_at      AS updated_at,
		         'bracket_completed' AS kind,
		         NULL              AS winner_item
		  FROM brackets b
		  WHERE b.author_id = $1 AND b.status = 'completed'` + cursorB + `
		)) t ORDER BY updated_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		payload := map[string]string{
			"author_username": viewerUsername,
		}
		subjectType := "matchup"
		if r.Kind == kindBracketCompleted {
			subjectType = "bracket"
		}
		if r.WinnerItem != nil && *r.WinnerItem != "" {
			payload["winner_item"] = *r.WinnerItem
		}
		out = append(out, &activityv1.ActivityItem{
			Id:           fmt.Sprintf("%s:%d", r.Kind, r.InternalID),
			Kind:         r.Kind,
			OccurredAt:   rfc3339(r.UpdatedAt),
			SubjectType:  subjectType,
			SubjectId:    r.PublicID,
			SubjectTitle: r.Title,
			Payload:      payload,
		})
	}
	return out, nil
}

// loadNotificationsActivity — reads persisted notification rows (the
// home for events that don't have a natural source row). Currently the
// scheduler emits `milestone_reached`; Step 4 of the plan adds
// `matchup_closing_soon` and `tie_needs_resolution` through the same
// table. We handle each kind uniformly by reading payload from jsonb.
//
// subject_id in the table is the INTERNAL integer id; we resolve it to
// the subject's public_id here so the frontend stays UUID-only.
//
// viewerUsername is echoed into payload.author_username for matchup /
// bracket kinds so the frontend can build `/users/{author}/matchup/{id}`.
// For user-subject kinds the viewer IS the subject, so the same value
// works.
func loadNotificationsActivity(ctx context.Context, db *sqlx.DB, userID uint, viewerUsername string, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		ID           int64      `db:"id"`
		Kind         string     `db:"kind"`
		SubjectType  string     `db:"subject_type"`
		SubjectID    uint       `db:"subject_id"`
		ThresholdInt *int       `db:"threshold_int"`
		Payload      []byte     `db:"payload"`
		OccurredAt   time.Time  `db:"occurred_at"`
		ReadAt       *time.Time `db:"read_at"`
	}

	cursor, arg := beforeClause(before, "n.occurred_at")
	query := `SELECT n.id,
		         n.kind,
		         n.subject_type,
		         n.subject_id,
		         n.threshold_int,
		         n.payload,
		         n.occurred_at,
		         n.read_at
		  FROM public.notifications n
		  WHERE n.user_id = $1` + cursor + `
		  ORDER BY n.occurred_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	var err error
	if arg != nil {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID, arg)
	} else {
		err = sqlx.SelectContext(ctx, db, &rows, query, userID)
	}
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}

	// Bucket subject_ids by type so we can resolve all public_ids with
	// one SELECT per bucket instead of N queries.
	var matchupIDs, bracketIDs, userIDs []uint
	for _, r := range rows {
		switch r.SubjectType {
		case "matchup":
			matchupIDs = append(matchupIDs, r.SubjectID)
		case "bracket":
			bracketIDs = append(bracketIDs, r.SubjectID)
		case "user":
			userIDs = append(userIDs, r.SubjectID)
		}
	}
	matchupPIDs := loadMatchupPublicIDMap(db, matchupIDs)
	bracketPIDs := loadBracketPublicIDMap(db, bracketIDs)
	userPIDs := loadUserPublicIDMap(db, userIDs)

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		// Payload is stored as jsonb map<string,any>; we unmarshal into
		// map[string]interface{} then stringify so our proto payload
		// (map<string,string>) stays happy. Numbers become their JSON
		// representation — acceptable because milestone thresholds are
		// already stored as a string in the scheduler SQL.
		payload := map[string]string{
			"author_username": viewerUsername,
		}
		if len(r.Payload) > 0 {
			var raw map[string]interface{}
			if jsonErr := json.Unmarshal(r.Payload, &raw); jsonErr == nil {
				for k, v := range raw {
					switch vv := v.(type) {
					case string:
						payload[k] = vv
					case float64:
						payload[k] = strconv.FormatFloat(vv, 'f', -1, 64)
					case bool:
						if vv {
							payload[k] = "true"
						} else {
							payload[k] = "false"
						}
					}
				}
			}
		}
		if r.ThresholdInt != nil {
			// Authoritative from the column even if payload disagrees.
			payload["threshold"] = strconv.Itoa(*r.ThresholdInt)
		}

		var subjectPID, subjectTitle string
		switch r.SubjectType {
		case "matchup":
			subjectPID = matchupPIDs[r.SubjectID]
		case "bracket":
			subjectPID = bracketPIDs[r.SubjectID]
		case "user":
			subjectPID = userPIDs[r.SubjectID]
		}
		if subjectPID == "" {
			// Subject row has been deleted since the notification was
			// emitted. Skip it — the link would 404 anyway.
			continue
		}
		if t, ok := payload["title"]; ok {
			subjectTitle = t
		}

		item := &activityv1.ActivityItem{
			Id:           fmt.Sprintf("%s:%d", r.Kind, r.ID),
			Kind:         r.Kind,
			OccurredAt:   rfc3339(r.OccurredAt),
			SubjectType:  r.SubjectType,
			SubjectId:    subjectPID,
			SubjectTitle: subjectTitle,
			Payload:      payload,
		}
		if r.ReadAt != nil {
			s := rfc3339(*r.ReadAt)
			item.ReadAt = &s
		}
		out = append(out, item)
	}
	return out, nil
}

// loadFollowedUserPostedActivity — surfaces new standalone matchups and
// new brackets from users the viewer follows. Purely derived — no
// write-path hooks, no migration. Joins on the existing `follows`
// table so it auto-updates as follow state changes.
//
// Visibility:
//   - Exclude the viewer's own posts (follower_id <> author).
//   - Exclude drafts; include active / published / completed only.
//   - Respect `matchups.visibility = 'mutuals'` — when set, the author
//     must also follow the viewer back.
//   - Private authors are already handled implicitly: if an author is
//     private and the viewer isn't following them, there's no follows
//     row to match, so nothing surfaces. (When the viewer IS a follower
//     — which is the whole predicate here — the private flag doesn't
//     further restrict.)
//
// Skipped by design:
//   - Bracket-child matchups (matchup_id with bracket_id != NULL) —
//     those are auto-generated when a bracket advances, not a creator
//     action. The parent bracket's creation surfaces separately.
//
// The kind uses the existing `social` category for opt-out, so no
// migration is needed for Step 5 prefs.
func loadFollowedUserPostedActivity(ctx context.Context, db *sqlx.DB, viewerID uint, before time.Time) ([]*activityv1.ActivityItem, error) {
	type row struct {
		InternalID     uint      `db:"internal_id"`
		PublicID       string    `db:"public_id"`
		Title          string    `db:"title"`
		CreatedAt      time.Time `db:"created_at"`
		SubjectType    string    `db:"subject_type"`
		ActorID        string    `db:"actor_id"`
		ActorUsername  string    `db:"actor_username"`
		AuthorUsername string    `db:"author_username"`
	}

	// Cursor goes inside each UNION branch; matches the idiom used by
	// loadLikesReceivedActivity and loadMentionActivity.
	cursorM, cursorB := "", ""
	args := []interface{}{viewerID}
	if !before.IsZero() {
		cursorM = " AND m.created_at < $2"
		cursorB = " AND b.created_at < $2"
		args = append(args, before.UTC())
	}

	query := `SELECT * FROM ((
		  SELECT m.id            AS internal_id,
		         m.public_id     AS public_id,
		         m.title         AS title,
		         m.created_at    AS created_at,
		         'matchup'       AS subject_type,
		         u.public_id     AS actor_id,
		         u.username      AS actor_username,
		         u.username      AS author_username
		  FROM public.matchups m
		  JOIN public.follows f ON f.followed_id = m.author_id AND f.follower_id = $1
		  JOIN public.users u   ON u.id = m.author_id
		  WHERE m.author_id <> $1
		    AND m.status IN ('active', 'published', 'completed')
		    AND m.bracket_id IS NULL
		    AND (m.visibility <> 'mutuals' OR EXISTS (
		        SELECT 1 FROM public.follows f2
		        WHERE f2.follower_id = m.author_id AND f2.followed_id = $1
		    ))` + cursorM + `
		) UNION ALL (
		  SELECT b.id            AS internal_id,
		         b.public_id     AS public_id,
		         b.title         AS title,
		         b.created_at    AS created_at,
		         'bracket'       AS subject_type,
		         u.public_id     AS actor_id,
		         u.username      AS actor_username,
		         u.username      AS author_username
		  FROM public.brackets b
		  JOIN public.follows f ON f.followed_id = b.author_id AND f.follower_id = $1
		  JOIN public.users u   ON u.id = b.author_id
		  WHERE b.author_id <> $1
		    AND b.status IN ('active', 'published', 'completed')
		    AND (b.visibility <> 'mutuals' OR EXISTS (
		        SELECT 1 FROM public.follows f2
		        WHERE f2.follower_id = b.author_id AND f2.followed_id = $1
		    ))` + cursorB + `
		)) t ORDER BY created_at DESC LIMIT ` + strconv.Itoa(perKindQueryLimit)

	var rows []row
	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, err
	}

	out := make([]*activityv1.ActivityItem, 0, len(rows))
	for _, r := range rows {
		actor := r.ActorID
		name := r.ActorUsername
		out = append(out, &activityv1.ActivityItem{
			Id:            fmt.Sprintf("%s:%s:%d", kindFollowedUserPosted, r.SubjectType, r.InternalID),
			Kind:          kindFollowedUserPosted,
			OccurredAt:    rfc3339(r.CreatedAt),
			ActorId:       &actor,
			ActorUsername: &name,
			SubjectType:   r.SubjectType,
			SubjectId:     r.PublicID,
			SubjectTitle:  r.Title,
			Payload: map[string]string{
				"author_username": r.AuthorUsername,
			},
		})
	}
	return out, nil
}
