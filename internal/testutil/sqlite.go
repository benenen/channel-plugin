package testutil

import (
	"testing"

	"github.com/benenen/channel-plugin/internal/store"
	"gorm.io/gorm"
)

func OpenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	if err := store.Migrate(db); err != nil {
		t.Fatalf("migrate test db: %v", err)
	}
	return db
}
