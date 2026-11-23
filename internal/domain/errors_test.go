package domain

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrorMethods(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "nil", err: nil, want: ""},
		{name: "message", err: ErrInvalidAmount, want: ErrInvalidAmount.Message},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestErrorIs(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{name: "same code via WithMessage", err: ErrInsufficientFunds.WithMessage("short"), target: ErrInsufficientFunds, want: true},
		{name: "different code", err: ErrInsufficientFunds, target: ErrSameWallet, want: false},
		{name: "plain error", err: ErrSameWallet, target: errors.New("nope"), want: false},
		{name: "nil receiver", err: (*Error)(nil), target: ErrSameWallet, want: false},
		{name: "wrapped", err: fmt.Errorf("wrap: %w", ErrWalletNotFound), target: ErrWalletNotFound, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, tc.target); got != tc.want {
				t.Fatalf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestWithMessage(t *testing.T) {
	cases := []struct {
		name string
		err  *Error
		msg  string
	}{
		{name: "nil", err: nil, msg: "x"},
		{name: "clone", err: ErrInvalidRequest, msg: "missing field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.err.WithMessage(tc.msg)
			if tc.err == nil {
				if got != nil {
					t.Fatalf("got %#v", got)
				}
				return
			}
			if got == tc.err {
				t.Fatal("WithMessage must return a copy")
			}
			if got.Message != tc.msg || got.Code != tc.err.Code || got.Status != tc.err.Status {
				t.Fatalf("got %#v", got)
			}
			if tc.err.Message == tc.msg {
				t.Fatal("original message was mutated")
			}
		})
	}
}

func TestAsError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		ok   bool
		code string
	}{
		{name: "nil", err: nil, ok: false},
		{name: "plain", err: errors.New("x"), ok: false},
		{name: "domain", err: ErrInternal, ok: true, code: "INTERNAL_ERROR"},
		{name: "wrapped", err: fmt.Errorf("boom: %w", ErrTemporarilyUnavailable), ok: true, code: "TEMPORARILY_UNAVAILABLE"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := AsError(tc.err)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !ok {
				return
			}
			if got.Code != tc.code {
				t.Fatalf("code = %s, want %s", got.Code, tc.code)
			}
		})
	}
}
