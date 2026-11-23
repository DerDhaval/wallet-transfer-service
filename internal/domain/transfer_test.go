package domain

import "testing"

func TestValidateCreateTransfer(t *testing.T) {
	valid := func() error {
		return ValidateCreateTransfer("abc123", "wallet_1", "wallet_2", 100)
	}
	if err := valid(); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}

	cases := []struct {
		name string
		key  string
		from string
		to   string
		amt  int64
		code string
	}{
		{name: "empty key", key: "", from: "wallet_1", to: "wallet_2", amt: 100, code: "INVALID_REQUEST"},
		{name: "bad key", key: "has space", from: "wallet_1", to: "wallet_2", amt: 100, code: "INVALID_REQUEST"},
		{name: "bad wallet", key: "k", from: "bad id", to: "wallet_2", amt: 100, code: "INVALID_REQUEST"},
		{name: "same wallet", key: "k", from: "wallet_1", to: "wallet_1", amt: 100, code: "SAME_WALLET"},
		{name: "zero", key: "k", from: "wallet_1", to: "wallet_2", amt: 0, code: "INVALID_AMOUNT"},
		{name: "negative", key: "k", from: "wallet_1", to: "wallet_2", amt: -5, code: "INVALID_AMOUNT"},
		{name: "too large", key: "k", from: "wallet_1", to: "wallet_2", amt: MaxTransferAmount + 1, code: "INVALID_AMOUNT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateCreateTransfer(tc.key, tc.from, tc.to, tc.amt)
			apiErr, ok := AsError(err)
			if !ok {
				t.Fatalf("got %v, want domain error", err)
			}
			if apiErr.Code != tc.code {
				t.Fatalf("code = %s, want %s", apiErr.Code, tc.code)
			}
		})
	}
}

func TestRequestHashStable(t *testing.T) {
	a := RequestHash("wallet_1", "wallet_2", 100)
	b := RequestHash("wallet_1", "wallet_2", 100)
	c := RequestHash("wallet_1", "wallet_2", 101)
	if a != b {
		t.Fatalf("hash changed for the same payload: %s vs %s", a, b)
	}
	if a == c {
		t.Fatal("hash ignored the amount")
	}
	if len(a) != 64 {
		t.Fatalf("hash length = %d", len(a))
	}
}

func TestOrderedWalletIDs(t *testing.T) {
	first, second := OrderedWalletIDs("wallet_b", "wallet_a")
	if first != "wallet_a" || second != "wallet_b" {
		t.Fatalf("order = %s, %s", first, second)
	}
	againFirst, againSecond := OrderedWalletIDs("wallet_a", "wallet_b")
	if againFirst != first || againSecond != second {
		t.Fatal("lock order depends on argument order")
	}
}

func TestValidTransition(t *testing.T) {
	if !ValidTransition(StatusPending, StatusProcessed) || !ValidTransition(StatusPending, StatusFailed) {
		t.Fatal("pending must be able to finish")
	}
	if ValidTransition(StatusProcessed, StatusFailed) || ValidTransition(StatusFailed, StatusProcessed) || ValidTransition(StatusProcessed, StatusPending) {
		t.Fatal("terminal status must stay terminal")
	}
}

func TestValidateCreateTransferDestinationID(t *testing.T) {
	err := ValidateCreateTransfer("k", "wallet_1", "bad id", 10)
	apiErr, ok := AsError(err)
	if !ok || apiErr.Code != "INVALID_REQUEST" {
		t.Fatalf("err = %v", err)
	}
}

func TestValidTransitionTable(t *testing.T) {
	cases := []struct {
		name string
		from TransferStatus
		to   TransferStatus
		want bool
	}{
		{name: "pending to processed", from: StatusPending, to: StatusProcessed, want: true},
		{name: "pending to failed", from: StatusPending, to: StatusFailed, want: true},
		{name: "processed to failed", from: StatusProcessed, to: StatusFailed, want: false},
		{name: "failed to processed", from: StatusFailed, to: StatusProcessed, want: false},
		{name: "processed to pending", from: StatusProcessed, to: StatusPending, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ValidTransition(tc.from, tc.to); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
