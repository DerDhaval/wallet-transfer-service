package domain

import "testing"

func TestValidateNewWalletTable(t *testing.T) {
	cases := []struct {
		name     string
		id       string
		currency string
		balance  int64
		emptyID  bool
		code     string
	}{
		{name: "generated id", id: "", currency: "INR", balance: 0, emptyID: true},
		{name: "explicit id", id: "wallet_1", currency: "USD", balance: 10, emptyID: false},
		{name: "bad id", id: "bad id", currency: "INR", balance: 1, emptyID: false, code: "INVALID_REQUEST"},
		{name: "lowercase currency", id: "w1", currency: "inr", balance: 1, emptyID: false, code: "INVALID_REQUEST"},
		{name: "short currency", id: "w1", currency: "IN", balance: 1, emptyID: false, code: "INVALID_REQUEST"},
		{name: "unsupported currency", id: "w1", currency: "XYZ", balance: 1, emptyID: false, code: "INVALID_REQUEST"},
		{name: "zero decimal currency", id: "w1", currency: "JPY", balance: 1, emptyID: false},
		{name: "three decimal currency", id: "w1", currency: "KWD", balance: 1, emptyID: false},
		{name: "negative", id: "w1", currency: "INR", balance: -1, emptyID: false, code: "INVALID_AMOUNT"},
		{name: "too large", id: "w1", currency: "INR", balance: MaxOpeningBalance + 1, emptyID: false, code: "INVALID_AMOUNT"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateNewWallet(tc.id, tc.currency, tc.balance, tc.emptyID)
			if tc.code == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			apiErr, ok := AsError(err)
			if !ok || apiErr.Code != tc.code {
				t.Fatalf("err = %v, want %s", err, tc.code)
			}
		})
	}
}
