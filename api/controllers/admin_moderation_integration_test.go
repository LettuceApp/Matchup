//go:build integration

package controllers

import (
	"context"
	"testing"

	adminv1 "Matchup/gen/admin/v1"
	reportv1 "Matchup/gen/report/v1"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
)

// Test coverage for the admin moderation queue. The happy-path spans
// ListReports → ResolveReport → side-effect assertion → admin_actions
// audit row check. Negative paths cover: non-admin blocked, invalid
// resolution rejected, double-resolve racing guard.

// TestListReports_RequiresAdmin — a non-admin caller hits
// PermissionDenied, not NotFound, so they know they reached the right
// endpoint (the server isn't hiding the feature).
func TestListReports_RequiresAdmin(t *testing.T) {
	db := setupTestDB(t)
	u := seedTestUser(t, db, "plain", "p@example.com", "TestPass123")

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), u.ID)
	_, err := h.ListReports(ctx, connectReq(&adminv1.ListReportsRequest{}, ""))
	assertConnectError(t, err, connect.CodePermissionDenied)
}

// TestListReports_ReturnsOpenByDefault — an admin viewing the queue
// without a status filter sees only open reports.
func TestListReports_ReturnsOpenByDefault(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	reporter := seedTestUser(t, db, "src", "src@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Reportable")

	// Seed one open + one resolved report for the same matchup so we
	// can assert the default filter excludes the resolved one.
	if _, err := db.Exec(`
		INSERT INTO reports (reporter_id, subject_type, subject_id, reason, status)
		VALUES ($1, 'matchup', $2, 'harassment', 'open'),
		       ($1, 'matchup', $2, 'spam',       'resolved')`,
		reporter.ID, m.ID,
	); err != nil {
		t.Fatalf("seed reports: %v", err)
	}

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)
	resp, err := h.ListReports(ctx, connectReq(&adminv1.ListReportsRequest{}, ""))
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(resp.Msg.Reports) != 1 {
		t.Fatalf("expected 1 open report, got %d", len(resp.Msg.Reports))
	}
	if resp.Msg.Reports[0].Status != "open" {
		t.Errorf("expected status=open, got %q", resp.Msg.Reports[0].Status)
	}
	if resp.Msg.Reports[0].SubjectType != "matchup" {
		t.Errorf("expected subject_type=matchup, got %q", resp.Msg.Reports[0].SubjectType)
	}
	if resp.Msg.Reports[0].Reason != "harassment" {
		t.Errorf("expected reason=harassment, got %q", resp.Msg.Reports[0].Reason)
	}
	// Subject enrichment should have populated the public UUID + title.
	if resp.Msg.Reports[0].SubjectId != m.PublicID {
		t.Errorf("expected subject_id=%q, got %q", m.PublicID, resp.Msg.Reports[0].SubjectId)
	}
	if resp.Msg.Reports[0].SubjectPreview != "Reportable" {
		t.Errorf("expected preview=Reportable, got %q", resp.Msg.Reports[0].SubjectPreview)
	}
}

// TestResolveReport_DismissClosesAndAudits — the simplest resolution.
// Report flips to resolved, admin_actions picks up one row.
func TestResolveReport_DismissClosesAndAudits(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	reporter := seedTestUser(t, db, "src", "src@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Dismissable")

	var reportID int64
	if err := db.Get(&reportID, `
		INSERT INTO reports (reporter_id, subject_type, subject_id, reason)
		VALUES ($1, 'matchup', $2, 'spam') RETURNING id`,
		reporter.ID, m.ID,
	); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)
	notes := "false positive"
	if _, err := h.ResolveReport(ctx, connectReq(&adminv1.ResolveReportRequest{
		ReportId:   reportID,
		Resolution: "dismiss",
		Notes:      &notes,
	}, "")); err != nil {
		t.Fatalf("ResolveReport: %v", err)
	}

	var status, resolution string
	_ = db.QueryRow("SELECT status, resolution FROM reports WHERE id = $1", reportID).
		Scan(&status, &resolution)
	if status != "resolved" || resolution != "dismiss" {
		t.Errorf("expected resolved/dismiss, got %s/%s", status, resolution)
	}

	var auditCount int
	_ = db.Get(&auditCount, `
		SELECT COUNT(*) FROM admin_actions
		 WHERE actor_id = $1 AND action = 'report_dismissed' AND subject_type = 'matchup' AND subject_id = $2`,
		admin.ID, m.ID,
	)
	if auditCount != 1 {
		t.Errorf("expected 1 admin_actions row, got %d", auditCount)
	}
}

