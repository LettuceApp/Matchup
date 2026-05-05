package controllers

import (
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	adminv1 "Matchup/gen/admin/v1"
	"Matchup/models"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// Admin-side moderation handlers: the review queue + the two ban
// endpoints. All four mount on AdminHandler (declared in
// admin_connect_handler.go) — this file just adds methods to the
// existing type so the base handler file stays focused on the
// already-shipped admin operations.

// reportsPageDefault is the per-page cap for ListReports. The admin
// UI paginates, so there's no user-facing reason to go higher; keeping
// the default small also bounds the JOIN work.
const reportsPageDefault = 25

// allowedReportStatuses — the subset of values the client can pass.
// Empty string maps to "open" in the handler. Anything outside this
// set is an InvalidArgument.
var allowedReportStatuses = map[string]bool{
	"":         true,
	"open":     true,
	"resolved": true,
}

// allowedResolutions mirrors the server-side CHECK on reports.resolution.
var allowedResolutions = map[string]bool{
	"dismiss":        true,
	"remove_content": true,
	"warn_user":      true,
	"ban_user":       true,
}

// reportsCursor — the paginator walks (created_at DESC, id DESC) so
// two rows with the same timestamp resolve deterministically. Encoded
// as a base64 string to stay opaque to clients.
type reportsCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        int64     `json:"i"`
}

func encodeReportsCursor(c reportsCursor) string {
	raw := fmt.Sprintf("%s|%d", c.CreatedAt.Format(time.RFC3339Nano), c.ID)
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeReportsCursor(s string) (*reportsCursor, error) {
	if s == "" {
		return nil, nil
	}
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("bad cursor")
	}
	t, err := time.Parse(time.RFC3339Nano, parts[0])
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, err
	}
	return &reportsCursor{CreatedAt: t, ID: id}, nil
}

// reportJoinRow is the flattened shape we pull back from the big JOIN —
// reporter username + reviewer username + the subject preview, all
// denormalised so the client can render without additional fetches.
type reportJoinRow struct {
	ID                 int64          `db:"id"`
	SubjectType        string         `db:"subject_type"`
	SubjectIDRaw       int64          `db:"subject_id"`
	Reason             string         `db:"reason"`
	ReasonDetail       sql.NullString `db:"reason_detail"`
	Status             string         `db:"status"`
	Resolution         sql.NullString `db:"resolution"`
	CreatedAt          time.Time      `db:"created_at"`
	ReviewedAt         sql.NullTime   `db:"reviewed_at"`
	ReporterID         sql.NullInt64  `db:"reporter_id"`
	ReporterUsername   sql.NullString `db:"reporter_username"`
	ReviewedByUsername sql.NullString `db:"reviewed_by_username"`
}

