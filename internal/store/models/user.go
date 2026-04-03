package models

import "time"

type User struct {
	ID             string `gorm:"primaryKey"`
	ExternalUserID string `gorm:"uniqueIndex;not null"`
	Status         string `gorm:"not null"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
}
