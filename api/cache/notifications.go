package cache

import (
	"context"
	"fmt"
)

// ActivityChannel returns the Redis pub/sub channel for a user's
// activity feed push notifications. Centralised so the SSE handler
// (subscriber) and every write-path publish hook use the same string.
func ActivityChannel(userID uint) string {
	return fmt.Sprintf("user:%d:activity", userID)
}

// PublishActivity fires a pub/sub ping on the given user's activity
// channel. The body is deliberately just "ping" — clients refetch the
// full list on any event, matching the derived-feed read model.
// Safe to call when Redis is unavailable; returns nil silently to let
// the caller stay on the happy path.
func PublishActivity(ctx context.Context, userID uint) error {
	if Client == nil {
		return nil
	}
	if userID == 0 {
		return nil
	}
	return Client.Publish(ctx, ActivityChannel(userID), "ping").Err()
}
