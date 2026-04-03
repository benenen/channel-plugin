package domain

import (
	"strings"
	"testing"
)

func TestNewPrefixedIDUsesULIDShape(t *testing.T) {
	got := NewPrefixedID("bind")

	if !strings.HasPrefix(got, "bind_") {
		t.Fatalf("unexpected id: %s", got)
	}

	body := strings.TrimPrefix(got, "bind_")
	if len(body) != 26 {
		t.Fatalf("unexpected ULID length: %d", len(body))
	}
}
