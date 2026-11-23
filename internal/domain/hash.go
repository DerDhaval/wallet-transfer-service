package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// RequestHash identifies the transfer payload that an idempotency key is bound to.
// The key itself is not part of the hash. v2 is a version prefix with length-prefixed
// fields to prevent canonicalization ambiguity (delimiter collisions).
func RequestHash(fromWalletID, toWalletID string, amount int64) string {
	payload := fmt.Appendf(nil, "v2|%d:%s|%d:%s|%d", len(fromWalletID), fromWalletID, len(toWalletID), toWalletID, amount)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
