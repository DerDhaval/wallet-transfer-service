package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"wallet-transfer-service/internal/middleware"
)

func TestRequestID(t *testing.T) {
	cases := []struct {
		name   string
		header string
	}{
		{name: "generated"},
		{name: "propagated", header: "req_custom"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen = middleware.RequestIDFrom(r.Context())
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			if tc.header != "" {
				req.Header.Set("X-Request-Id", tc.header)
			}
			rec := httptest.NewRecorder()
			middleware.RequestID(next).ServeHTTP(rec, req)

			got := rec.Header().Get("X-Request-Id")
			if got == "" {
				t.Fatal("missing response header")
			}
			if tc.header != "" && got != tc.header {
				t.Fatalf("header = %s, want %s", got, tc.header)
			}
			if seen != got {
				t.Fatalf("context id = %s, header = %s", seen, got)
			}
			if middleware.RequestIDFrom(req.Context()) != "" {
				t.Fatal("original request context should stay empty")
			}
		})
	}
}

func TestAccessLogAndStatusRecorder(t *testing.T) {
	cases := []struct {
		name   string
		status int
	}{
		{name: "explicit status", status: http.StatusCreated},
		{name: "default ok", status: http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.status != http.StatusOK {
					w.WriteHeader(tc.status)
				}
				_, _ = w.Write([]byte("ok"))
			})
			req := httptest.NewRequest(http.MethodGet, "/x", nil)
			rec := httptest.NewRecorder()
			middleware.RequestID(middleware.AccessLog(next)).ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}
