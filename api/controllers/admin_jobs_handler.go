package controllers

// admin_jobs_handler.go exposes manually-triggerable admin
// maintenance jobs via plain chi routes (not Connect-RPC). Used
// when the hosting environment doesn't provide shell access — e.g.
// Render's free tier — so an admin can still kick off recovery
// work by curling the endpoint with their bearer token.
//
// Each handler is wrapped at the route layer by middlewares.TokenAuthMiddleware
// which extracts the JWT, validates the user, and stuffs userID +
// isAdmin into the request context. The per-handler admin gate
// below then refuses anyone who isn't actually flagged is_admin.

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	httpctx "Matchup/utils/httpctx"

	"github.com/jmoiron/sqlx"
)

// adminJobResponse is the small JSON envelope every admin job
// returns. `Job` echoes back the action that ran (so the response
// is self-describing in logs / dev tools); `RowsAffected` reports
// whatever work the job actually performed; `OK` is the boolean
// shorthand a curl-from-a-shell user can grep for.
type adminJobResponse struct {
	OK           bool   `json:"ok"`
	Job          string `json:"job"`
	RowsAffected int64  `json:"rows_affected"`
}

// ReconcileVoteCountsHandler runs the same matchup_items.votes
// reconciliation as the nightly cron job, on demand. Returns the
// number of rows it had to repair so the caller can sanity-check
// the result.
//
// curl example (run while signed in as the admin user):
//
//	curl -X POST https://<host>/admin/jobs/reconcile-vote-counts \
//	     -H "Authorization: Bearer $TOKEN"
//
// Or from a browser DevTools console with the admin signed in:
//
//	fetch('/admin/jobs/reconcile-vote-counts', {
//	  method: 'POST',
//	  headers: { Authorization: 'Bearer ' + localStorage.token },
//	}).then(r => r.json()).then(console.log)
//
// Non-admin callers receive 403 with `{"error":"admin only"}`.
// Anonymous callers are short-circuited at the TokenAuthMiddleware
// layer with 401 before reaching this function.
func ReconcileVoteCountsHandler(db *sqlx.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Admin gate. TokenAuthMiddleware already validated that the
		// caller has a real JWT — we just confirm they're flagged
		// is_admin. Non-admins get a plain 403 instead of a hint that
		// the endpoint exists.
		if !httpctx.IsAdminRequest(r.Context()) {
			http.Error(w, `{"error":"admin only"}`, http.StatusForbidden)
			return
		}

		// 10-minute upper bound matches the nightly cron's budget —
		// the reconciliation is a single UPDATE per matchup_items
		// row touched, and even a million-row table fits well under
		// that bound. Cap exists mainly so a runaway query can't pin
		// a request goroutine indefinitely.
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
		defer cancel()

		n, err := ReconcileVoteCounts(ctx, db)
		if err != nil {
			log.Printf("admin/jobs/reconcile-vote-counts: %v", err)
			http.Error(w, `{"error":"internal error"}`, http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(adminJobResponse{
			OK:           true,
			Job:          "reconcile_vote_counts",
			RowsAffected: n,
		})
	}
}
