//go:build integration

package controllers

import (
	"context"
	"testing"

	reportv1 "Matchup/gen/report/v1"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
)

// reportSubjectUUID returns a matchup public_id for seed convenience —
// any kind works; matchup is the most common case and already has a
// seed helper.
func reportSubjectUUID(t *testing.T, handler *ReportHandler, authorID uint) (subjectID string, matchupInternalID uint) {
	t.Helper()
	m := seedTestMatchup(t, handler.DB, authorID, "Subject of a report")
	return m.PublicID, m.ID
}

// TestReportContent_HappyPath — authed caller submits a valid report;
// row lands in `reports` with status=open + reason recorded.
func TestReportContent_HappyPath(t *testing.T) {
	db := setupTestDB(t)
	reporter := seedTestUser(t, db, "reporter", "reporter@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "author@example.com", "TestPass123")
	handler := &ReportHandler{DB: db}
	subjectUUID, matchupID := reportSubjectUUID(t, handler, author.ID)

	ctx := httpctx.WithUserID(context.Background(), reporter.ID)
	detail := "looks like spam to me"
	resp, err := handler.ReportContent(ctx, connectReq(&reportv1.ReportContentRequest{
		SubjectType:  "matchup",
		SubjectId:    subjectUUID,
		Reason:       "spam",
		ReasonDetail: &detail,
	}, ""))
	if err != nil {
		t.Fatalf("ReportContent: %v", err)
	}
	if resp.Msg.Id == 0 {
		t.Error("expected non-zero report id")
	}

	// Row details match.
	var row struct {
		ReporterID     uint    `db:"reporter_id"`
		SubjectType    string  `db:"subject_type"`
		SubjectID      uint    `db:"subject_id"`
		Reason         string  `db:"reason"`
		ReasonDetail   *string `db:"reason_detail"`
		Status         string  `db:"status"`
	}
	if err := db.Get(&row,
		"SELECT reporter_id, subject_type, subject_id, reason, reason_detail, status FROM reports WHERE id = $1",
		resp.Msg.Id,
	); err != nil {
		t.Fatalf("read back: %v", err)
	}
	if row.ReporterID != reporter.ID {
		t.Errorf("reporter_id = %d, want %d", row.ReporterID, reporter.ID)
	}
	if row.SubjectType != "matchup" {
		t.Errorf("subject_type = %q", row.SubjectType)
	}
	if row.SubjectID != matchupID {
		t.Errorf("subject_id = %d, want %d (internal matchup id)", row.SubjectID, matchupID)
	}
	if row.Reason != "spam" {
		t.Errorf("reason = %q", row.Reason)
	}
	if row.ReasonDetail == nil || *row.ReasonDetail != detail {
		t.Errorf("reason_detail = %v, want %q", row.ReasonDetail, detail)
	}
	if row.Status != "open" {
		t.Errorf("status = %q, want open", row.Status)
	}
}

// TestReportContent_OtherRequiresDetail — reason='other' without a
// detail is an InvalidArgument before any DB work.
func TestReportContent_OtherRequiresDetail(t *testing.T) {
	db := setupTestDB(t)
	reporter := seedTestUser(t, db, "reporter", "reporter@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "author@example.com", "TestPass123")
	handler := &ReportHandler{DB: db}
	subjectUUID, _ := reportSubjectUUID(t, handler, author.ID)

	ctx := httpctx.WithUserID(context.Background(), reporter.ID)
	_, err := handler.ReportContent(ctx, connectReq(&reportv1.ReportContentRequest{
		SubjectType: "matchup",
		SubjectId:   subjectUUID,
		Reason:      "other",
		// ReasonDetail intentionally absent
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

// TestReportContent_RejectsSelfReport — users can't report their own
// account. Minor prevention for the profile "report this user" flow.
func TestReportContent_RejectsSelfReport(t *testing.T) {
	db := setupTestDB(t)
	me := seedTestUser(t, db, "alone", "alone@example.com", "TestPass123")
	handler := &ReportHandler{DB: db}

	ctx := httpctx.WithUserID(context.Background(), me.ID)
	_, err := handler.ReportContent(ctx, connectReq(&reportv1.ReportContentRequest{
		SubjectType: "user",
		SubjectId:   me.PublicID,
		Reason:      "harassment",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

// TestReportContent_UnknownReasonRejected — closed vocabulary, no
// silent fallback to "other".
func TestReportContent_UnknownReasonRejected(t *testing.T) {
	db := setupTestDB(t)
	reporter := seedTestUser(t, db, "reporter", "r@example.com", "TestPass123")
	author := seedTestUser(t, db, "author", "a@example.com", "TestPass123")
	handler := &ReportHandler{DB: db}
	subjectUUID, _ := reportSubjectUUID(t, handler, author.ID)

	ctx := httpctx.WithUserID(context.Background(), reporter.ID)
	_, err := handler.ReportContent(ctx, connectReq(&reportv1.ReportContentRequest{
		SubjectType: "matchup",
		SubjectId:   subjectUUID,
		Reason:      "gibberish",
	}, ""))
	assertConnectError(t, err, connect.CodeInvalidArgument)
}

// TestReportContent_RejectsAnon — no auth context → 401.
func TestReportContent_RejectsAnon(t *testing.T) {
	db := setupTestDB(t)
	handler := &ReportHandler{DB: db}
	_, err := handler.ReportContent(context.Background(), connectReq(&reportv1.ReportContentRequest{
		SubjectType: "user",
		SubjectId:   "any",
		Reason:      "spam",
	}, ""))
	assertConnectError(t, err, connect.CodeUnauthenticated)
}

// TestReportContent_SubjectNotFound — resolver returns CodeNotFound
// for a bogus public_id (not Internal / not InvalidArgument).
func TestReportContent_SubjectNotFound(t *testing.T) {
	db := setupTestDB(t)
	reporter := seedTestUser(t, db, "reporter", "r@example.com", "TestPass123")
	handler := &ReportHandler{DB: db}

	ctx := httpctx.WithUserID(context.Background(), reporter.ID)
	_, err := handler.ReportContent(ctx, connectReq(&reportv1.ReportContentRequest{
		SubjectType: "matchup",
		SubjectId:   "00000000-0000-0000-0000-000000000000",
		Reason:      "spam",
	}, ""))
	assertConnectError(t, err, connect.CodeNotFound)
}
