package domain

import (
	"strings"
	"testing"
)

func TestNewID(t *testing.T) {
	cases := []struct {
		name   string
		prefix string
	}{
		{name: "wallet", prefix: "wal"},
		{name: "transfer", prefix: "trf"},
		{name: "request", prefix: "req"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := NewID(tc.prefix)
			if !strings.HasPrefix(id, tc.prefix+"_") {
				t.Fatalf("id = %s", id)
			}
			if strings.Contains(id, "-") {
				t.Fatalf("uuid dashes should be stripped: %s", id)
			}
			other := NewID(tc.prefix)
			if id == other {
				t.Fatal("ids must be unique")
			}
		})
	}
}
