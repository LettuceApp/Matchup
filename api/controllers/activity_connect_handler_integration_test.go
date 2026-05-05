//go:build integration

package controllers

import (
	"context"
	"testing"
	"time"

	activityv1 "Matchup/gen/activity/v1"
	httpctx "Matchup/utils/httpctx"
)

// Helper — reduces per-test boilerplate for the plain user_id form.
func callActivity(t *testing.T, h *ActivityHandler, ctx context.Context, userID string) *activityv1.GetUserActivityResponse {
	t.Helper()
	resp, err := h.GetUserActivity(ctx, connectReq(&activityv1.GetUserActivityRequest{
		UserId: userID,
	}, ""))
	if err != nil {
		t.Fatalf("GetUserActivity: %v", err)
	}
	return resp.Msg
}

// kindsInResponse returns the set of activity kinds present in the
// response, as a map for O(1) membership checks in assertions.
func kindsInResponse(resp *activityv1.GetUserActivityResponse) map[string]bool {
	out := make(map[string]bool)
	for _, item := range resp.Items {
		out[item.Kind] = true
	}
	return out
}

// TestGetUserActivity_EmptyForFreshUser — a user with no votes, no
// followers, no creators should still get a valid empty response
// rather than an error.
func TestGetUserActivity_EmptyForFreshUser(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "solo", "solo@example.com", "TestPass123")
	h := &ActivityHandler{DB: db, ReadDB: db}

	resp := callActivity(t, h, context.Background(), user.Username)
	if len(resp.Items) != 0 {
		t.Errorf("expected 0 items for user with no activity, got %d", len(resp.Items))
	}
}

