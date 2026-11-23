package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// RequestHash identifies the transfer payload that an idempotency key is bound to.
// The key itself is not part of the hash. v1 is a version prefix so the canonical
// form can change later without silently matching old rows.
func RequestHash(fromWalletID, toWalletID string, amount int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("v1|%s|%s|%d", fromWalletID, toWalletID, amount)))
	return hex.EncodeToString(sum[:])
}
