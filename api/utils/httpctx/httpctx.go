package httpctx

import "context"

type contextKey int

const (
	userIDKey      contextKey = iota
	isAdminKey     contextKey = iota
	readPrimaryKey contextKey = iota
)

// WithUserID stores the authenticated user ID in the context.
func WithUserID(ctx context.Context, id uint) context.Context {
	return context.WithValue(ctx, userIDKey, id)
}

// CurrentUserID retrieves the authenticated user ID from context.
func CurrentUserID(ctx context.Context) (uint, bool) {
	val := ctx.Value(userIDKey)
	if val == nil {
		return 0, false
	}
	uid, ok := val.(uint)
	return uid, ok
}

// WithIsAdmin stores the admin flag in the context.
func WithIsAdmin(ctx context.Context, isAdmin bool) context.Context {
	return context.WithValue(ctx, isAdminKey, isAdmin)
}

// IsAdminRequest reports whether the current request is from an admin.
func IsAdminRequest(ctx context.Context) bool {
	val := ctx.Value(isAdminKey)
	if val == nil {
		return false
	}
	isAdmin, ok := val.(bool)
	return ok && isAdmin
}

// WithReadPrimary marks the context so that read queries should be routed
// to the primary database instead of the replica.
func WithReadPrimary(ctx context.Context) context.Context {
	return context.WithValue(ctx, readPrimaryKey, true)
}

// ShouldReadPrimary reports whether reads should be routed to the primary.
func ShouldReadPrimary(ctx context.Context) bool {
	val, _ := ctx.Value(readPrimaryKey).(bool)
	return val
}
