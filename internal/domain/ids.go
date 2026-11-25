package domain

import (
	"strings"

	"github.com/google/uuid"
)

// NewID returns a prefixed identifier. Prefixes keep logs readable; the random part is a UUID.
func NewID(prefix string) string {
	return prefix + "_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}
