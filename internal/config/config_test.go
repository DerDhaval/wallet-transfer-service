package config

import (
	"log/slog"
	"testing"
)

func TestLoad(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
		addr    string
		level   slog.Level
	}{
		{
			name:    "missing database url",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name:  "defaults",
			env:   map[string]string{"DATABASE_URL": "postgres://localhost/db"},
			addr:  ":8080",
			level: slog.LevelInfo,
		},
		{
			name:  "custom addr and level",
			env:   map[string]string{"DATABASE_URL": " postgres://localhost/db ", "HTTP_ADDR": ":9090", "LOG_LEVEL": "DEBUG"},
			addr:  ":9090",
			level: slog.LevelDebug,
		},
		{
			name:    "bad log level",
			env:     map[string]string{"DATABASE_URL": "postgres://localhost/db", "LOG_LEVEL": "nope"},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("HTTP_ADDR", "")
			t.Setenv("LOG_LEVEL", "")
			for key, value := range tc.env {
				t.Setenv(key, value)
			}

			cfg, err := Load()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if cfg.DatabaseURL == "" {
				t.Fatal("database url empty")
			}
			if cfg.HTTPAddr != tc.addr {
				t.Fatalf("addr = %s, want %s", cfg.HTTPAddr, tc.addr)
			}
			if cfg.LogLevel != tc.level {
				t.Fatalf("level = %v, want %v", cfg.LogLevel, tc.level)
			}
		})
	}
}
