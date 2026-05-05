package middlewares

import (
	"fmt"
	"net/http"

	httpctx "Matchup/utils/httpctx"

	sentryhttp "github.com/getsentry/sentry-go/http"
	gosentry "github.com/getsentry/sentry-go"
)

// SentryMiddleware wraps each request in a Sentry hub so captures
// inside the handler carry the request's tags + user context, AND
// recovers panics into a captureException + 500 response. It
// replaces chi's `middleware.Recoverer` in the middleware chain (we
// don't want panics to be double-reported).
//
// The official sentry-go/http handler does the plumbing; we thread
// it through a wrapper so we can attach per-request user tags (the
// viewer's ID from httpctx) before the hub fires.
func SentryMiddleware() func(http.Handler) http.Handler {
	// Repanic:true lets chi's own recovery chain still render a 500
	// response after Sentry has captured the event — without this
	// the connection closes abruptly.
	sh := sentryhttp.New(sentryhttp.Options{Repanic: true})

	return func(next http.Handler) http.Handler {
		wrapped := sh.Handle(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if hub := gosentry.GetHubFromContext(r.Context()); hub != nil {
				hub.Scope().SetTag("path", r.URL.Path)
				hub.Scope().SetTag("method", r.Method)
				if uid, ok := httpctx.CurrentUserID(r.Context()); ok {
					hub.Scope().SetUser(gosentry.User{
						ID: fmt.Sprintf("%d", uid),
					})
				}
			}
			next.ServeHTTP(w, r)
		}))
		return wrapped
	}
}
