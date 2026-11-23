package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	DatabaseURL string
	HTTPAddr    string
}

func Load() (Config, error) {
	// Load .env if present. Existing process env wins (not overwritten).
	_ = godotenv.Load()

	cfg := Config{
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		HTTPAddr:    strings.TrimSpace(os.Getenv("HTTP_ADDR")),
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required (set it in .env or the environment)")
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":8080"
	}
	return cfg, nil
}
