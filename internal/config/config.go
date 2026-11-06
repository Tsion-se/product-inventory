// Package config loads application configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds runtime configuration.
type Config struct {
	DBURL string // PostgreSQL connection string (required)
	Port  string // HTTP listen port
	Env   string // "development" or "production"
}

// Load reads configuration from the environment.
func Load() (Config, error) {
	cfg := Config{
		DBURL: strings.TrimSpace(os.Getenv("DB_URL")),
		Port:  getenv("PORT", "8080"),
		Env:   getenv("ENV", "development"),
	}
	if cfg.DBURL == "" {
		return Config{}, fmt.Errorf("DB_URL environment variable is required")
	}
	port, err := strconv.Atoi(cfg.Port)
	if err != nil || port < 1 || port > 65535 {
		return Config{}, fmt.Errorf("PORT must be an integer between 1 and 65535, got %q", cfg.Port)
	}
	return cfg, nil
}

// IsProduction reports whether the app runs in production mode.
func (c Config) IsProduction() bool {
	return strings.EqualFold(c.Env, "production")
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
