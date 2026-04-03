package wechat

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/benenen/channel-plugin/internal/channel"
)

type Provider struct {
	client Client
}

func NewProvider(client Client) *Provider {
	return &Provider{client: client}
}

func (p *Provider) CreateBinding(ctx context.Context, req channel.CreateBindingRequest) (channel.CreateBindingResult, error) {
	result, err := p.client.CreateBindingSession(ctx, req.BindingID)
	if err != nil {
		return channel.CreateBindingResult{}, fmt.Errorf("wechat create binding: %w", err)
	}
	return channel.CreateBindingResult{
		ProviderBindingRef: result.ProviderBindingRef,
		QRCodePayload:      result.QRCodePayload,
		ExpiresAt:          result.ExpiresAt,
	}, nil
}

func (p *Provider) RefreshBinding(ctx context.Context, req channel.RefreshBindingRequest) (channel.RefreshBindingResult, error) {
	result, err := p.client.GetBindingSession(ctx, req.ProviderBindingRef)
	if err != nil {
		return channel.RefreshBindingResult{}, fmt.Errorf("wechat refresh binding: %w", err)
	}
	return channel.RefreshBindingResult{
		ProviderStatus:    result.Status,
		QRCodePayload:     result.QRCodePayload,
		ExpiresAt:         result.ExpiresAt,
		AccountUID:        result.AccountUID,
		DisplayName:       result.DisplayName,
		AvatarURL:         result.AvatarURL,
		CredentialPayload: result.CredentialPayload,
		CredentialVersion: result.CredentialVersion,
		ErrorMessage:      result.ErrorMessage,
	}, nil
}

func (p *Provider) BuildRuntimeConfig(ctx context.Context, req channel.BuildRuntimeConfigRequest) (channel.RuntimeConfig, error) {
	var payload map[string]any
	if err := json.Unmarshal(req.CredentialPayload, &payload); err != nil {
		return nil, fmt.Errorf("unmarshal credential payload: %w", err)
	}
	return channel.RuntimeConfig{
		"credential_blob": map[string]any{
			"version": req.CredentialVersion,
			"payload": payload,
		},
		"runtime_options": map[string]any{
			"poll_interval_seconds": 3,
		},
	}, nil
}