// ListReports returns the moderation queue. Non-admins get 403; admins
// get open reports by default, filterable by status.
func (h *AdminHandler) ListReports(ctx context.Context, req *connect.Request[adminv1.ListReportsRequest]) (*connect.Response[adminv1.ListReportsResponse], error) {
	if err := requireAdmin(ctx, h.DB); err != nil {
		return nil, err
	}

	status := req.Msg.GetStatus()
	if !allowedReportStatuses[status] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid status filter"))
	}
	if status == "" {
		status = "open"
	}

	limit := int(req.Msg.GetLimit())
	if limit < 1 || limit > 100 {
		limit = reportsPageDefault
	}

	cursor, err := decodeReportsCursor(req.Msg.GetCursor())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor"))
	}

	db := dbForRead(ctx, h.DB, h.ReadDB)

	// The LEFT JOINs keep the query resilient to reporter-deleted +
	// reviewer-deleted rows — the reports.reporter_id + reviewed_by
	// columns ON DELETE SET NULL, so a reporter's own deletion can
	// leave that id null.
	var rows []reportJoinRow
	query := `
		SELECT r.id, r.subject_type, r.subject_id, r.reason, r.reason_detail,
		       r.status, r.resolution, r.created_at, r.reviewed_at,
		       r.reporter_id,
		       reporter.username AS reporter_username,
		       reviewer.username AS reviewed_by_username
		  FROM public.reports r
		  LEFT JOIN public.users reporter ON reporter.id = r.reporter_id
		  LEFT JOIN public.users reviewer ON reviewer.id = r.reviewed_by
		 WHERE r.status = $1`
	args := []interface{}{status}

	if cursor != nil {
		query += ` AND ((r.created_at < $2) OR (r.created_at = $2 AND r.id < $3))`
		args = append(args, cursor.CreatedAt, cursor.ID)
	}
	query += ` ORDER BY r.created_at DESC, r.id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)

	if err := sqlx.SelectContext(ctx, db, &rows, query, args...); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	var nextCursor string
	if len(rows) > limit {
		last := rows[limit-1]
		nextCursor = encodeReportsCursor(reportsCursor{CreatedAt: last.CreatedAt, ID: last.ID})
		rows = rows[:limit]
	}

	// For every row, resolve the subject's display id (UUID for
	// matchup/bracket, comment public_id, username for user) + a
	// short preview string. Done in one batch per subject_type so we
	// cap the round-trips.
	subjectIDs := map[string][]int64{}
	for _, r := range rows {
		subjectIDs[r.SubjectType] = append(subjectIDs[r.SubjectType], r.SubjectIDRaw)
	}
	enriched := enrichReportSubjects(ctx, db, subjectIDs)

	out := make([]*adminv1.AdminReport, 0, len(rows))
	for _, r := range rows {
		info := enriched[reportSubjectKey(r.SubjectType, r.SubjectIDRaw)]
		ar := &adminv1.AdminReport{
			Id:              r.ID,
			SubjectType:     r.SubjectType,
			SubjectId:       info.publicID,
			SubjectPreview:  info.preview,
			Reason:          r.Reason,
			ReasonDetail:    r.ReasonDetail.String,
			Status:          r.Status,
			Resolution:      r.Resolution.String,
			ReporterId:      r.ReporterID.Int64,
			ReporterUsername: r.ReporterUsername.String,
			CreatedAt:       r.CreatedAt.Format(time.RFC3339),
			ReviewedByUsername: r.ReviewedByUsername.String,
		}
		if r.ReviewedAt.Valid {
			ar.ReviewedAt = r.ReviewedAt.Time.Format(time.RFC3339)
		}
		out = append(out, ar)
	}

	resp := &adminv1.ListReportsResponse{Reports: out}
	if nextCursor != "" {
		resp.NextCursor = &nextCursor
	}
	return connect.NewResponse(resp), nil
}

// ResolveReport closes a report + writes the admin_actions audit row +
// executes the side effect for remove_content / ban_user. Runs inside
// a transaction so audit + mutation are atomic.
func (h *AdminHandler) ResolveReport(ctx context.Context, req *connect.Request[adminv1.ResolveReportRequest]) (*connect.Response[adminv1.ResolveReportResponse], error) {
	if err := requireAdmin(ctx, h.DB); err != nil {
		return nil, err
	}
	actorID, _ := httpctx.CurrentUserID(ctx)

	resolution := req.Msg.GetResolution()
	if !allowedResolutions[resolution] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid resolution"))
	}
	if req.Msg.GetReportId() == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("report_id required"))
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	// Lock the report row so two concurrent admins can't double-resolve.
	var report struct {
		ID          int64         `db:"id"`
		SubjectType string        `db:"subject_type"`
		SubjectID   int64         `db:"subject_id"`
		Status      string        `db:"status"`
		ReporterID  sql.NullInt64 `db:"reporter_id"`
	}
	if err := tx.GetContext(ctx, &report,
		`SELECT id, subject_type, subject_id, status, reporter_id
		   FROM public.reports WHERE id = $1 FOR UPDATE`,
		req.Msg.GetReportId(),
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("report not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if report.Status == "resolved" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("report already resolved"))
	}

	// Mark resolved first — every branch below writes to this row.
	if _, err := tx.ExecContext(ctx,
		`UPDATE public.reports
		    SET status = 'resolved',
		        resolution = $1,
		        reviewed_at = NOW(),
		        reviewed_by = $2
		  WHERE id = $3`,
		resolution, actorID, report.ID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	// Side effects. Each branch writes a descriptive admin_actions row
	// so the audit trail records exactly what happened.
	notes := req.Msg.GetNotes()
	switch resolution {
	case "dismiss":
		if err := writeAdminAction(ctx, tx, actorID, "report_dismissed",
			report.SubjectType, report.SubjectID, notes); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

	case "warn_user":
		if err := writeAdminAction(ctx, tx, actorID, "user_warned",
			report.SubjectType, report.SubjectID, notes); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

	case "remove_content":
		if err := removeReportedContent(ctx, tx, report.SubjectType, report.SubjectID); err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("remove content: %w", err))
		}
		if err := writeAdminAction(ctx, tx, actorID, "content_removed",
			report.SubjectType, report.SubjectID, notes); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}

	case "ban_user":
		banReason := strings.TrimSpace(req.Msg.GetBanReason())
		if banReason == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("ban_reason required for ban_user"))
		}
		target, err := resolveSubjectAuthorID(ctx, tx, report.SubjectType, report.SubjectID)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if target == 0 {
			return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("ban: subject has no author to ban"))
		}
		if err := banUserInTx(ctx, tx, target, banReason); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		if err := writeAdminAction(ctx, tx, actorID, "user_banned",
			"user", int64(target), notes+" | reason="+banReason); err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	return connect.NewResponse(&adminv1.ResolveReportResponse{
		Message: "Report resolved.",
	}), nil
}

// BanUser = direct admin action (no report required). Same semantics as
// the ban branch of ResolveReport but without a report row.
func (h *AdminHandler) BanUser(ctx context.Context, req *connect.Request[adminv1.BanUserRequest]) (*connect.Response[adminv1.BanUserResponse], error) {
	if err := requireAdmin(ctx, h.DB); err != nil {
		return nil, err
	}
	actorID, _ := httpctx.CurrentUserID(ctx)

	reason := strings.TrimSpace(req.Msg.GetReason())
	if reason == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("reason required"))
	}

	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := banUserInTx(ctx, tx, target.ID, reason); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := writeAdminAction(ctx, tx, actorID, "user_banned",
		"user", int64(target.ID), "direct ban | reason="+reason); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&adminv1.BanUserResponse{Message: "User banned."}), nil
}

// UnbanUser clears deleted_at + banned_at + ban_reason in one write so
// the user can log back in. Does NOT restore their follow edges or
// engagement rows.
func (h *AdminHandler) UnbanUser(ctx context.Context, req *connect.Request[adminv1.UnbanUserRequest]) (*connect.Response[adminv1.UnbanUserResponse], error) {
	if err := requireAdmin(ctx, h.DB); err != nil {
		return nil, err
	}
	actorID, _ := httpctx.CurrentUserID(ctx)

	target, err := resolveUserByIdentifier(h.DB, req.Msg.GetId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user not found"))
	}

	tx, err := h.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		`UPDATE public.users
		    SET deleted_at = NULL,
		        banned_at = NULL,
		        ban_reason = NULL,
		        deletion_reason = NULL
		  WHERE id = $1`,
		target.ID,
	); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := writeAdminAction(ctx, tx, actorID, "user_unbanned",
		"user", int64(target.ID), ""); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&adminv1.UnbanUserResponse{Message: "User unbanned."}), nil
}

// ---- helpers ----

// requireAdmin authorises the caller. Returns Unauthenticated when no
// user is signed in; PermissionDenied when they aren't is_admin.
func requireAdmin(ctx context.Context, db *sqlx.DB) error {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("unauthorized"))
	}
	var u models.User
	found, err := u.FindUserByID(db, uid)
	if err != nil {
		return connect.NewError(connect.CodeInternal, err)
	}
	if found == nil || !found.IsAdmin {
		return connect.NewError(connect.CodePermissionDenied, fmt.Errorf("admin only"))
	}
	return nil
}

// writeAdminAction appends one audit row. Kept tiny so every branch
// above reads the same shape.
func writeAdminAction(ctx context.Context, tx *sqlx.Tx, actorID uint, action, subjectType string, subjectID int64, notes string) error {
	notesCol := sql.NullString{}
	if strings.TrimSpace(notes) != "" {
		notesCol = sql.NullString{String: strings.TrimSpace(notes), Valid: true}
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO public.admin_actions (actor_id, action, subject_type, subject_id, notes)
		 VALUES ($1, $2, $3, $4, $5)`,
		actorID, action, subjectType, subjectID, notesCol,
	)
	return err
}

