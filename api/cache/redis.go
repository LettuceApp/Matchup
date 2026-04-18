package cache

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

// VoteDeltaKeyPrefix is the Redis key prefix for buffered vote-count
// deltas. The producer (controllers.VoteItem) writes keys as
// "votes:item:<id>"; the consumer (handlers.RunFlushVotes) SCANs them.
// Defined here so both packages import a single source of truth without
// either one depending on the other.
const VoteDeltaKeyPrefix = "votes:item:"

// InitFromEnv initializes Redis for Render, Valkey Cloud, or local dev.
func InitFromEnv() error {

	// Render internal Key-Value DB
	if redisURL := os.Getenv("REDIS_URL"); redisURL != "" {
		opt, err := redis.ParseURL(redisURL)
		if err != nil {
			return fmt.Errorf("failed to parse REDIS_URL: %w", err)
		}
		opt.PoolSize = 10
		opt.MinIdleConns = 2

		// DO NOT ENABLE TLS — internal Redis uses redis:// (no TLS)
		Client = redis.NewClient(opt)
	}

	// Optional: Valkey Cloud external endpoint (TLS)
	if Client == nil {
		if valkeyURL := os.Getenv("VALKEY_URL"); valkeyURL != "" {
			opt, err := redis.ParseURL(valkeyURL)
			if err != nil {
				return fmt.Errorf("failed to parse VALKEY_URL: %w", err)
			}
			opt.PoolSize = 10
			opt.MinIdleConns = 2

			// TLS is only used for rediss:// URLs
			Client = redis.NewClient(opt)
		}
	}

	// Local fallback for dev (docker-compose, local Redis)
	if Client == nil {
		addr := os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = "localhost:6379"
		}

		Client = redis.NewClient(&redis.Options{
			Addr:         addr,
			Username:     os.Getenv("REDIS_USERNAME"),
			Password:     os.Getenv("REDIS_PASSWORD"),
			PoolSize:     10,
			MinIdleConns: 2,
		})
	}

	// Test Redis connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := Client.Ping(ctx).Err(); err != nil {
		return fmt.Errorf("failed to connect to redis: %w", err)
	}

	return nil
}

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

func Delete(ctx context.Context, key string) error {
	if Client == nil {
		return nil
	}
	return Client.Del(ctx, key).Err()
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
