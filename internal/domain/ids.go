package domain

import (
	"crypto/rand"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

func NewPrefixedID(prefix string) string {
	normalizedPrefix := strings.TrimSpace(prefix)
	if normalizedPrefix == "" {
		normalizedPrefix = "id"
	}

	entropy := ulid.Monotonic(rand.Reader, 0)
	id := ulid.MustNew(ulid.Timestamp(time.Now().UTC()), entropy)
	return fmt.Sprintf("%s_%s", normalizedPrefix, strings.ToLower(id.String()))
}
