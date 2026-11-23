package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/handler"
	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/internal/router"
	"wallet-transfer-service/internal/service"
	"wallet-transfer-service/tests/testutil"
)

func TestMain(m *testing.M) {
	os.Exit(testutil.Run(m, "handler"))
}

func TestRejectsMalformedTransferRequests(t *testing.T) {
	api := router.New(handler.New(rejectingService{}, nil))
	cases := []struct {
		name        string
		body        string
		contentType string
		want        int
		code        string
	}{
		{name: "empty", body: "", contentType: "application/json", want: 400, code: "INVALID_REQUEST"},
		{name: "unknown field", body: `{"idempotencyKey":"k","fromWalletId":"wallet_1","toWalletId":"wallet_2","amount":1,"extra":true}`, contentType: "application/json", want: 400, code: "INVALID_REQUEST"},
		{name: "fractional amount", body: `{"idempotencyKey":"k","fromWalletId":"wallet_1","toWalletId":"wallet_2","amount":1.5}`, contentType: "application/json", want: 400, code: "INVALID_REQUEST"},
		{name: "text", body: `{}`, contentType: "text/plain", want: 415, code: "UNSUPPORTED_MEDIA_TYPE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/transfers", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			var payload struct {
				Error struct {
					Code string `json:"code"`
				} `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.Error.Code != tc.code {
				t.Fatalf("code = %s", payload.Error.Code)
			}
		})
	}
}

func TestTransferHTTPFlow(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	testutil.Clean(t, pool)
	svc := service.New(repository.New(pool))
	server := httptest.NewServer(router.New(handler.New(svc, func(context.Context) error { return nil })))
	t.Cleanup(server.Close)

	health := request(t, server, http.MethodGet, "/health", nil)
	if health.StatusCode != 200 {
		t.Fatalf("health = %d", health.StatusCode)
	}

	createWallet(t, server, "wallet_1", "INR", 50_000)
	createWallet(t, server, "wallet_2", "INR", 0)

	same := request(t, server, http.MethodPost, "/transfers", map[string]any{
		"idempotencyKey": "bad", "fromWalletId": "wallet_1", "toWalletId": "wallet_1", "amount": 10,
	})
	requireStatus(t, same, 400, "SAME_WALLET")

	missing := request(t, server, http.MethodPost, "/transfers", map[string]any{
		"idempotencyKey": "missing", "fromWalletId": "wallet_1", "toWalletId": "wallet_9", "amount": 10,
	})
	requireStatus(t, missing, 404, "WALLET_NOT_FOUND")

	first := request(t, server, http.MethodPost, "/transfers", map[string]any{
		"idempotencyKey": "abc123", "fromWalletId": "wallet_1", "toWalletId": "wallet_2", "amount": 100,
	})
	if first.StatusCode != 201 {
		t.Fatalf("create = %d %s", first.StatusCode, first.Body)
	}
	if first.Header.Get("X-Idempotent-Replay") != "" {
		t.Fatal("first response was marked as a replay")
	}
	var created struct {
		TransferID string `json:"transferId"`
		Status     string `json:"status"`
		Amount     int64  `json:"amount"`
	}
	decode(t, first.Body, &created)
	if created.Status != "PROCESSED" || created.Amount != 100 || created.TransferID == "" {
		t.Fatalf("created = %+v", created)
	}

	replay := request(t, server, http.MethodPost, "/transfers", map[string]any{
		"idempotencyKey": "abc123", "fromWalletId": "wallet_1", "toWalletId": "wallet_2", "amount": 100,
	})
	if replay.StatusCode != 201 || replay.Header.Get("X-Idempotent-Replay") != "true" || !bytes.Equal(first.Body, replay.Body) {
		t.Fatalf("replay = %d header %q body %s", replay.StatusCode, replay.Header.Get("X-Idempotent-Replay"), replay.Body)
	}

	conflict := request(t, server, http.MethodPost, "/transfers", map[string]any{
		"idempotencyKey": "abc123", "fromWalletId": "wallet_1", "toWalletId": "wallet_2", "amount": 90,
	})
	requireStatus(t, conflict, 409, "IDEMPOTENCY_CONFLICT")

	poor := request(t, server, http.MethodPost, "/transfers", map[string]any{
		"idempotencyKey": "too-much", "fromWalletId": "wallet_2", "toWalletId": "wallet_1", "amount": 101,
	})
	requireStatus(t, poor, 422, "INSUFFICIENT_FUNDS")

	got := request(t, server, http.MethodGet, "/transfers/"+created.TransferID, nil)
	if got.StatusCode != 200 || !bytes.Contains(got.Body, []byte(created.TransferID)) {
		t.Fatalf("get transfer = %d %s", got.StatusCode, got.Body)
	}
	balance := request(t, server, http.MethodGet, "/wallets/wallet_1/balance", nil)
	if balance.StatusCode != 200 || !bytes.Contains(balance.Body, []byte(`"balance":49900`)) {
		t.Fatalf("balance = %d %s", balance.StatusCode, balance.Body)
	}
	history := request(t, server, http.MethodGet, "/wallets/wallet_1/transactions", nil)
	if history.StatusCode != 200 || !bytes.Contains(history.Body, []byte(`"entryType":"DEBIT"`)) {
		t.Fatalf("history = %d %s", history.StatusCode, history.Body)
	}
	missingTransfer := request(t, server, http.MethodGet, "/transfers/trf_missing", nil)
	requireStatus(t, missingTransfer, 404, "TRANSFER_NOT_FOUND")
}

type httpResult struct {
	StatusCode int
	Header     http.Header
	Body       []byte
}

func createWallet(t *testing.T, server *httptest.Server, id, currency string, balance int64) {
	t.Helper()
	res := request(t, server, http.MethodPost, "/wallets", map[string]any{
		"id": id, "currency": currency, "initialBalance": balance,
	})
	if res.StatusCode != 201 {
		t.Fatalf("create wallet %s = %d %s", id, res.StatusCode, res.Body)
	}
}

func request(t *testing.T, server *httptest.Server, method, path string, body any) httpResult {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequest(method, server.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	payload, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return httpResult{StatusCode: res.StatusCode, Header: res.Header, Body: payload}
}

func requireStatus(t *testing.T, res httpResult, status int, code string) {
	t.Helper()
	if res.StatusCode != status {
		t.Fatalf("status = %d body = %s, want %d", res.StatusCode, res.Body, status)
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(res.Body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != code {
		t.Fatalf("code = %s, want %s", payload.Error.Code, code)
	}
}

func decode(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}

type rejectingService struct{}

func (rejectingService) CreateTransfer(context.Context, service.CreateTransferCommand) (service.Result, error) {
	return service.Result{}, domain.ErrInternal.WithMessage("service should not be called")
}
func (rejectingService) GetTransfer(context.Context, string) (domain.Transfer, error) {
	return domain.Transfer{}, domain.ErrInternal
}
func (rejectingService) CreateWallet(context.Context, service.CreateWalletCommand) (domain.Wallet, error) {
	return domain.Wallet{}, domain.ErrInternal
}
func (rejectingService) GetWallet(context.Context, string) (domain.Wallet, error) {
	return domain.Wallet{}, domain.ErrInternal
}
func (rejectingService) ListWalletEntries(context.Context, string, int, int) ([]domain.LedgerEntry, error) {
	return nil, domain.ErrInternal
}
