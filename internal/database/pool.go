// Package database provides PostgreSQL connectivity.
package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	connectAttempts = 10
	connectDelay    = 2 * time.Second
)

// NewPool creates a pgx connection pool and verifies connectivity, retrying
// briefly so the app tolerates a database that is still starting up.
func NewPool(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse DB_URL: %w", err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create connection pool: %w", err)
	}

	var lastErr error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		lastErr = pool.Ping(pingCtx)
		cancel()
		if lastErr == nil {
			return pool, nil
		}
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(connectDelay):
		}
	}
	pool.Close()
	return nil, fmt.Errorf("connect to database after %d attempts: %w", connectAttempts, lastErr)
}
