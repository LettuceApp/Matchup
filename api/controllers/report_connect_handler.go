package controllers

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	reportv1 "Matchup/gen/report/v1"
	"Matchup/gen/report/v1/reportv1connect"
	"Matchup/middlewares"
	httpctx "Matchup/utils/httpctx"

	"connectrpc.com/connect"
	"github.com/jmoiron/sqlx"
)

// ReportHandler implements reportv1connect.ReportServiceHandler.
type ReportHandler struct{ DB *sqlx.DB }

var _ reportv1connect.ReportServiceHandler = (*ReportHandler)(nil)

// Report rate-limit: 10 per user per hour. A single disgruntled user
// can't flood the review queue faster than moderators can triage.
const (
	reportRateBucket = "report"
	reportRateBudget = 10
	reportRateWindow = time.Hour
)

// Closed vocabulary — mirrors the CHECK constraints in migration 023.
// Package-level sets so ReportContent + the admin-side filter can
// share validation without copy-paste.
var (
	validReportSubjectTypes = map[string]bool{
		"matchup":          true,
		"bracket":          true,
		"comment":          true,
		"bracket_comment":  true,
		"user":             true,
	}
	validReportReasons = map[string]bool{
		"harassment":     true,
		"spam":           true,
		"violence":       true,
		"nudity":         true,
		"misinformation": true,
		"self_harm":      true,
		"copyright":      true,
		"other":          true,
	}
)

// ReportContent drops a row into `reports` for admin triage. Resolves
// the subject's public UUID to the internal bigint before insert so
// the admin-side query + cascade doesn't have to re-JOIN on public_id.
func (h *ReportHandler) ReportContent(ctx context.Context, req *connect.Request[reportv1.ReportContentRequest]) (*connect.Response[reportv1.ReportContentResponse], error) {
	uid, ok := httpctx.CurrentUserID(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, errors.New("unauthorized"))
	}

	// Rate-limit before any DB work — a flooder hitting the same
	// subject repeatedly shouldn't cost us resolver lookups.
	if allowed, _ := middlewares.CheckUserRateLimit(ctx, uid, reportRateBucket, reportRateBudget, reportRateWindow); !allowed {
		return nil, connect.NewError(
			connect.CodeResourceExhausted,
			errors.New("you're reporting too fast; try again in a little while"),
		)
	}

	m := req.Msg
	subjectType := m.GetSubjectType()
	if !validReportSubjectTypes[subjectType] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported subject_type: %q", subjectType))
	}
	reason := m.GetReason()
	if !validReportReasons[reason] {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported reason: %q", reason))
	}
	// "other" MUST carry a reason_detail — otherwise the admin has
	// nothing to act on. Other reasons can omit it.
	reasonDetail := strings.TrimSpace(m.GetReasonDetail())
	if reason == "other" && reasonDetail == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("reason_detail is required when reason is \"other\""))
	}

	// Resolve subject public_id → internal id. Each kind has its own
	// resolver; a blanket helper would hide the "we don't know what
	// shape this row is" error.
	subjectInternalID, err := resolveReportSubjectInternalID(h.DB, subjectType, m.GetSubjectId())
	if err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}

	// Self-report guard: users can't report themselves. Minor abuse
	// prevention; primarily a sanity check for the "report this user"
	// profile flow.
	if subjectType == "user" && subjectInternalID == uid {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("you can't report your own account"))
	}

	var reasonDetailArg interface{}
	if reasonDetail != "" {
		reasonDetailArg = reasonDetail
	}

	var id int64
	if err := h.DB.QueryRowContext(ctx, `
		INSERT INTO public.reports
			(reporter_id, subject_type, subject_id, reason, reason_detail)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		uid, subjectType, subjectInternalID, reason, reasonDetailArg,
	).Scan(&id); err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}

	resp := connect.NewResponse(&reportv1.ReportContentResponse{Id: id})
	setReadPrimaryCookie(resp.Header())
	return resp, nil
}

// resolveReportSubjectInternalID routes to the right resolver based
// on subject_type and returns the internal bigint id. Kept package-
// private; the admin-side ListReports handler will JOIN directly on
// these subject_id values rather than resolving again.
func resolveReportSubjectInternalID(db sqlx.ExtContext, subjectType, publicID string) (uint, error) {
	switch subjectType {
	case "matchup":
		m, err := resolveMatchupByIdentifier(db, publicID)
		if err != nil {
			return 0, fmt.Errorf("matchup not found")
		}
		return m.ID, nil
	case "bracket":
		b, err := resolveBracketByIdentifier(db, publicID)
		if err != nil {
			return 0, fmt.Errorf("bracket not found")
		}
		return b.ID, nil
	case "comment":
		c, err := resolveCommentByIdentifier(db, publicID)
		if err != nil {
			return 0, fmt.Errorf("comment not found")
		}
		return c.ID, nil
	case "bracket_comment":
		c, err := resolveBracketCommentByIdentifier(db, publicID)
		if err != nil {
			return 0, fmt.Errorf("bracket comment not found")
		}
		return c.ID, nil
	case "user":
		u, err := resolveUserByIdentifier(db, publicID)
		if err != nil {
			return 0, fmt.Errorf("user not found")
		}
		return u.ID, nil
	default:
		return 0, fmt.Errorf("unsupported subject_type: %q", subjectType)
	}
}
