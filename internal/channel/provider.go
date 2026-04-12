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
	QRShareURL         string
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

type RuntimeEvent struct {
	BotID       string
	ChannelType string
	MessageID   string
	From        string
	Text        string
	Raw         []byte
}

type RuntimeState string

const (
	RuntimeStateConnected RuntimeState = "connected"
	RuntimeStateError     RuntimeState = "error"
	RuntimeStateStopped   RuntimeState = "stopped"
)

type RuntimeStateEvent struct {
	BotID       string
	ChannelType string
	State       RuntimeState
	Err         error
	Reason      string
}

type RuntimeCallbacks struct {
	OnEvent func(RuntimeEvent)
	OnState func(RuntimeStateEvent)
}

type StartRuntimeRequest struct {
	BotID             string
	ChannelType       string
	AccountUID        string
	CredentialPayload []byte
	CredentialVersion int
	Callbacks         RuntimeCallbacks
}

type RuntimeHandle interface {
	Stop()
	Done() <-chan struct{}
}

type RuntimeStarter interface {
	StartRuntime(ctx context.Context, req StartRuntimeRequest) (RuntimeHandle, error)
}
