package controllers

import (
	"context"
	"net/http"

	httpctx "Matchup/utils/httpctx"

	"github.com/jmoiron/sqlx"
)

// dbForRead returns the primary DB when the request context has the
// read-primary flag set (i.e. a recent write happened), otherwise the
// replica. This is the single point where read-after-write routing
// decisions are made for handler code.
func dbForRead(ctx context.Context, primary, replica *sqlx.DB) *sqlx.DB {
	if httpctx.ShouldReadPrimary(ctx) {
		return primary
	}
	return replica
}

// setReadPrimaryCookie sets a short-lived cookie that tells the
// ReadAfterWriteMiddleware to route subsequent reads to the primary.
// Call this from any write handler before returning the response.
func setReadPrimaryCookie(h http.Header) {
	h.Add("Set-Cookie", "_rwp=1; Max-Age=5; Path=/; HttpOnly; SameSite=Strict")
}
