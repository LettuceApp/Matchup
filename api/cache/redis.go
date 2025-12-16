package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

// InitFromEnv initializes Redis using either:
// - REDIS_URL (Render internal Redis)
// - VALKEY_URL (for future hosted Valkey with TLS)
// - or local fallback
func InitFromEnv() error {
	redisURL := os.Getenv("REDIS_URL")
	valkeyURL := os.Getenv("VALKEY_URL") // optional future use

	switch {
	// Render Key-Value (internal, no TLS)
	case redisURL != "":
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return fmt.Errorf("failed to parse REDIS_URL: %w", err)
		}
		Client = redis.NewClient(opt)

	// Valkey Cloud (TLS required)
	case valkeyURL != "":
		opt, err := redis.ParseURL(valkeyURL)
		if err != nil {
			return fmt.Errorf("failed to parse VALKEY_URL: %w", err)
		}
		// Only enable TLS for rediss://
		Client = redis.NewClient(opt)

	// Local Dev Fallback (docker-compose, etc.)
	default:
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "localhost:6379"
		}

		Client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: os.Getenv("REDIS_PASSWORD"),
			Username: os.Getenv("REDIS_USERNAME"),
		})
	}

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis/valkey: %w", err)
	}

	return nil
}

// Basic helpers remain unchanged...

func Get(ctx context.Context, key string) (string, error) {
	if Client == nil {
		return "", fmt.Errorf("redis client not initialized")
	}

	val, err := Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil
	}
	return val, err
}

func Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if Client == nil {
		return fmt.Errorf("redis client not initialized")
	}
	return Client.Set(ctx, key, value, ttl).Err()
}

func DeleteByPrefix(ctx context.Context, prefix string) error {
	if Client == nil {
		return nil
	}

	var cursor uint64
	for {
		keys, next, err := Client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := Client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return nil
}
