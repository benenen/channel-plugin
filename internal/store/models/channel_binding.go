package models

import "time"

type ChannelBinding struct {
	ID                 string `gorm:"primaryKey"`
	BotID              string `gorm:"not null;default:''"`
	UserID             string `gorm:"not null"`
	ChannelType        string `gorm:"not null"`
	Status             string `gorm:"not null"`
	ProviderBindingRef string `gorm:"not null;default:''"`
	QRCodePayload      string `gorm:"not null;default:''"`
	ExpiresAt          *time.Time
	ErrorMessage       string `gorm:"not null;default:''"`
	ChannelAccountID   string `gorm:"not null;default:''"`
	CreatedAt          time.Time
	UpdatedAt          time.Time
	FinishedAt         *time.Time
}
