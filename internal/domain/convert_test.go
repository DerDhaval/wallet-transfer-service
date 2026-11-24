package domain

import (
	"math"
	"regexp"
	"testing"
)

func TestCurrencyTableIsValid(t *testing.T) {
	code := regexp.MustCompile(`^[A-Z]{3}$`)
	seen := map[string]bool{}
	prev := ""
	for _, c := range Currencies() {
		if !code.MatchString(c.Code) || c.Name == "" || c.MinorUnits < 0 || c.MinorUnits > 3 {
			t.Fatalf("bad currency %+v", c)
		}
		if seen[c.Code] {
			t.Fatalf("duplicate currency %s", c.Code)
		}
		if c.Code < prev {
			t.Fatalf("currency table is not sorted: %s before %s", prev, c.Code)
		}
		seen[c.Code] = true
		prev = c.Code

		got, ok := lookupCurrency(c.Code)
		if !ok || got.rate == nil || got.rate.Sign() <= 0 {
			t.Fatalf("lookup/rate missing for %s", c.Code)
		}
	}
	for _, want := range []string{"USD", "EUR", "GBP", "INR", "JPY", "KWD", "AED", "AUD", "CAD", "CNY"} {
		if !seen[want] {
			t.Fatalf("%s is not supported", want)
		}
	}
	if len(seen) < 150 {
		t.Fatalf("only %d currencies", len(seen))
	}
}

func TestConvertTable(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		amount   int64
		wantTo   int64
		wantRate string
		code     string
	}{
		{name: "same currency", from: "INR", to: "INR", amount: 500, wantTo: 500, wantRate: "1"},
		{name: "same unknown currency", from: "XYZ", to: "XYZ", amount: 500, code: "CURRENCY_NOT_SUPPORTED"},
		{name: "inr to usd rounds down", from: "INR", to: "USD", amount: 10_000, wantTo: 112, wantRate: "0.011299435028"},
		{name: "usd to jpy no decimals", from: "USD", to: "JPY", amount: 100, wantTo: 152, wantRate: "152"},
		{name: "jpy to usd exact", from: "JPY", to: "USD", amount: 152, wantTo: 100, wantRate: "0.006578947368"},
		{name: "usd to kwd three decimals", from: "USD", to: "KWD", amount: 100, wantTo: 307, wantRate: "0.307"},
		{name: "kwd to usd exact", from: "KWD", to: "USD", amount: 307, wantTo: 100, wantRate: "3.257328990228"},
		{name: "eur to inr", from: "EUR", to: "INR", amount: 100, wantTo: 10_290, wantRate: "102.906976744186"},
		{name: "amount too small", from: "VND", to: "KWD", amount: 1, code: "AMOUNT_TOO_SMALL"},
		{name: "unknown source", from: "XYZ", to: "USD", amount: 100, code: "CURRENCY_NOT_SUPPORTED"},
		{name: "unknown destination", from: "USD", to: "XYZ", amount: 100, code: "CURRENCY_NOT_SUPPORTED"},
		{name: "overflow", from: "VND", to: "IRR", amount: math.MaxInt64, code: "AMOUNT_OVERFLOW"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Convert(tc.from, tc.to, tc.amount)
			if tc.code != "" {
				apiErr, ok := AsError(err)
				if !ok || apiErr.Code != tc.code {
					t.Fatalf("err = %v, want %s", err, tc.code)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.ToAmount != tc.wantTo || got.Rate != tc.wantRate || got.FromAmount != tc.amount {
				t.Fatalf("got %+v, want to=%d rate=%s", got, tc.wantTo, tc.wantRate)
			}
		})
	}
}

func TestConvertNeverCreatesMoney(t *testing.T) {
	for _, pair := range [][2]string{{"INR", "USD"}, {"USD", "JPY"}, {"EUR", "KWD"}, {"GBP", "INR"}} {
		out, err := Convert(pair[0], pair[1], 123_456)
		if err != nil {
			t.Fatal(err)
		}
		back, err := Convert(pair[1], pair[0], out.ToAmount)
		if err != nil {
			t.Fatal(err)
		}
		if back.ToAmount > 123_456 {
			t.Fatalf("%v: round trip created money: %d", pair, back.ToAmount)
		}
	}
}
