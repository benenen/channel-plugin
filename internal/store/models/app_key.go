package models

import "time"

type AppKey struct {
	ID               string `gorm:"primaryKey"`
	UserID           string `gorm:"not null"`
	ChannelAccountID string `gorm:"not null"`
	AppKeyHash       string `gorm:"not null;index:idx_app_keys_hash"`
	AppKeyPrefix     string `gorm:"not null"`
	Status           string `gorm:"not null"`
	LastUsedAt       *time.Time
	CreatedAt        time.Time
	DisabledAt       *time.Time
}
