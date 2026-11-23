package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
	LogLevel    slog.Level
}

func Load() (Config, error) {
	cfg := Config{
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddr:    strings.TrimSpace(os.Getenv("HTTP_ADDR")),
		LogLevel:    slog.LevelInfo,
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	if raw := strings.TrimSpace(os.Getenv("LOG_LEVEL")); raw != "" {
		var level slog.Level
		if err := level.UnmarshalText([]byte(strings.ToLower(raw))); err != nil {
			return Config{}, fmt.Errorf("LOG_LEVEL is invalid: %w", err)
		}
		cfg.LogLevel = level
	}
	return cfg, nil
}
