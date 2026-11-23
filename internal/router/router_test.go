package router_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/handler"
	"wallet-transfer-service/internal/router"
	"wallet-transfer-service/internal/service"
)

func TestRouterRoutes(t *testing.T) {
	api := router.New(handler.New(stubService{}, nil))
	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{name: "health", method: http.MethodGet, path: "/health", want: http.StatusOK},
		{name: "missing transfer", method: http.MethodGet, path: "/transfers/trf_x", want: http.StatusInternalServerError},
		{name: "unknown", method: http.MethodGet, path: "/nope", want: http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d body = %s", rec.Code, rec.Body.String())
			}
			if tc.path != "/nope" && rec.Header().Get("X-Request-Id") == "" {
				t.Fatal("middleware did not set request id")
			}
		})
	}
}

type stubService struct{}

func (stubService) CreateTransfer(context.Context, service.CreateTransferCommand) (service.Result, error) {
	return service.Result{}, domain.ErrInternal
}
func (stubService) GetTransfer(context.Context, string) (domain.Transfer, error) {
	return domain.Transfer{}, domain.ErrInternal
}
func (stubService) CreateWallet(context.Context, service.CreateWalletCommand) (domain.Wallet, error) {
	return domain.Wallet{}, domain.ErrInternal
}
func (stubService) GetWallet(context.Context, string) (domain.Wallet, error) {
	return domain.Wallet{}, domain.ErrInternal
}
func (stubService) ListWalletEntries(context.Context, string, int, int) ([]domain.LedgerEntry, error) {
	return nil, domain.ErrInternal
}
