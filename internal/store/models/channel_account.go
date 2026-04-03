package models

import "time"

type ChannelAccount struct {
	ID                   string `gorm:"primaryKey"`
	UserID               string `gorm:"not null;uniqueIndex:idx_channel_accounts_unique_user_channel_uid"`
	ChannelType          string `gorm:"not null;uniqueIndex:idx_channel_accounts_unique_user_channel_uid"`
	AccountUID           string `gorm:"not null;uniqueIndex:idx_channel_accounts_unique_user_channel_uid"`
	DisplayName          string `gorm:"not null;default:''"`
	AvatarURL            string `gorm:"not null;default:''"`
	CredentialCiphertext []byte
	CredentialVersion    int        `gorm:"not null;default:0"`
	LastBoundAt          *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}