// TestResolveReport_RemoveContent_DeletesMatchup — the most
// consequential resolution. Reports against a matchup that resolve to
// "remove_content" drop the matchup row + its engagement.
func TestResolveReport_RemoveContent_DeletesMatchup(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	reporter := seedTestUser(t, db, "src", "src@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Will be removed")

	// Add a like so we can verify engagement got cleaned up.
	if _, err := db.Exec(
		`INSERT INTO likes (user_id, matchup_id, created_at, updated_at) VALUES ($1, $2, NOW(), NOW())`,
		reporter.ID, m.ID,
	); err != nil {
		t.Fatalf("seed like: %v", err)
	}

	var reportID int64
	_ = db.Get(&reportID, `
		INSERT INTO reports (reporter_id, subject_type, subject_id, reason)
		VALUES ($1, 'matchup', $2, 'harassment') RETURNING id`,
		reporter.ID, m.ID,
	)

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)
	if _, err := h.ResolveReport(ctx, connectReq(&adminv1.ResolveReportRequest{
		ReportId:   reportID,
		Resolution: "remove_content",
	}, "")); err != nil {
		t.Fatalf("ResolveReport: %v", err)
	}

	var matchupCount int
	_ = db.Get(&matchupCount, "SELECT COUNT(*) FROM matchups WHERE id = $1", m.ID)
	if matchupCount != 0 {
		t.Errorf("expected matchup deleted, got count=%d", matchupCount)
	}
	var likeCount int
	_ = db.Get(&likeCount, "SELECT COUNT(*) FROM likes WHERE matchup_id = $1", m.ID)
	if likeCount != 0 {
		t.Errorf("expected likes cleared, got count=%d", likeCount)
	}
}

