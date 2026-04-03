package store

import (
	"embed"
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func Open(dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	return db, nil
}

func Migrate(db *gorm.DB) error {
	sql, err := migrationsFS.ReadFile("migrations/0001_init.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if err := db.Exec(string(sql)).Error; err != nil {
		return fmt.Errorf("exec migration: %w", err)
	}
	return nil
}