// banUserInTx stamps deleted_at + banned_at + ban_reason + revokes
// refresh tokens + deletes push subs, all atomically.
func banUserInTx(ctx context.Context, tx *sqlx.Tx, userID uint, reason string) error {
	if _, err := tx.ExecContext(ctx,
		`UPDATE public.users
		    SET deleted_at = COALESCE(deleted_at, NOW()),
		        banned_at  = NOW(),
		        ban_reason = $1
		  WHERE id = $2`,
		reason, userID,
	); err != nil {
		return fmt.Errorf("stamp user: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM public.push_subscriptions WHERE user_id = $1`, userID,
	); err != nil {
		return fmt.Errorf("wipe push: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE public.refresh_tokens SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`,
		userID,
	); err != nil {
		return fmt.Errorf("revoke refresh: %w", err)
	}
	return nil
}

// removeReportedContent handles the remove_content branch. Each
// subject_type maps to a specific deletion operation; "user" has no
// content to remove (use ban for that), so it no-ops.
func removeReportedContent(ctx context.Context, tx *sqlx.Tx, subjectType string, subjectID int64) error {
	switch subjectType {
	case "matchup":
		// Reuse the existing admin cascade by issuing the same DELETEs
		// directly here — it's a small set and keeping it inline avoids
		// introducing a cross-tx helper.
		for _, q := range []string{
			`DELETE FROM likes WHERE matchup_id = $1`,
			`DELETE FROM comments WHERE matchup_id = $1`,
			`DELETE FROM matchup_votes WHERE matchup_public_id = (SELECT public_id FROM matchups WHERE id = $1)`,
			`DELETE FROM matchup_items WHERE matchup_id = $1`,
			`DELETE FROM matchups WHERE id = $1`,
		} {
			if _, err := tx.ExecContext(ctx, q, subjectID); err != nil {
				return fmt.Errorf("delete matchup %q: %w", q, err)
			}
		}
	case "bracket":
		for _, q := range []string{
			`DELETE FROM bracket_likes WHERE bracket_id = $1`,
			`DELETE FROM bracket_comments WHERE bracket_id = $1`,
			`UPDATE matchups SET bracket_id = NULL WHERE bracket_id = $1`,
			`DELETE FROM brackets WHERE id = $1`,
		} {
			if _, err := tx.ExecContext(ctx, q, subjectID); err != nil {
				return fmt.Errorf("delete bracket %q: %w", q, err)
			}
		}
	case "comment":
		if _, err := tx.ExecContext(ctx, `DELETE FROM comments WHERE id = $1`, subjectID); err != nil {
			return err
		}
	case "bracket_comment":
		if _, err := tx.ExecContext(ctx, `DELETE FROM bracket_comments WHERE id = $1`, subjectID); err != nil {
			return err
		}
	case "user":
		// No content here; admin should use ban_user if they want to
		// act on a user-profile report.
	}
	return nil
}

// resolveSubjectAuthorID returns the user id responsible for a reported
// subject. Needed by the ban_user branch of ResolveReport so it can
// target the right user given only (subject_type, subject_id).
func resolveSubjectAuthorID(ctx context.Context, tx *sqlx.Tx, subjectType string, subjectID int64) (uint, error) {
	var userID uint
	var query string
	// Matchups + brackets use `author_id`; comments + bracket_comments
	// use `user_id`. Don't unify — the schema was already set this way
	// in migration 001 and renaming would be a cross-cutting change.
	switch subjectType {
	case "matchup":
		query = `SELECT author_id FROM matchups WHERE id = $1`
	case "bracket":
		query = `SELECT author_id FROM brackets WHERE id = $1`
	case "comment":
		query = `SELECT user_id FROM comments WHERE id = $1`
	case "bracket_comment":
		query = `SELECT user_id FROM bracket_comments WHERE id = $1`
	case "user":
		return uint(subjectID), nil
	default:
		return 0, fmt.Errorf("unknown subject_type %q", subjectType)
	}
	if err := tx.GetContext(ctx, &userID, query, subjectID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	return userID, nil
}

// ---- Subject enrichment ----

// reportSubjectInfo is the UI-ready snapshot — a publicID the admin
// UI can deep-link to + a short preview string.
type reportSubjectInfo struct {
	publicID string
	preview  string
}

func reportSubjectKey(t string, id int64) string { return t + ":" + strconv.FormatInt(id, 10) }

// enrichReportSubjects runs one SELECT per subject_type that appears
// in the batch. Avoids per-row round-trips without getting fancy.
func enrichReportSubjects(ctx context.Context, db sqlx.ExtContext, ids map[string][]int64) map[string]reportSubjectInfo {
	out := make(map[string]reportSubjectInfo)
	for subjectType, rawIDs := range ids {
		if len(rawIDs) == 0 {
			continue
		}
		q := ""
		switch subjectType {
		case "matchup":
			q = `SELECT id, public_id AS pid, COALESCE(title, '') AS preview FROM matchups WHERE id = ANY($1)`
		case "bracket":
			q = `SELECT id, public_id AS pid, COALESCE(title, '') AS preview FROM brackets WHERE id = ANY($1)`
		case "comment":
			q = `SELECT id, COALESCE(public_id::text, '') AS pid, LEFT(body, 120) AS preview FROM comments WHERE id = ANY($1)`
		case "bracket_comment":
			q = `SELECT id, COALESCE(public_id::text, '') AS pid, LEFT(body, 120) AS preview FROM bracket_comments WHERE id = ANY($1)`
		case "user":
			q = `SELECT id, username AS pid, COALESCE(bio, '') AS preview FROM users WHERE id = ANY($1)`
		default:
			continue
		}
		rows, err := db.QueryxContext(ctx, q, int64ArrayParam(rawIDs))
		if err != nil {
			continue
		}
		for rows.Next() {
			var (
				id      int64
				pid     string
				preview string
			)
			if err := rows.Scan(&id, &pid, &preview); err == nil {
				out[reportSubjectKey(subjectType, id)] = reportSubjectInfo{
					publicID: pid,
					preview:  preview,
				}
			}
		}
		_ = rows.Close()
	}
	return out
}

// int64ArrayParam wraps the slice so ANY($1) sees a PostgreSQL array.
// The project uses the lib/pq driver, whose pq.Array adapter converts
// []int64 into the correct wire-format array for the ANY() clause.
func int64ArrayParam(xs []int64) interface{} {
	return pq.Array(xs)
}