// TestResolveReport_BanUser_StampsUserAndAudits — ban_user resolves
// the report + stamps deleted_at/banned_at/ban_reason on the author
// + writes the audit row. Ban reason is required.
func TestResolveReport_BanUser_StampsUserAndAudits(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	reporter := seedTestUser(t, db, "src", "src@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Bannable")

	var reportID int64
	_ = db.Get(&reportID, `
		INSERT INTO reports (reporter_id, subject_type, subject_id, reason)
		VALUES ($1, 'matchup', $2, 'harassment') RETURNING id`,
		reporter.ID, m.ID,
	)

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)

	// Missing ban_reason → InvalidArgument.
	_, err := h.ResolveReport(ctx, connectReq(&adminv1.ResolveReportRequest{
		ReportId:   reportID,
		Resolution: "ban_user",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)

	// Happy path with ban_reason.
	banReason := "repeated harassment across multiple matchups"
	if _, err := h.ResolveReport(ctx, connectReq(&adminv1.ResolveReportRequest{
		ReportId:   reportID,
		Resolution: "ban_user",
		BanReason:  &banReason,
	}, "")); err != nil {
		t.Fatalf("ResolveReport: %v", err)
	}

	// User row has deleted_at + banned_at + ban_reason all set.
	var deletedAt, bannedAt, banReasonCol *string
	_ = db.QueryRow(
		"SELECT deleted_at::text, banned_at::text, ban_reason FROM users WHERE id = $1",
		author.ID,
	).Scan(&deletedAt, &bannedAt, &banReasonCol)
	if deletedAt == nil || bannedAt == nil {
		t.Fatalf("expected deleted_at + banned_at set; got %v / %v", deletedAt, bannedAt)
	}
	if banReasonCol == nil || *banReasonCol != banReason {
		t.Errorf("expected ban_reason=%q, got %v", banReason, banReasonCol)
	}

	// Audit row captured the ban.
	var auditCount int
	_ = db.Get(&auditCount,
		`SELECT COUNT(*) FROM admin_actions WHERE action = 'user_banned' AND subject_id = $1`,
		author.ID,
	)
	if auditCount != 1 {
		t.Errorf("expected 1 user_banned audit row, got %d", auditCount)
	}
}

// TestResolveReport_DoubleResolveRejected — two admins can't each
// resolve the same report; the second call hits FailedPrecondition.
func TestResolveReport_DoubleResolveRejected(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	reporter := seedTestUser(t, db, "src", "src@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "Single-touch")

	var reportID int64
	_ = db.Get(&reportID, `
		INSERT INTO reports (reporter_id, subject_type, subject_id, reason)
		VALUES ($1, 'matchup', $2, 'spam') RETURNING id`,
		reporter.ID, m.ID,
	)

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)
	if _, err := h.ResolveReport(ctx, connectReq(&adminv1.ResolveReportRequest{
		ReportId:   reportID,
		Resolution: "dismiss",
	}, "")); err != nil {
		t.Fatalf("first resolve: %v", err)
	}

	_, err := h.ResolveReport(ctx, connectReq(&adminv1.ResolveReportRequest{
		ReportId:   reportID,
		Resolution: "dismiss",
	}, ""))
	assertConnectError(t, err, connect.CodeFailedPrecondition)
}

// TestBanUser_DirectSoftDeletesAndRevokes — BanUser outside a report
// flow. Stamps the user, revokes their refresh tokens, deletes push
// subs, and writes an audit row.
func TestBanUser_DirectSoftDeletesAndRevokes(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	target := seedTestUser(t, db, "badactor", "b@example.com", "TestPass123")

	// Seed a refresh token + push subscription so we can verify they're
	// cleaned up by the ban. Family_id can be any UUID — we just need a
	// live row to confirm revoked_at gets stamped.
	if _, err := db.Exec(
		`INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
		 VALUES ($1, 'hash', gen_random_uuid(), NOW() + INTERVAL '30 days')`,
		target.ID,
	); err != nil {
		t.Fatalf("seed refresh token: %v", err)
	}
	if _, err := db.Exec(
		`INSERT INTO push_subscriptions (user_id, endpoint, p256dh_key, auth_key)
		 VALUES ($1, 'https://example.com/e', 'k', 'a')`,
		target.ID,
	); err != nil {
		t.Fatalf("seed push: %v", err)
	}

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)
	if _, err := h.BanUser(ctx, connectReq(&adminv1.BanUserRequest{
		Id:     target.Username,
		Reason: "spam network",
	}, "")); err != nil {
		t.Fatalf("BanUser: %v", err)
	}

	var deletedAt, bannedAt *string
	_ = db.QueryRow("SELECT deleted_at::text, banned_at::text FROM users WHERE id = $1", target.ID).
		Scan(&deletedAt, &bannedAt)
	if deletedAt == nil || bannedAt == nil {
		t.Errorf("expected deleted_at + banned_at stamped, got %v / %v", deletedAt, bannedAt)
	}

	var pushCount, tokenCount int
	_ = db.Get(&pushCount, "SELECT COUNT(*) FROM push_subscriptions WHERE user_id = $1", target.ID)
	_ = db.Get(&tokenCount, "SELECT COUNT(*) FROM refresh_tokens WHERE user_id = $1 AND revoked_at IS NOT NULL", target.ID)
	if pushCount != 0 {
		t.Errorf("expected push subs wiped, got %d", pushCount)
	}
	if tokenCount != 1 {
		t.Errorf("expected refresh tokens revoked, got %d", tokenCount)
	}
}

