package identity

import (
	"crypto/sha1"
	"encoding/hex"

	"github.com/google/uuid"
)

// New returns a UUIDv7 string. v7 is time-ordered, so inserts land at the end
// of the B-tree instead of scattering across it like v4. That matters on SQLite,
// where random primary keys turn every insert into a page split.
func New() string {
	if id, err := uuid.NewV7(); err == nil {
		return id.String()
	}
	return uuid.NewString()
}

// Deterministic builds a stable id from a natural key. Used for seeds so a
// re-run does not duplicate rows.
func Deterministic(namespace, key string) string {
	sum := sha1.Sum([]byte(namespace + ":" + key))
	// Format as a UUID-shaped string so ids stay type-compatible.
	return hex.EncodeToString(sum[:16])
}
