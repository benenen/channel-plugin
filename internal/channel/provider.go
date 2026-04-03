package channel

import (
	"context"
	"time"
)

type Provider interface {
	CreateBinding(ctx context.Context, req CreateBindingRequest) (CreateBindingResult, error)
	RefreshBinding(ctx context.Context, req RefreshBindingRequest) (RefreshBindingResult, error)
	BuildRuntimeConfig(ctx context.Context, req BuildRuntimeConfigRequest) (RuntimeConfig, error)
}

type CreateBindingRequest struct {
	BindingID   string
	ChannelType string
}

type CreateBindingResult struct {
	ProviderBindingRef string
	QRCodePayload      string
	ExpiresAt          time.Time
}

type RefreshBindingRequest struct {
	ProviderBindingRef string
}

type RefreshBindingResult struct {
	ProviderStatus    string
	QRCodePayload     string
	ExpiresAt         time.Time
	AccountUID        string
	DisplayName       string
	AvatarURL         string
	CredentialPayload []byte
	CredentialVersion int
	ErrorMessage      string
}

type BuildRuntimeConfigRequest struct {
	AccountUID        string
	CredentialPayload []byte
	CredentialVersion int
}

type RuntimeConfig map[string]any
