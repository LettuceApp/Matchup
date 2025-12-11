package cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

// InitFromEnv initializes the global Redis client using env vars.
// This is where we plug in your Redis Cloud endpoint.
func InitFromEnv() error {
	// Render provides rediss:// URL (TLS required)
	valkeyURL := os.Getenv("VALKEY_URL")
	if valkeyURL != "" {
		opt, err := redis.ParseURL(valkeyURL)
		if err != nil {
			return fmt.Errorf("failed to parse VALKEY_URL: %w", err)
		}

		// MUST enable TLS
		opt.TLSConfig = &tls.Config{}

		Client = redis.NewClient(opt)
	} else {
		// Local dev fallback (Docker redis — NO TLS)
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "redis:6379"
		}

		Client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Username: os.Getenv("REDIS_USERNAME"),
			Password: os.Getenv("REDIS_PASSWORD"),
			DB:       0,
		})
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis/valkey: %w", err)
	}

	return nil
}

// Get returns the cached value for a key, or empty string if not found / error.
func Get(ctx context.Context, key string) (string, error) {
	if Client == nil {
		return "", fmt.Errorf("redis client not initialized")
	}

	val, err := Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // key not found
	}
	return val, err
}

// Set stores a value for a key with a TTL.
func Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if Client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	return Client.Set(ctx, key, value, ttl).Err()
}

// DeleteByPrefix deletes all keys that start with the given prefix.
// It is safe to call even if Redis is not configured (Client == nil).
func DeleteByPrefix(ctx context.Context, prefix string) error {
	if Client == nil {
		return nil
	}

	var cursor uint64
	for {
		keys, nextCursor, err := Client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return err
		}

		if len(keys) > 0 {
			if err := Client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}

		cursor = nextCursor
		if cursor == 0 {
			break
		}
	}

	return nil
}