// TestUnbanUser_Reactivates — unban clears deleted_at + banned_at +
// ban_reason so the user can log back in. Old engagement stays gone
// (we don't restore follows or content).
func TestUnbanUser_Reactivates(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	target := seedTestUser(t, db, "reformed", "r@example.com", "TestPass123")

	// Pre-seed the banned state.
	if _, err := db.Exec(
		`UPDATE users SET deleted_at=NOW(), banned_at=NOW(), ban_reason='past offense' WHERE id=$1`,
		target.ID,
	); err != nil {
		t.Fatalf("seed ban: %v", err)
	}

	h := &AdminHandler{DB: db, ReadDB: db}
	ctx := httpctx.WithUserID(context.Background(), admin.ID)
	if _, err := h.UnbanUser(ctx, connectReq(&adminv1.UnbanUserRequest{
		Id: target.Username,
	}, "")); err != nil {
		t.Fatalf("UnbanUser: %v", err)
	}

	var deletedAt, bannedAt, banReason *string
	_ = db.QueryRow(
		"SELECT deleted_at::text, banned_at::text, ban_reason FROM users WHERE id = $1",
		target.ID,
	).Scan(&deletedAt, &bannedAt, &banReason)
	if deletedAt != nil || bannedAt != nil || banReason != nil {
		t.Errorf("expected all ban cols cleared; got deleted_at=%v banned_at=%v ban_reason=%v",
			deletedAt, bannedAt, banReason)
	}

	var auditCount int
	_ = db.Get(&auditCount,
		`SELECT COUNT(*) FROM admin_actions WHERE action='user_unbanned' AND subject_id=$1`,
		target.ID,
	)
	if auditCount != 1 {
		t.Errorf("expected 1 unban audit row, got %d", auditCount)
	}
}

// TestReportThenListShowsCreatedReport — round-trips through the
// user-facing ReportContent RPC first, then the admin ListReports sees
// the new row. Ensures the two sides actually agree on the schema.
func TestReportThenListShowsCreatedReport(t *testing.T) {
	db := setupTestDB(t)
	admin := seedAdminUser(t, db, "mod", "mod@example.com", "TestPass123")
	reporter := seedTestUser(t, db, "user", "u@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	m := seedTestMatchup(t, db, author.ID, "End-to-end")

	rh := &ReportHandler{DB: db}
	ctx := httpctx.WithUserID(context.Background(), reporter.ID)
	detail := "consistently toxic comments in this matchup"
	if _, err := rh.ReportContent(ctx, connectReq(&reportv1.ReportContentRequest{
		SubjectType:  "matchup",
		SubjectId:    m.PublicID,
		Reason:       "other",
		ReasonDetail: &detail,
	}, "")); err != nil {
		t.Fatalf("ReportContent: %v", err)
	}

	ah := &AdminHandler{DB: db, ReadDB: db}
	adminCtx := httpctx.WithUserID(context.Background(), admin.ID)
	resp, err := ah.ListReports(adminCtx, connectReq(&adminv1.ListReportsRequest{}, ""))
	if err != nil {
		t.Fatalf("ListReports: %v", err)
	}
	if len(resp.Msg.Reports) != 1 {
		t.Fatalf("expected 1 report, got %d", len(resp.Msg.Reports))
	}
	if resp.Msg.Reports[0].ReasonDetail != detail {
		t.Errorf("expected detail=%q, got %q", detail, resp.Msg.Reports[0].ReasonDetail)
	}
	if resp.Msg.Reports[0].ReporterUsername != "user" {
		t.Errorf("expected reporter=user, got %q", resp.Msg.Reports[0].ReporterUsername)
	}
}
