package service_test

import (
	"context"
	"testing"

	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/internal/service"
	"wallet-transfer-service/tests/testutil"
)

func TestCreateTransferValidationTable(t *testing.T) {
	pool := testutil.RequirePostgres(t)
	testutil.Clean(t, pool)
	svc := service.New(repository.New(pool))
	createWallet(t, svc, "tv_a", "INR", 100)
	createWallet(t, svc, "tv_b", "INR", 0)
	createWallet(t, svc, "tv_usd", "USD", 50)

	cases := []struct {
		name string
		cmd  service.CreateTransferCommand
		code string
		http int
	}{
		{
			name: "ok",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-ok", FromWalletID: "tv_a", ToWalletID: "tv_b", Amount: 7},
			http: 201,
		},
		{
			name: "same wallet",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-same", FromWalletID: "tv_a", ToWalletID: "tv_a", Amount: 1},
			code: "SAME_WALLET",
		},
		{
			name: "zero amount",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-zero", FromWalletID: "tv_a", ToWalletID: "tv_b", Amount: 0},
			code: "INVALID_AMOUNT",
		},
		{
			name: "missing source",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-ms", FromWalletID: "missing", ToWalletID: "tv_b", Amount: 1},
			code: "WALLET_NOT_FOUND",
		},
		{
			name: "missing destination",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-md", FromWalletID: "tv_a", ToWalletID: "missing", Amount: 1},
			code: "WALLET_NOT_FOUND",
		},
		{
			name: "currency mismatch",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-fx", FromWalletID: "tv_a", ToWalletID: "tv_usd", Amount: 1},
			http: 422,
		},
		{
			name: "insufficient funds",
			cmd:  service.CreateTransferCommand{IdempotencyKey: "tv-poor", FromWalletID: "tv_b", ToWalletID: "tv_a", Amount: 50},
			http: 422,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := svc.CreateTransfer(context.Background(), tc.cmd)
			if tc.code != "" {
				requireCode(t, err, tc.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if result.StatusCode != tc.http {
				t.Fatalf("status = %d body = %s", result.StatusCode, result.Body)
			}
		})
	}
}
