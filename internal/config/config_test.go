package config

import "testing"

func TestLoad(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		wantErr bool
		addr    string
	}{
		{
			name:    "missing database url",
			env:     map[string]string{},
			wantErr: true,
		},
		{
			name: "defaults",
			env:  map[string]string{"DATABASE_URL": "postgres://localhost/db"},
			addr: ":8080",
		},
		{
			name: "custom addr",
			env:  map[string]string{"DATABASE_URL": " postgres://localhost/db ", "HTTP_ADDR": ":9090"},
			addr: ":9090",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "")
			t.Setenv("HTTP_ADDR", "")
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
		})
	}
}
