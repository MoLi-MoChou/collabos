// Package util holds small conversion helpers.
package util

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// ParseUUID converts a string to pgtype.UUID.
func ParseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		return u, fmt.Errorf("invalid uuid %q: %w", s, err)
	}
	return u, nil
}

// MustUUID is ParseUUID for trusted values (round-trips from the database).
func MustUUID(s string) pgtype.UUID {
	u, err := ParseUUID(s)
	if err != nil {
		panic(err)
	}
	return u
}

// UUIDString formats a pgtype.UUID ("" when invalid).
func UUIDString(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// TS formats a timestamp as RFC3339 ("" when null).
func TS(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339Nano)
}

// OptUUID parses s, returning an invalid (NULL) UUID for "".
func OptUUID(s string) (pgtype.UUID, error) {
	if s == "" {
		return pgtype.UUID{}, nil
	}
	return ParseUUID(s)
}

// JSON marshals v, falling back to "null".
func JSON(v any) []byte {
	b, err := jsonMarshal(v)
	if err != nil {
		return []byte("null")
	}
	return b
}
