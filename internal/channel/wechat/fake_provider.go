package wechat

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/benenen/channel-plugin/internal/channel"
)

type FakeProvider struct {
	mu     sync.Mutex
	states map[string]*fakeBindingState
}

type fakeBindingState struct {
	status            string
	qrCodePayload     string
	expiresAt         time.Time
	accountUID        string
	displayName       string
	avatarURL         string
	credentialPayload []byte
	credentialVersion int
	errorMessage      string
}

func NewFakeProvider() *FakeProvider {
	return &FakeProvider{
		states: make(map[string]*fakeBindingState),
	}
}

func (p *FakeProvider) CreateBinding(_ context.Context, req channel.CreateBindingRequest) (channel.CreateBindingResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	ref := "wxbind_" + req.BindingID
	p.states[ref] = &fakeBindingState{
		status:        "qr_ready",
		qrCodePayload: "weixin://fake_qr_" + req.BindingID,
		expiresAt:     time.Now().Add(5 * time.Minute),
	}

	return channel.CreateBindingResult{
		ProviderBindingRef: ref,
		QRCodePayload:      p.states[ref].qrCodePayload,
		ExpiresAt:          p.states[ref].expiresAt,
	}, nil
}

func (p *FakeProvider) RefreshBinding(_ context.Context, req channel.RefreshBindingRequest) (channel.RefreshBindingResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.states[req.ProviderBindingRef]
	if !ok {
		return channel.RefreshBindingResult{
			ProviderStatus: "expired",
			ErrorMessage:   "binding not found",
		}, nil
	}

	return channel.RefreshBindingResult{
		ProviderStatus:    state.status,
		QRCodePayload:     state.qrCodePayload,
		ExpiresAt:         state.expiresAt,
		AccountUID:        state.accountUID,
		DisplayName:       state.displayName,
		AvatarURL:         state.avatarURL,
		CredentialPayload: state.credentialPayload,
		CredentialVersion: state.credentialVersion,
		ErrorMessage:      state.errorMessage,
	}, nil
}

func (p *FakeProvider) BuildRuntimeConfig(_ context.Context, req channel.BuildRuntimeConfigRequest) (channel.RuntimeConfig, error) {
	var payload map[string]any
	if req.CredentialPayload != nil {
		json.Unmarshal(req.CredentialPayload, &payload)
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

// SimulateConfirm simulates a successful login for testing.
func (p *FakeProvider) SimulateConfirm(providerBindingRef string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	state, ok := p.states[providerBindingRef]
	if !ok {
		return
	}
	state.status = "confirmed"
	state.accountUID = "wxid_fake_user"
	state.displayName = "Fake User"
	state.avatarURL = "https://example.com/avatar.png"
	state.credentialPayload, _ = json.Marshal(map[string]any{
		"wechat_session": map[string]string{"token": "fake_token"},
		"device":         map[string]string{"id": "fake_device"},
	})
	state.credentialVersion = 1
}