// TestGetUserActivity_VoteCast — seeding a matchup_votes row must
// surface as a vote_cast activity item.
func TestGetUserActivity_VoteCast(t *testing.T) {
	db := setupTestDB(t)
	voter := seedTestUser(t, db, "voter", "voter@example.com", "TestPass123")
	owner := seedTestUser(t, db, "author", "author@example.com", "TestPass123")
	matchup := seedTestMatchup(t, db, owner.ID, "Is this a vote cast?")

	// matchup_items are created by seedTestMatchup; pick the first one.
	var itemPublic string
	if err := db.Get(&itemPublic, "SELECT public_id FROM matchup_items WHERE matchup_id = $1 LIMIT 1", matchup.ID); err != nil {
		t.Fatalf("lookup item: %v", err)
	}

	if _, err := db.Exec(
		`INSERT INTO matchup_votes (public_id, user_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
		voter.ID, matchup.PublicID, itemPublic,
	); err != nil {
		t.Fatalf("seed vote: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), voter.Username)

	if !kindsInResponse(resp)["vote_cast"] {
		t.Errorf("expected vote_cast in activity; got %+v", resp.Items)
	}
}

// TestGetUserActivity_VoteWinAndLoss — when two voters back different
// items in a completed matchup, one should see vote_win and the other
// vote_loss. Confirms the CASE predicate in loadVoteOutcomeActivity
// correctly compares voted_item_id against matchup.winner_item_id.
func TestGetUserActivity_VoteWinAndLoss(t *testing.T) {
	db := setupTestDB(t)
	winner := seedTestUser(t, db, "winner", "w@example.com", "TestPass123")
	loser := seedTestUser(t, db, "loser", "l@example.com", "TestPass123")
	owner := seedTestUser(t, db, "author", "a@example.com", "TestPass123")

	m := seedTestMatchup(t, db, owner.ID, "A vs B")

	var items []struct {
		ID       uint   `db:"id"`
		PublicID string `db:"public_id"`
	}
	if err := db.Select(&items, "SELECT id, public_id FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", m.ID); err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(items))
	}
	winItem := items[0]
	loseItem := items[1]

	// Winner voted for items[0]; loser voted for items[1].
	for _, v := range []struct {
		userID     uint
		itemPublic string
	}{
		{winner.ID, winItem.PublicID},
		{loser.ID, loseItem.PublicID},
	} {
		if _, err := db.Exec(
			`INSERT INTO matchup_votes (public_id, user_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
			v.userID, m.PublicID, v.itemPublic,
		); err != nil {
			t.Fatalf("seed vote for %d: %v", v.userID, err)
		}
	}

	// Complete the matchup with item[0] as the winner.
	if _, err := db.Exec(
		"UPDATE matchups SET status = 'completed', winner_item_id = $1, updated_at = NOW() WHERE id = $2",
		winItem.ID, m.ID,
	); err != nil {
		t.Fatalf("complete matchup: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}

	winResp := callActivity(t, h, context.Background(), winner.Username)
	if !kindsInResponse(winResp)["vote_win"] {
		t.Errorf("winner should see vote_win; got kinds %v", kindsInResponse(winResp))
	}

	loseResp := callActivity(t, h, context.Background(), loser.Username)
	if !kindsInResponse(loseResp)["vote_loss"] {
		t.Errorf("loser should see vote_loss; got kinds %v", kindsInResponse(loseResp))
	}
}

// TestGetUserActivity_BracketProgress — a user who voted in a bracket's
// child matchup should see the parent bracket in their feed.
func TestGetUserActivity_BracketProgress(t *testing.T) {
	db := setupTestDB(t)
	voter := seedTestUser(t, db, "voter", "v@example.com", "TestPass123")
	owner := seedTestUser(t, db, "owner", "o@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Tournament", 4)

	// Create a child matchup linked to the bracket.
	m := seedTestMatchup(t, db, owner.ID, "Round 1 - Match 1")
	if _, err := db.Exec("UPDATE matchups SET bracket_id = $1 WHERE id = $2", bracket.ID, m.ID); err != nil {
		t.Fatalf("attach matchup to bracket: %v", err)
	}
	var itemPublic string
	if err := db.Get(&itemPublic, "SELECT public_id FROM matchup_items WHERE matchup_id = $1 LIMIT 1", m.ID); err != nil {
		t.Fatalf("lookup item: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO matchup_votes (public_id, user_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
		voter.ID, m.PublicID, itemPublic,
	); err != nil {
		t.Fatalf("seed vote: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), voter.Username)
	if !kindsInResponse(resp)["bracket_progress"] {
		t.Errorf("expected bracket_progress; got kinds %v", kindsInResponse(resp))
	}
}

// TestGetUserActivity_LikeReceived — someone else liking your matchup
// surfaces as like_received. Also asserts owner's self-like is excluded.
func TestGetUserActivity_LikeReceived(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "o@example.com", "TestPass123")
	fan := seedTestUser(t, db, "fan", "f@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "Likeable")

	// Owner self-likes (should NOT appear).
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		owner.ID, m.ID,
	); err != nil {
		t.Fatalf("seed self-like: %v", err)
	}
	// Fan likes (should appear).
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		fan.ID, m.ID,
	); err != nil {
		t.Fatalf("seed fan like: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), owner.Username)

	var likeItems []*activityv1.ActivityItem
	for _, item := range resp.Items {
		if item.Kind == "like_received" {
			likeItems = append(likeItems, item)
		}
	}
	if len(likeItems) != 1 {
		t.Fatalf("expected exactly 1 like_received (owner self-like should be excluded), got %d", len(likeItems))
	}
	if got := likeItems[0].ActorUsername; got == nil || *got != "fan" {
		t.Errorf("expected actor_username=fan, got %v", got)
	}
}

// TestGetUserActivity_CommentReceived — a comment from another user
// on a bracket you authored shows up as comment_received with the
// bracket as the subject.
func TestGetUserActivity_CommentReceived(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "o@example.com", "TestPass123")
	commenter := seedTestUser(t, db, "commenter", "c@example.com", "TestPass123")
	bracket := seedTestBracket(t, db, owner.ID, "Tournament", 4)

	if _, err := db.Exec(
		`INSERT INTO bracket_comments (user_id, bracket_id, body, created_at, updated_at)
		 VALUES ($1, $2, 'nice', NOW(), NOW())`,
		commenter.ID, bracket.ID,
	); err != nil {
		t.Fatalf("seed bracket comment: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), owner.Username)

	var match *activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "comment_received" {
			match = it
			break
		}
	}
	if match == nil {
		t.Fatal("expected comment_received in feed")
	}
	if match.SubjectType != "bracket" {
		t.Errorf("expected subject_type=bracket, got %q", match.SubjectType)
	}
	if match.Payload["body"] != "nice" {
		t.Errorf("expected payload.body=nice, got %q", match.Payload["body"])
	}
}

// TestGetUserActivity_MatchupVoteReceived — votes on matchups you
// authored show up; your own votes on your own matchup do not.
func TestGetUserActivity_MatchupVoteReceived(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "o@example.com", "TestPass123")
	voter := seedTestUser(t, db, "voter", "v@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "My matchup")

	var itemPublic string
	_ = db.Get(&itemPublic, "SELECT public_id FROM matchup_items WHERE matchup_id = $1 LIMIT 1", m.ID)

	// Owner self-vote (EXCLUDED).
	if _, err := db.Exec(
		`INSERT INTO matchup_votes (public_id, user_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
		owner.ID, m.PublicID, itemPublic,
	); err != nil {
		t.Fatalf("seed owner self-vote: %v", err)
	}
	// Third-party vote (INCLUDED).
	if _, err := db.Exec(
		`INSERT INTO matchup_votes (public_id, user_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
		voter.ID, m.PublicID, itemPublic,
	); err != nil {
		t.Fatalf("seed third-party vote: %v", err)
	}
	// Anonymous vote (INCLUDED — no user_id).
	if _, err := db.Exec(
		`INSERT INTO matchup_votes (public_id, user_id, anon_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), NULL, 'anon-device-xyz', $1, $2, NOW(), NOW())`,
		m.PublicID, itemPublic,
	); err != nil {
		t.Fatalf("seed anon vote: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), owner.Username)

	var received []*activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "matchup_vote_received" {
			received = append(received, it)
		}
	}
	if len(received) != 2 {
		t.Errorf("expected 2 matchup_vote_received (third-party + anon, NOT owner), got %d", len(received))
	}
}

// TestGetUserActivity_NewFollower — a follow relationship surfaces as
// new_follower for the user who was followed.
func TestGetUserActivity_NewFollower(t *testing.T) {
	db := setupTestDB(t)
	followed := seedTestUser(t, db, "famous", "f@example.com", "TestPass123")
	follower := seedTestUser(t, db, "fan", "fan@example.com", "TestPass123")

	if _, err := db.Exec(
		`INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW())`,
		follower.ID, followed.ID,
	); err != nil {
		t.Fatalf("seed follow: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), followed.Username)

	var item *activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "new_follower" {
			item = it
			break
		}
	}
	if item == nil {
		t.Fatal("expected new_follower item")
	}
	if item.ActorUsername == nil || *item.ActorUsername != "fan" {
		t.Errorf("expected actor_username=fan, got %v", item.ActorUsername)
	}
}

// TestGetUserActivity_MentionReceived — comments that mention the
// viewer's username surface as mention_received. We confirm:
//   - a matchup comment mentioning @viewer appears (word-boundary OK)
//   - a matchup comment without the handle does NOT appear
//   - a bracket comment mentioning @viewer appears (UNION leg works)
//   - a self-comment containing @viewer does NOT appear (user_id<>$1)
//   - a comment mentioning `@viewer1234` does NOT appear (word-boundary)
func TestGetUserActivity_MentionReceived(t *testing.T) {
	db := setupTestDB(t)
	viewer := seedTestUser(t, db, "viewer", "v@example.com", "TestPass123")
	commenter := seedTestUser(t, db, "alice", "a@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "auth@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Hot take thread")
	bracket := seedTestBracket(t, db, author.ID, "Bracket talk", 4)

	// 1. Matchup comment that mentions @viewer — should appear.
	if _, err := db.Exec(
		`INSERT INTO comments (user_id, matchup_id, body, created_at, updated_at)
		 VALUES ($1, $2, 'yo @viewer what do you think?', NOW(), NOW())`,
		commenter.ID, m.ID,
	); err != nil {
		t.Fatalf("seed matchup mention: %v", err)
	}
	// 2. Matchup comment without any @-mention — should NOT appear.
	if _, err := db.Exec(
		`INSERT INTO comments (user_id, matchup_id, body, created_at, updated_at)
		 VALUES ($1, $2, 'just some unrelated take', NOW(), NOW())`,
		commenter.ID, m.ID,
	); err != nil {
		t.Fatalf("seed unrelated: %v", err)
	}
	// 3. Bracket comment that mentions @viewer — should appear.
	if _, err := db.Exec(
		`INSERT INTO bracket_comments (user_id, bracket_id, body, created_at, updated_at)
		 VALUES ($1, $2, 'gotta side with @viewer here', NOW(), NOW())`,
		commenter.ID, bracket.ID,
	); err != nil {
		t.Fatalf("seed bracket mention: %v", err)
	}
	// 4. Viewer mentions themselves — should NOT appear (user_id<>$1).
	if _, err := db.Exec(
		`INSERT INTO comments (user_id, matchup_id, body, created_at, updated_at)
		 VALUES ($1, $2, 'reminder to self: @viewer you wrote this', NOW(), NOW())`,
		viewer.ID, m.ID,
	); err != nil {
		t.Fatalf("seed self-mention: %v", err)
	}
	// 5. Near-match that ILIKE catches but the Go regex should reject.
	if _, err := db.Exec(
		`INSERT INTO comments (user_id, matchup_id, body, created_at, updated_at)
		 VALUES ($1, $2, 'nope, meant @viewer1234 not you', NOW(), NOW())`,
		commenter.ID, m.ID,
	); err != nil {
		t.Fatalf("seed near-match: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), viewer.Username)

	var mentions []*activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "mention_received" {
			mentions = append(mentions, it)
		}
	}
	if len(mentions) != 2 {
		t.Fatalf("expected exactly 2 mention_received items, got %d: %+v", len(mentions), mentions)
	}

	// Both mentions carry actor_username + a body snippet + the subject
	// author's username for link construction.
	subjectTypes := map[string]bool{}
	for _, it := range mentions {
		if it.ActorUsername == nil || *it.ActorUsername != "alice" {
			t.Errorf("expected actor_username=alice, got %v", it.ActorUsername)
		}
		if it.Payload["body"] == "" {
			t.Errorf("expected payload.body to be set")
		}
		if it.Payload["author_username"] != "author" {
			t.Errorf("expected payload.author_username=author, got %q", it.Payload["author_username"])
		}
		subjectTypes[it.SubjectType] = true
	}
	if !subjectTypes["matchup"] || !subjectTypes["bracket"] {
		t.Errorf("expected both matchup and bracket subject types; got %v", subjectTypes)
	}
}

// TestGetUserActivity_AuthoredCompleted — a matchup the viewer authored
// that reached status=completed should surface as `matchup_completed`
// with the winner item name in payload. A completed matchup authored
// by someone else must NOT appear. Brackets work the same way.
func TestGetUserActivity_AuthoredCompleted(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	other := seedTestUser(t, db, "other", "o@example.com", "TestPass123")

	// My matchup — completed with a winner declared.
	mine := seedTestMatchup(t, db, me.ID, "My closed matchup")
	var items []struct {
		ID       uint   `db:"id"`
		PublicID string `db:"public_id"`
	}
	if err := db.Select(&items, "SELECT id, public_id FROM matchup_items WHERE matchup_id = $1 ORDER BY id ASC", mine.ID); err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) < 2 {
		t.Fatalf("expected >=2 matchup_items, got %d", len(items))
	}
	if _, err := db.Exec(
		"UPDATE matchups SET status='completed', winner_item_id=$1, updated_at=NOW() WHERE id=$2",
		items[0].ID, mine.ID,
	); err != nil {
		t.Fatalf("complete my matchup: %v", err)
	}

	// Someone else's completed matchup — should NOT appear for me.
	theirs := seedTestMatchup(t, db, other.ID, "Not mine")
	var theirItem uint
	if err := db.Get(&theirItem, "SELECT id FROM matchup_items WHERE matchup_id = $1 LIMIT 1", theirs.ID); err != nil {
		t.Fatalf("lookup their item: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE matchups SET status='completed', winner_item_id=$1, updated_at=NOW() WHERE id=$2",
		theirItem, theirs.ID,
	); err != nil {
		t.Fatalf("complete their matchup: %v", err)
	}

	// My bracket — also completed. Uses a seed helper that creates the
	// bracket row; flip status after creation.
	myBracket := seedTestBracket(t, db, me.ID, "My tournament", 4)
	if _, err := db.Exec(
		"UPDATE brackets SET status='completed', updated_at=NOW() WHERE id=$1", myBracket.ID,
	); err != nil {
		t.Fatalf("complete my bracket: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), me.Username)

	var mcItems, bcItems []*activityv1.ActivityItem
	for _, it := range resp.Items {
		switch it.Kind {
		case "matchup_completed":
			mcItems = append(mcItems, it)
		case "bracket_completed":
			bcItems = append(bcItems, it)
		}
	}
	if len(mcItems) != 1 {
		t.Fatalf("expected exactly 1 matchup_completed (not others'), got %d", len(mcItems))
	}
	if mcItems[0].SubjectTitle != "My closed matchup" {
		t.Errorf("expected my matchup title, got %q", mcItems[0].SubjectTitle)
	}
	if mcItems[0].Payload["winner_item"] == "" {
		t.Errorf("expected winner_item in payload, got empty")
	}
	if mcItems[0].Payload["author_username"] != "me" {
		t.Errorf("expected author_username=me, got %q", mcItems[0].Payload["author_username"])
	}
	if len(bcItems) != 1 {
		t.Fatalf("expected exactly 1 bracket_completed, got %d", len(bcItems))
	}
	if bcItems[0].SubjectTitle != "My tournament" {
		t.Errorf("expected bracket title, got %q", bcItems[0].SubjectTitle)
	}
}

// TestGetUserActivity_Milestone — seeding matchup_items.votes to SUM=100
// and running ScanMilestones should emit a single milestone_reached
// notification for the matchup author. Running the scanner a second
// time is a no-op (unique constraint).
func TestGetUserActivity_Milestone(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Viral matchup")

	// Bump one item's vote count to 100 — SUM across items is 100.
	if _, err := db.Exec(
		"UPDATE matchup_items SET votes = 100 WHERE matchup_id = $1 AND id = (SELECT MIN(id) FROM matchup_items WHERE matchup_id = $1)",
		m.ID,
	); err != nil {
		t.Fatalf("seed votes: %v", err)
	}

	// First scan emits the 10 and 100 thresholds (both crossed).
	if err := ScanMilestones(context.Background(), db); err != nil {
		t.Fatalf("ScanMilestones: %v", err)
	}
	// Second scan is a no-op — confirms idempotency.
	if err := ScanMilestones(context.Background(), db); err != nil {
		t.Fatalf("ScanMilestones (rerun): %v", err)
	}

	// Direct table check — we expect exactly 2 rows (10 + 100 thresholds)
	// for this matchup's votes metric. Higher thresholds (1000, 10000)
	// haven't crossed yet so they don't emit.
	var n int
	if err := db.Get(&n,
		`SELECT COUNT(*) FROM notifications
		 WHERE user_id = $1 AND subject_type='matchup' AND subject_id=$2
		   AND kind='milestone_reached' AND payload->>'metric'='votes'`,
		author.ID, m.ID,
	); err != nil {
		t.Fatalf("count notifications: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 milestone rows for vote thresholds 10+100, got %d", n)
	}

	// Activity handler surfaces them. Subject_id in the proto is the
	// PUBLIC id resolved by loadNotificationsActivity.
	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), author.Username)

	var milestones []*activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "milestone_reached" {
			milestones = append(milestones, it)
		}
	}
	if len(milestones) != 2 {
		t.Fatalf("expected 2 milestone_reached items in feed, got %d", len(milestones))
	}
	for _, it := range milestones {
		if it.SubjectType != "matchup" {
			t.Errorf("expected matchup subject, got %q", it.SubjectType)
		}
		if it.SubjectId != m.PublicID {
			t.Errorf("expected public id %q, got %q", m.PublicID, it.SubjectId)
		}
		if it.SubjectTitle != "Viral matchup" {
			t.Errorf("expected title from payload.title, got %q", it.SubjectTitle)
		}
		if it.Payload["metric"] != "votes" {
			t.Errorf("expected metric=votes, got %q", it.Payload["metric"])
		}
		if it.Payload["threshold"] == "" {
			t.Errorf("expected threshold in payload")
		}
	}
}

// TestGetUserActivity_ClosingSoon — an active matchup whose end_time is
// exactly 60 minutes out should trigger a matchup_closing_soon
// notification to the author. The scanner should be a no-op on a second
// run (idempotent via the dedupe index).
func TestGetUserActivity_ClosingSoon(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Closes soon")

	// Force end_time to NOW()+60 minutes, status='active'. seedTestMatchup
	// defaults vary — we set both explicitly so the predicate hits.
	if _, err := db.Exec(
		"UPDATE matchups SET status='active', end_time = NOW() + INTERVAL '60 minutes' WHERE id = $1",
		m.ID,
	); err != nil {
		t.Fatalf("set end_time: %v", err)
	}

	// Another active matchup that ISN'T within the window — should not fire.
	other := seedTestMatchup(t, db, author.ID, "Plenty of time")
	if _, err := db.Exec(
		"UPDATE matchups SET status='active', end_time = NOW() + INTERVAL '6 hours' WHERE id = $1",
		other.ID,
	); err != nil {
		t.Fatalf("set other end_time: %v", err)
	}

	if err := ScanClosingMatchups(context.Background(), db); err != nil {
		t.Fatalf("ScanClosingMatchups: %v", err)
	}
	// Second pass — idempotency check.
	if err := ScanClosingMatchups(context.Background(), db); err != nil {
		t.Fatalf("ScanClosingMatchups rerun: %v", err)
	}

	var n int
	if err := db.Get(&n, "SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND kind='matchup_closing_soon'", author.ID); err != nil {
		t.Fatalf("count: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected exactly 1 matchup_closing_soon row, got %d", n)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), author.Username)

	var found *activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "matchup_closing_soon" {
			found = it
			break
		}
	}
	if found == nil {
		t.Fatal("expected matchup_closing_soon in feed")
	}
	if found.SubjectTitle != "Closes soon" {
		t.Errorf("expected title 'Closes soon', got %q", found.SubjectTitle)
	}
	if found.Payload["role"] != "author" {
		t.Errorf("expected payload.role=author on author's row, got %q", found.Payload["role"])
	}
}

// TestGetUserActivity_ClosingSoonForVoters — voters on a matchup that's
// about to close should each receive a matchup_closing_soon row with
// role=voter. The author's own row keeps role=author. Anonymous voters
// can't receive notifications (no user account). A re-run is a no-op.
func TestGetUserActivity_ClosingSoonForVoters(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	voter1 := seedTestUser(t, db, "voter1", "v1@example.com", "TestPass123")
	voter2 := seedTestUser(t, db, "voter2", "v2@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Closes soon")

	// Window the matchup into the 59-61 min bracket, status=active.
	if _, err := db.Exec(
		"UPDATE matchups SET status='active', end_time = NOW() + INTERVAL '60 minutes' WHERE id = $1",
		m.ID,
	); err != nil {
		t.Fatalf("set end_time: %v", err)
	}

	var itemPublic string
	if err := db.Get(&itemPublic, "SELECT public_id FROM matchup_items WHERE matchup_id = $1 LIMIT 1", m.ID); err != nil {
		t.Fatalf("lookup item: %v", err)
	}

	// Two authed voters + one anonymous vote. Anonymous must NOT get a
	// row (no user_id to target).
	for _, u := range []uint{voter1.ID, voter2.ID} {
		if _, err := db.Exec(
			`INSERT INTO matchup_votes (public_id, user_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
			 VALUES (gen_random_uuid(), $1, $2, $3, NOW(), NOW())`,
			u, m.PublicID, itemPublic,
		); err != nil {
			t.Fatalf("seed vote for %d: %v", u, err)
		}
	}
	if _, err := db.Exec(
		`INSERT INTO matchup_votes (public_id, user_id, anon_id, matchup_public_id, matchup_item_public_id, created_at, updated_at)
		 VALUES (gen_random_uuid(), NULL, 'anon-xyz', $1, $2, NOW(), NOW())`,
		m.PublicID, itemPublic,
	); err != nil {
		t.Fatalf("seed anon vote: %v", err)
	}

	// First pass: emits author + 2 voters. Second pass: no-op.
	if err := ScanClosingMatchups(context.Background(), db); err != nil {
		t.Fatalf("ScanClosingMatchups: %v", err)
	}
	if err := ScanClosingMatchups(context.Background(), db); err != nil {
		t.Fatalf("ScanClosingMatchups rerun: %v", err)
	}

	// Total rows: 1 (author) + 2 (voters) = 3. Anonymous excluded.
	var total int
	if err := db.Get(&total,
		"SELECT COUNT(*) FROM notifications WHERE kind='matchup_closing_soon' AND subject_id=$1",
		m.ID,
	); err != nil {
		t.Fatalf("count total: %v", err)
	}
	if total != 3 {
		t.Fatalf("expected 3 matchup_closing_soon rows (author + 2 voters), got %d", total)
	}

	// Voter1 sees their voter-role row in the feed.
	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), voter1.Username)

	var found *activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "matchup_closing_soon" {
			found = it
			break
		}
	}
	if found == nil {
		t.Fatal("expected matchup_closing_soon in voter1's feed")
	}
	if found.Payload["role"] != "voter" {
		t.Errorf("expected payload.role=voter on voter's row, got %q", found.Payload["role"])
	}
}

// TestGetUserActivity_TieNeedsResolution — a matchup whose end_time has
// passed, with no winner declared and 2+ items tied at the top vote
// count, should fire a tie_needs_resolution notification to the author.
// A matchup with a single clear leader at max votes should NOT fire.
func TestGetUserActivity_TieNeedsResolution(t *testing.T) {
	db := setupTestDB(t)
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")

	// Tied matchup: past end_time, no winner, 2 items at votes=5.
	tied := seedTestMatchup(t, db, author.ID, "Tied matchup")
	if _, err := db.Exec(
		"UPDATE matchups SET status='active', end_time = NOW() - INTERVAL '1 hour', winner_item_id = NULL WHERE id = $1",
		tied.ID,
	); err != nil {
		t.Fatalf("set tied matchup status: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE matchup_items SET votes = 5 WHERE matchup_id = $1",
		tied.ID,
	); err != nil {
		t.Fatalf("seed tied votes: %v", err)
	}

	// Clear-winner matchup: past end_time, no winner set YET, but one
	// item has strictly more votes than the other — scanner should not
	// fire for this one.
	solo := seedTestMatchup(t, db, author.ID, "Clear leader")
	if _, err := db.Exec(
		"UPDATE matchups SET status='active', end_time = NOW() - INTERVAL '1 hour', winner_item_id = NULL WHERE id = $1",
		solo.ID,
	); err != nil {
		t.Fatalf("set solo status: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE matchup_items SET votes = 10 WHERE matchup_id = $1 AND id = (SELECT MIN(id) FROM matchup_items WHERE matchup_id = $1)",
		solo.ID,
	); err != nil {
		t.Fatalf("seed solo winner: %v", err)
	}
	if _, err := db.Exec(
		"UPDATE matchup_items SET votes = 3 WHERE matchup_id = $1 AND votes = 0",
		solo.ID,
	); err != nil {
		t.Fatalf("seed solo loser: %v", err)
	}

	if err := ScanTiesNeedingResolution(context.Background(), db); err != nil {
		t.Fatalf("ScanTiesNeedingResolution: %v", err)
	}
	if err := ScanTiesNeedingResolution(context.Background(), db); err != nil {
		t.Fatalf("ScanTiesNeedingResolution rerun: %v", err)
	}

	var ties []struct {
		SubjectID uint   `db:"subject_id"`
		Title     string `db:"title"`
	}
	if err := db.Select(&ties,
		`SELECT n.subject_id, n.payload->>'title' AS title
		 FROM notifications n
		 WHERE n.user_id = $1 AND n.kind = 'tie_needs_resolution'`,
		author.ID,
	); err != nil {
		t.Fatalf("select ties: %v", err)
	}
	if len(ties) != 1 {
		t.Fatalf("expected exactly 1 tie_needs_resolution row, got %d: %+v", len(ties), ties)
	}
	if ties[0].SubjectID != tied.ID {
		t.Errorf("expected tied matchup %d, got %d", tied.ID, ties[0].SubjectID)
	}
}

// TestGetUserActivity_RespectsNotificationPrefs — muting the
// `engagement` category should hide like_received items from the feed.
// Flipping it back ON surfaces them again.
func TestGetUserActivity_RespectsNotificationPrefs(t *testing.T) {
	db := setupTestDB(t)
	owner := seedTestUser(t, db, "owner", "o@example.com", "TestPass123")
	fan := seedTestUser(t, db, "fan", "f@example.com", "TestPass123")
	m := seedTestMatchup(t, db, owner.ID, "Mutable feed")

	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		fan.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}

	// Default prefs (all on) → like_received appears.
	resp := callActivity(t, h, context.Background(), owner.Username)
	if !kindsInResponse(resp)["like_received"] {
		t.Fatal("expected like_received under default prefs")
	}

	// Turn engagement off.
	if _, err := db.Exec(
		`UPDATE users SET notification_prefs = $1::jsonb WHERE id = $2`,
		`{"mention":true,"engagement":false,"milestone":true,"prompt":true,"social":true}`,
		owner.ID,
	); err != nil {
		t.Fatalf("mute engagement: %v", err)
	}

	resp = callActivity(t, h, context.Background(), owner.Username)
	if kindsInResponse(resp)["like_received"] {
		t.Errorf("expected like_received to be filtered out when engagement is muted, got %v", kindsInResponse(resp))
	}

	// Turn engagement back on.
	if _, err := db.Exec(
		`UPDATE users SET notification_prefs = $1::jsonb WHERE id = $2`,
		`{"mention":true,"engagement":true,"milestone":true,"prompt":true,"social":true}`,
		owner.ID,
	); err != nil {
		t.Fatalf("unmute engagement: %v", err)
	}
	resp = callActivity(t, h, context.Background(), owner.Username)
	if !kindsInResponse(resp)["like_received"] {
		t.Error("expected like_received to reappear after re-enabling engagement")
	}
}

// TestGetUserActivity_FollowedUserPosted — surfaces new standalone
// matchups and brackets from users the viewer follows. Verifies the
// key exclusions: not-followed users, own content, bracket-child
// auto-matchups, and drafts must all be filtered out.
func TestGetUserActivity_FollowedUserPosted(t *testing.T) {
	db := setupTestDB(t)
	viewer := seedTestUser(t, db, "viewer", "v@example.com", "TestPass123")
	creator := seedTestUser(t, db, "creator", "c@example.com", "TestPass123")
	stranger := seedTestUser(t, db, "stranger", "s@example.com", "TestPass123")

	// Viewer follows creator (but NOT stranger).
	if _, err := db.Exec(
		`INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, NOW())`,
		viewer.ID, creator.ID,
	); err != nil {
		t.Fatalf("seed follow: %v", err)
	}

	// (1) Creator posts a standalone matchup → should appear.
	standalone := seedTestMatchup(t, db, creator.ID, "Creator's hot take")

	// (2) Creator starts a bracket → should appear after we flip status
	//     off 'draft' (the helper defaults to 'draft').
	bracket := seedTestBracket(t, db, creator.ID, "Creator's tournament", 4)
	if _, err := db.Exec("UPDATE brackets SET status='active' WHERE id=$1", bracket.ID); err != nil {
		t.Fatalf("activate bracket: %v", err)
	}

	// (3) Stranger posts a matchup → should NOT appear (not followed).
	strangerMatchup := seedTestMatchup(t, db, stranger.ID, "Not in your feed")

	// (4) Viewer posts their own → should NOT appear (author = viewer).
	ownMatchup := seedTestMatchup(t, db, viewer.ID, "My own post")

	// (5) Creator's bracket spawns a child matchup → should NOT appear
	//     (bracket_id IS NOT NULL filter excludes auto-generated rows).
	childMatchup := seedTestMatchup(t, db, creator.ID, "Round 1 · Match 1")
	if _, err := db.Exec(
		"UPDATE matchups SET bracket_id = $1 WHERE id = $2", bracket.ID, childMatchup.ID,
	); err != nil {
		t.Fatalf("attach child matchup to bracket: %v", err)
	}

	// (6) Creator saves a draft → should NOT appear (status filter).
	draftMatchup := seedTestMatchup(t, db, creator.ID, "Still cooking")
	if _, err := db.Exec(
		"UPDATE matchups SET status='draft' WHERE id=$1", draftMatchup.ID,
	); err != nil {
		t.Fatalf("set draft: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), viewer.Username)

	// Bucket the followed_user_posted rows and assert: exactly two —
	// the standalone matchup + the bracket. Others must be absent.
	var items []*activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "followed_user_posted" {
			items = append(items, it)
		}
	}
	if len(items) != 2 {
		t.Fatalf("expected exactly 2 followed_user_posted items, got %d: %+v", len(items), items)
	}

	// Extract subject IDs so we can assert the right two surfaced and
	// the forbidden ones did not.
	got := map[string]bool{}
	for _, it := range items {
		got[it.SubjectType+":"+it.SubjectId] = true
		if it.ActorUsername == nil || *it.ActorUsername != "creator" {
			t.Errorf("expected actor=creator, got %v", it.ActorUsername)
		}
		if it.Payload["author_username"] != "creator" {
			t.Errorf("expected payload.author_username=creator, got %q", it.Payload["author_username"])
		}
	}
	if !got["matchup:"+standalone.PublicID] {
		t.Errorf("expected creator's standalone matchup in feed")
	}
	if !got["bracket:"+bracket.PublicID] {
		t.Errorf("expected creator's bracket in feed")
	}
	if got["matchup:"+strangerMatchup.PublicID] {
		t.Errorf("stranger's matchup should not surface")
	}
	if got["matchup:"+ownMatchup.PublicID] {
		t.Errorf("viewer's own matchup should not surface")
	}
	if got["matchup:"+childMatchup.PublicID] {
		t.Errorf("bracket-child matchup should not surface (double-count with bracket)")
	}
	if got["matchup:"+draftMatchup.PublicID] {
		t.Errorf("draft matchup should not surface")
	}
}

// TestMarkActivityRead_StampsUnreadRowsUpToCursor — calls the new RPC
// with a cursor timestamp and verifies only the rows older-or-equal get
// stamped. Re-running it is a no-op because the predicate excludes
// already-stamped rows.
func TestMarkActivityRead_StampsUnreadRowsUpToCursor(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	m := seedTestMatchup(t, db, me.ID, "Some matchup")

	// Seed three notification rows with staggered timestamps. jsonb
	// payload is minimal — this test cares about read_at stamping only.
	now := time.Now().UTC()
	for i, offset := range []time.Duration{-2 * time.Hour, -1 * time.Hour, -30 * time.Minute} {
		if _, err := db.Exec(
			`INSERT INTO notifications (user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
			 VALUES ($1, 'milestone_reached', 'matchup', $2, $3, '{"metric":"votes"}'::jsonb, $4)`,
			me.ID, m.ID, 10*(i+1), now.Add(offset),
		); err != nil {
			t.Fatalf("seed notification %d: %v", i, err)
		}
	}

	// Cursor: 45 minutes ago. Stamps rows at -2h and -1h but NOT the
	// -30m one which is newer.
	cursor := now.Add(-45 * time.Minute).Format(time.RFC3339)
	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}

	before := cursor
	resp, err := h.MarkActivityRead(ctx, connectReq(&activityv1.MarkActivityReadRequest{
		Before: &before,
	}, ""))
	if err != nil {
		t.Fatalf("MarkActivityRead: %v", err)
	}
	if resp.Msg.MarkedCount != 2 {
		t.Errorf("expected marked_count=2, got %d", resp.Msg.MarkedCount)
	}

	// DB check: exactly 2 rows now have read_at set; 1 row (the newest)
	// stays null.
	var stampedCount, unstampedCount int
	if err := db.Get(&stampedCount,
		"SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND read_at IS NOT NULL", me.ID,
	); err != nil {
		t.Fatalf("count stamped: %v", err)
	}
	if err := db.Get(&unstampedCount,
		"SELECT COUNT(*) FROM notifications WHERE user_id=$1 AND read_at IS NULL", me.ID,
	); err != nil {
		t.Fatalf("count unstamped: %v", err)
	}
	if stampedCount != 2 || unstampedCount != 1 {
		t.Errorf("expected 2 stamped + 1 unstamped, got %d + %d", stampedCount, unstampedCount)
	}

	// Second call with the same cursor should mark nothing (idempotent).
	resp2, err := h.MarkActivityRead(ctx, connectReq(&activityv1.MarkActivityReadRequest{
		Before: &before,
	}, ""))
	if err != nil {
		t.Fatalf("MarkActivityRead rerun: %v", err)
	}
	if resp2.Msg.MarkedCount != 0 {
		t.Errorf("rerun should stamp 0 rows, got %d", resp2.Msg.MarkedCount)
	}
}

// TestMarkActivityRead_DefaultCursorStampsAll — no `before` means the
// handler uses NOW() and stamps everything.
func TestMarkActivityRead_DefaultCursorStampsAll(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	m := seedTestMatchup(t, db, me.ID, "Some matchup")

	for i := 0; i < 3; i++ {
		if _, err := db.Exec(
			`INSERT INTO notifications (user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
			 VALUES ($1, 'milestone_reached', 'matchup', $2, $3, '{}'::jsonb, NOW() - INTERVAL '1 minute')`,
			me.ID, m.ID, 10*(i+1),
		); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	ctx := httpctx.WithUserID(context.Background(), me.ID)
	h := &ActivityHandler{DB: db, ReadDB: db}
	resp, err := h.MarkActivityRead(ctx, connectReq(&activityv1.MarkActivityReadRequest{}, ""))
	if err != nil {
		t.Fatalf("MarkActivityRead: %v", err)
	}
	if resp.Msg.MarkedCount != 3 {
		t.Errorf("expected 3, got %d", resp.Msg.MarkedCount)
	}
}

// TestMarkActivityRead_RejectsAnon — no auth context → 401.
func TestMarkActivityRead_RejectsAnon(t *testing.T) {
	db := setupTestDB(t)
	h := &ActivityHandler{DB: db, ReadDB: db}
	_, err := h.MarkActivityRead(context.Background(), connectReq(&activityv1.MarkActivityReadRequest{}, ""))
	if err == nil {
		t.Fatal("expected unauthenticated error, got nil")
	}
}

// TestGetUserActivity_ExposesReadAt — after stamping, the subsequent
// feed read returns read_at populated on the stamped row and null on
// the newer unstamped row.
func TestGetUserActivity_ExposesReadAt(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "me", "me@example.com", "TestPass123")
	m := seedTestMatchup(t, db, me.ID, "Some matchup")

	// Two rows: one old (will be stamped), one new (will not).
	if _, err := db.Exec(
		`INSERT INTO notifications (user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at, read_at)
		 VALUES ($1, 'milestone_reached', 'matchup', $2, 10, '{}'::jsonb, NOW() - INTERVAL '2 hours', NOW())`,
		me.ID, m.ID,
	); err != nil {
		t.Fatalf("seed stamped: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO notifications (user_id, kind, subject_type, subject_id, threshold_int, payload, occurred_at)
		 VALUES ($1, 'milestone_reached', 'matchup', $2, 100, '{}'::jsonb, NOW())`,
		me.ID, m.ID,
	); err != nil {
		t.Fatalf("seed unstamped: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), me.Username)

	var milestones []*activityv1.ActivityItem
	for _, it := range resp.Items {
		if it.Kind == "milestone_reached" {
			milestones = append(milestones, it)
		}
	}
	if len(milestones) != 2 {
		t.Fatalf("expected 2 milestone items, got %d", len(milestones))
	}

	// Response is DESC by occurred_at; [0] is the newer (unstamped)
	// row, [1] is older (stamped).
	if milestones[0].ReadAt != nil {
		t.Errorf("newer row should have nil read_at, got %v", *milestones[0].ReadAt)
	}
	if milestones[1].ReadAt == nil || *milestones[1].ReadAt == "" {
		t.Errorf("older row should have read_at populated")
	}
}

// TestGetUserActivity_MergeAndSort — with multiple kinds seeded at
// staggered timestamps, the merged feed must be returned in occurred_at
// DESC order. This locks down the fan-out+merge correctness.
func TestGetUserActivity_MergeAndSort(t *testing.T) {
	db := setupTestDB(t)
	user := seedTestUser(t, db, "busy", "b@example.com", "TestPass123")
	other := seedTestUser(t, db, "other", "o@example.com", "TestPass123")

	m := seedTestMatchup(t, db, user.ID, "My matchup")
	var itemPublic string
	_ = db.Get(&itemPublic, "SELECT public_id FROM matchup_items WHERE matchup_id = $1 LIMIT 1", m.ID)

	// Event 1: 3 minutes ago — someone liked my matchup.
	now := time.Now()
	ts1 := now.Add(-3 * time.Minute)
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, $3, $3)`,
		other.ID, m.ID, ts1,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}

	// Event 2: 1 minute ago — someone followed me.
	ts2 := now.Add(-1 * time.Minute)
	if _, err := db.Exec(
		`INSERT INTO follows (follower_id, followed_id, created_at) VALUES ($1, $2, $3)`,
		other.ID, user.ID, ts2,
	); err != nil {
		t.Fatalf("seed follow: %v", err)
	}

	h := &ActivityHandler{DB: db, ReadDB: db}
	resp := callActivity(t, h, context.Background(), user.Username)
	if len(resp.Items) < 2 {
		t.Fatalf("expected at least 2 items, got %d", len(resp.Items))
	}

	// First item (newest) should be the follow; the like is older.
	if resp.Items[0].Kind != "new_follower" {
		t.Errorf("expected new_follower first (newest), got %q", resp.Items[0].Kind)
	}
}
