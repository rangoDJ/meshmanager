package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	ListenAddr  string
	DatabaseURL string
	DataDir     string

	// Optional first-run bootstrap; otherwise the web UI asks for an admin account.
	AdminUsername string
	AdminPassword string

	SessionTTL time.Duration
	// "auto" marks the session cookie Secure when the request arrived over HTTPS.
	CookieSecure string
	// Trust X-Forwarded-For / X-Forwarded-Proto. Only enable behind a reverse proxy.
	TrustProxy bool

	WorkerConcurrency int
	LogLevel          slog.Level
}

func Load() (Config, error) {
	c := Config{
		ListenAddr:    env("LISTEN_ADDR", ":8080"),
		DatabaseURL:   env("DATABASE_URL", "postgres://mesh:mesh@db:5432/mesh?sslmode=disable"),
		DataDir:       env("DATA_DIR", "/data"),
		AdminUsername: os.Getenv("ADMIN_USERNAME"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		CookieSecure:  strings.ToLower(env("COOKIE_SECURE", "auto")),
	}

	ttlDays, err := intEnv("SESSION_TTL_DAYS", 30)
	if err != nil {
		return c, err
	}
	c.SessionTTL = time.Duration(ttlDays) * 24 * time.Hour

	if c.WorkerConcurrency, err = intEnv("WORKER_CONCURRENCY", 4); err != nil {
		return c, err
	}
	if c.TrustProxy, err = boolEnv("TRUST_PROXY", false); err != nil {
		return c, err
	}
	if err := c.LogLevel.UnmarshalText([]byte(env("LOG_LEVEL", "info"))); err != nil {
		return c, fmt.Errorf("LOG_LEVEL: %w", err)
	}

	switch c.CookieSecure {
	case "auto", "true", "false":
	default:
		return c, fmt.Errorf("COOKIE_SECURE must be auto, true or false, got %q", c.CookieSecure)
	}
	if (c.AdminUsername == "") != (c.AdminPassword == "") {
		return c, fmt.Errorf("ADMIN_USERNAME and ADMIN_PASSWORD must be set together")
	}
	return c, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func intEnv(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer, got %q", key, v)
	}
	return n, nil
}

func boolEnv(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false, got %q", key, v)
	}
	return b, nil
}
