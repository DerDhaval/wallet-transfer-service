package service_test

import (
	"context"
	"testing"

	"wallet-transfer-service/internal/domain"
	"wallet-transfer-service/internal/repository"
	"wallet-transfer-service/internal/service"
	"wallet-transfer-service/tests/testutil"
)

func TestCreateWalletTable(t *testing.T) {
	pool := testutil.FreshPool(t)
	svc := service.New(repository.New(pool))

	cases := []struct {
		name    string
		cmd     service.CreateWalletCommand
		code    string
		checkID bool
	}{
		{name: "explicit", cmd: service.CreateWalletCommand{ID: "svc_w1", Currency: "INR", InitialBalance: 25}},
		{name: "generated", cmd: service.CreateWalletCommand{Currency: "USD", InitialBalance: 0}, checkID: true},
		{name: "duplicate", cmd: service.CreateWalletCommand{ID: "svc_w1", Currency: "INR", InitialBalance: 1}, code: "WALLET_EXISTS"},
		{name: "bad currency", cmd: service.CreateWalletCommand{ID: "svc_w2", Currency: "inr", InitialBalance: 1}, code: "INVALID_REQUEST"},
		{name: "negative", cmd: service.CreateWalletCommand{ID: "svc_w3", Currency: "INR", InitialBalance: -1}, code: "INVALID_AMOUNT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wallet, err := svc.CreateWallet(context.Background(), tc.cmd)
			if tc.code != "" {
				testutil.RequireCode(t, err, tc.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.checkID && wallet.ID == "" {
				t.Fatal("expected generated id")
			}
			if !tc.checkID && wallet.ID != tc.cmd.ID {
				t.Fatalf("id = %s", wallet.ID)
			}
			if wallet.Balance != tc.cmd.InitialBalance || wallet.Currency != tc.cmd.Currency {
				t.Fatalf("wallet = %+v", wallet)
			}
		})
	}
}

func TestGetWalletAndListEntriesTable(t *testing.T) {
	pool := testutil.FreshPool(t)
	svc := service.New(repository.New(pool))
	if _, err := svc.CreateWallet(context.Background(), service.CreateWalletCommand{ID: "svc_a", Currency: "INR", InitialBalance: 50}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateWallet(context.Background(), service.CreateWalletCommand{ID: "svc_b", Currency: "INR", InitialBalance: 0}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "svc-list", FromWalletID: "svc_a", ToWalletID: "svc_b", Amount: 5,
	}); err != nil {
		t.Fatal(err)
	}

	getCases := []struct {
		name string
		id   string
		code string
	}{
		{name: "ok", id: "svc_a"},
		{name: "bad id", id: "bad id", code: "INVALID_REQUEST"},
		{name: "missing", id: "missing", code: "WALLET_NOT_FOUND"},
	}
	for _, tc := range getCases {
		t.Run("get/"+tc.name, func(t *testing.T) {
			wallet, err := svc.GetWallet(context.Background(), tc.id)
			if tc.code != "" {
				testutil.RequireCode(t, err, tc.code)
				return
			}
			if err != nil || wallet.ID != tc.id {
				t.Fatalf("wallet=%+v err=%v", wallet, err)
			}
		})
	}

	listCases := []struct {
		name   string
		id     string
		limit  int
		offset int
		code   string
		count  int
	}{
		{name: "default page", id: "svc_a", count: 1},
		{name: "explicit page", id: "svc_a", limit: 10, offset: 0, count: 1},
		{name: "empty page", id: "svc_a", limit: 10, offset: 50, count: 0},
		{name: "bad limit", id: "svc_a", limit: 101, code: "INVALID_REQUEST"},
		{name: "negative offset", id: "svc_a", limit: 10, offset: -1, code: "INVALID_REQUEST"},
		{name: "bad wallet id", id: "bad id", code: "INVALID_REQUEST"},
		{name: "missing wallet", id: "missing", limit: 10, code: "WALLET_NOT_FOUND"},
	}
	for _, tc := range listCases {
		t.Run("list/"+tc.name, func(t *testing.T) {
			entries, err := svc.ListWalletEntries(context.Background(), tc.id, tc.limit, tc.offset)
			if tc.code != "" {
				testutil.RequireCode(t, err, tc.code)
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != tc.count {
				t.Fatalf("count = %d, want %d", len(entries), tc.count)
			}
		})
	}
}

func TestGetTransferTable(t *testing.T) {
	pool := testutil.FreshPool(t)
	svc := service.New(repository.New(pool))
	createWallet(t, svc, "gt_a", "INR", 20)
	createWallet(t, svc, "gt_b", "INR", 0)
	result, err := svc.CreateTransfer(context.Background(), service.CreateTransferCommand{
		IdempotencyKey: "gt-1", FromWalletID: "gt_a", ToWalletID: "gt_b", Amount: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	transferID := testutil.TransferIDFromBody(t, result.Body)

	cases := []struct {
		name string
		id   string
		code string
	}{
		{name: "ok", id: transferID},
		{name: "bad id", id: "bad id", code: "INVALID_REQUEST"},
		{name: "missing", id: "trf_missing", code: "TRANSFER_NOT_FOUND"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := svc.GetTransfer(context.Background(), tc.id)
			if tc.code != "" {
				testutil.RequireCode(t, err, tc.code)
				return
			}
			if err != nil || got.ID != tc.id || got.Status != domain.StatusProcessed {
				t.Fatalf("got=%+v err=%v", got, err)
			}
		})
	}
}
