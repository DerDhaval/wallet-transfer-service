package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/handler"
	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/internal/router"
	"wallet-transfer-service/internal/service"
	"wallet-transfer-service/tests/testutil"
)

func TestHandlerTableDriven(t *testing.T) {
	pool := testutil.FreshPool(t)
	svc := service.New(repository.New(pool))
	api := router.New(handler.New(svc, pool.Ping))

	createWalletHTTP(t, api, "h_w1", "INR", 1_000)
	createWalletHTTP(t, api, "h_w2", "INR", 0)

	cases := []struct {
		name        string
		method      string
		path        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
		wantSubstr  string
	}{
		{
			name: "health ok", method: http.MethodGet, path: "/health",
			wantStatus: 200, wantSubstr: `"status":"ok"`,
		},
		{
			name: "create wallet missing currency", method: http.MethodPost, path: "/wallets",
			body: `{"id":"h_w3","initialBalance":1}`, contentType: "application/json",
			wantStatus: 400, wantCode: "INVALID_REQUEST",
		},
		{
			name: "create wallet duplicate", method: http.MethodPost, path: "/wallets",
			body: `{"id":"h_w1","currency":"INR","initialBalance":1}`, contentType: "application/json",
			wantStatus: 409, wantCode: "WALLET_EXISTS",
		},
		{
			name: "balance ok", method: http.MethodGet, path: "/wallets/h_w1/balance",
			wantStatus: 200, wantSubstr: `"balance":1000`,
		},
		{
			name: "balance missing", method: http.MethodGet, path: "/wallets/missing/balance",
			wantStatus: 404, wantCode: "WALLET_NOT_FOUND",
		},
		{
			name: "transactions bad limit", method: http.MethodGet, path: "/wallets/h_w1/transactions?limit=abc",
			wantStatus: 400, wantCode: "INVALID_REQUEST",
		},
		{
			name: "transactions bad offset", method: http.MethodGet, path: "/wallets/h_w1/transactions?offset=x",
			wantStatus: 400, wantCode: "INVALID_REQUEST",
		},
		{
			name: "transactions ok", method: http.MethodGet, path: "/wallets/h_w1/transactions?limit=10&offset=0",
			wantStatus: 200, wantSubstr: `"transactions"`,
		},
		{
			name: "transfer body too large ignored by tiny body", method: http.MethodPost, path: "/transfers",
			body: `{"idempotencyKey":"h1","fromWalletId":"h_w1","toWalletId":"h_w2","amount":10}`, contentType: "application/json",
			wantStatus: 201, wantSubstr: `"status":"PROCESSED"`,
		},
		{
			name: "transfer replay", method: http.MethodPost, path: "/transfers",
			body: `{"idempotencyKey":"h1","fromWalletId":"h_w1","toWalletId":"h_w2","amount":10}`, contentType: "application/json",
			wantStatus: 201, wantSubstr: `"status":"PROCESSED"`,
		},
		{
			name: "transfer conflict", method: http.MethodPost, path: "/transfers",
			body: `{"idempotencyKey":"h1","fromWalletId":"h_w1","toWalletId":"h_w2","amount":11}`, contentType: "application/json",
			wantStatus: 409, wantCode: "IDEMPOTENCY_CONFLICT",
		},
		{
			name: "transfer insufficient", method: http.MethodPost, path: "/transfers",
			body: `{"idempotencyKey":"h2","fromWalletId":"h_w2","toWalletId":"h_w1","amount":9999}`, contentType: "application/json",
			wantStatus: 422, wantCode: "INSUFFICIENT_FUNDS",
		},
		{
			name: "transfer same wallet", method: http.MethodPost, path: "/transfers",
			body: `{"idempotencyKey":"h3","fromWalletId":"h_w1","toWalletId":"h_w1","amount":1}`, contentType: "application/json",
			wantStatus: 400, wantCode: "SAME_WALLET",
		},
		{
			name: "transfer empty body", method: http.MethodPost, path: "/transfers",
			body: "", contentType: "application/json",
			wantStatus: 400, wantCode: "INVALID_REQUEST",
		},
		{
			name: "transfer trailing json", method: http.MethodPost, path: "/transfers",
			body: `{"idempotencyKey":"h4","fromWalletId":"h_w1","toWalletId":"h_w2","amount":1}{}`, contentType: "application/json",
			wantStatus: 400, wantCode: "INVALID_REQUEST",
		},
		{
			name: "get missing transfer", method: http.MethodGet, path: "/transfers/trf_missing",
			wantStatus: 404, wantCode: "TRANSFER_NOT_FOUND",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var reader *bytes.Reader
			if tc.body != "" || tc.method == http.MethodPost {
				reader = bytes.NewReader([]byte(tc.body))
			}
			var req *http.Request
			if reader != nil {
				req = httptest.NewRequest(tc.method, tc.path, reader)
			} else {
				req = httptest.NewRequest(tc.method, tc.path, nil)
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if tc.wantCode != "" {
				var payload struct {
					Error struct {
						Code string `json:"code"`
					} `json:"error"`
				}
				if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Error.Code != tc.wantCode {
					t.Fatalf("code = %s, want %s body = %s", payload.Error.Code, tc.wantCode, rec.Body.String())
				}
			}
			if tc.wantSubstr != "" && !strings.Contains(rec.Body.String(), tc.wantSubstr) {
				t.Fatalf("body %s missing %s", rec.Body.String(), tc.wantSubstr)
			}
		})
	}
}

func TestHealthUnavailable(t *testing.T) {
	api := router.New(handler.New(rejectingService{}, func(context.Context) error {
		return context.DeadlineExceeded
	}))
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestWriteErrorInternalFallback(t *testing.T) {
	api := router.New(handler.New(rejectingService{}, nil))
	req := httptest.NewRequest(http.MethodGet, "/transfers/trf_x", nil)
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "INTERNAL_ERROR") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func createWalletHTTP(t *testing.T, api http.Handler, id, currency string, balance int64) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"id": id, "currency": currency, "initialBalance": balance})
	req := httptest.NewRequest(http.MethodPost, "/wallets", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create wallet = %d %s", rec.Code, rec.Body.String())
	}
}

// Ensure domain import stays for clarity in failure assertions above.
var _ = domain.ErrInternal
