package app

import (
	"context"
	"errors"
	"time"

	"github.com/benenen/myclaw/internal/channel"
	"github.com/benenen/myclaw/internal/domain"
	"github.com/benenen/myclaw/internal/security"
)

type BindingService struct {
	users    domain.UserRepository
	bindings domain.ChannelBindingRepository
	accounts domain.ChannelAccountRepository
	cipher   *security.Cipher
	provider channel.Provider
}

func NewBindingService(
	users domain.UserRepository,
	bindings domain.ChannelBindingRepository,
	accounts domain.ChannelAccountRepository,
	cipher *security.Cipher,
	provider channel.Provider,
) *BindingService {
	return &BindingService{
		users:    users,
		bindings: bindings,
		accounts: accounts,
		cipher:   cipher,
		provider: provider,
	}
}

type CreateBindingInput struct {
	ExternalUserID string
	ChannelType    string
}

type CreateBindingOutput struct {
	BindingID     string
	Status        string
	QRCodePayload string
	ExpiresAt     *time.Time
}

func (s *BindingService) CreateBinding(ctx context.Context, input CreateBindingInput) (CreateBindingOutput, error) {
	if input.ExternalUserID == "" || input.ChannelType == "" {
		return CreateBindingOutput{}, domain.ErrInvalidArg
	}

	user, err := s.users.FindOrCreateByExternalUserID(ctx, input.ExternalUserID)
	if err != nil {
		return CreateBindingOutput{}, err
	}

	bindingID := domain.NewPrefixedID("bind")
	binding, err := s.bindings.Create(ctx, domain.ChannelBinding{
		ID:          bindingID,
		UserID:      user.ID,
		ChannelType: input.ChannelType,
		Status:      domain.BindingStatusPending,
	})
	if err != nil {
		return CreateBindingOutput{}, err
	}

	result, err := s.provider.CreateBinding(ctx, channel.CreateBindingRequest{
		BindingID:   bindingID,
		ChannelType: input.ChannelType,
	})
	if err != nil {
		binding.Status = domain.BindingStatusFailed
		binding.ErrorMessage = err.Error()
		now := time.Now().UTC()
		binding.FinishedAt = &now
		s.bindings.Update(ctx, binding)
		return CreateBindingOutput{}, errors.New("binding failed: " + err.Error())
	}

	binding.Status = domain.BindingStatusQRReady
	binding.ProviderBindingRef = result.ProviderBindingRef
	binding.QRCodePayload = result.QRCodePayload
	binding.ExpiresAt = &result.ExpiresAt
	binding, err = s.bindings.Update(ctx, binding)
	if err != nil {
		return CreateBindingOutput{}, err
	}

	return CreateBindingOutput{
		BindingID:     binding.ID,
		Status:        binding.Status,
		QRCodePayload: binding.QRCodePayload,
		ExpiresAt:     binding.ExpiresAt,
	}, nil
}

type BindingDetail struct {
	BindingID        string
	Status           string
	ChannelType      string
	ChannelAccountID string
	DisplayName      string
	AccountUID       string
	ExpiresAt        *time.Time
	ErrorMessage     string
}

func (s *BindingService) GetBindingDetail(ctx context.Context, bindingID string) (BindingDetail, error) {
	binding, err := s.bindings.GetByID(ctx, bindingID)
	if err != nil {
		return BindingDetail{}, err
	}

	// Terminal states don't need provider refresh
	if binding.Status == domain.BindingStatusConfirmed ||
		binding.Status == domain.BindingStatusFailed ||
		binding.Status == domain.BindingStatusExpired {
		return toBindingDetail(binding), nil
	}

	if binding.ProviderBindingRef == "" {
		return toBindingDetail(binding), nil
	}

	// Check local expiry
	if binding.ExpiresAt != nil && time.Now().UTC().After(*binding.ExpiresAt) {
		binding.Status = domain.BindingStatusExpired
		now := time.Now().UTC()
		binding.FinishedAt = &now
		binding, _ = s.bindings.Update(ctx, binding)
		return toBindingDetail(binding), nil
	}

	// Refresh from provider
	refreshed, err := s.provider.RefreshBinding(ctx, channel.RefreshBindingRequest{
		ProviderBindingRef: binding.ProviderBindingRef,
	})
	if err != nil {
		return toBindingDetail(binding), nil
	}

	switch refreshed.ProviderStatus {
	case "qr_ready":
		if refreshed.QRCodePayload != "" {
			binding.QRCodePayload = refreshed.QRCodePayload
		}
		if !refreshed.ExpiresAt.IsZero() {
			binding.ExpiresAt = &refreshed.ExpiresAt
		}
		binding, _ = s.bindings.Update(ctx, binding)

	case "confirmed":
		now := time.Now().UTC()
		encrypted, err := s.cipher.Encrypt(refreshed.CredentialPayload)
		if err != nil {
			return BindingDetail{}, err
		}
		account, err := s.accounts.Upsert(ctx, domain.ChannelAccount{
			ID:                   domain.NewPrefixedID("acct"),
			UserID:               binding.UserID,
			ChannelType:          binding.ChannelType,
			AccountUID:           refreshed.AccountUID,
			DisplayName:          refreshed.DisplayName,
			AvatarURL:            refreshed.AvatarURL,
			CredentialCiphertext: encrypted,
			CredentialVersion:    refreshed.CredentialVersion,
			LastBoundAt:          &now,
		})
		if err != nil {
			return BindingDetail{}, err
		}
		binding.Status = domain.BindingStatusConfirmed
		binding.ChannelAccountID = account.ID
		binding.FinishedAt = &now
		binding, _ = s.bindings.Update(ctx, binding)

	case "failed":
		now := time.Now().UTC()
		binding.Status = domain.BindingStatusFailed
		binding.ErrorMessage = refreshed.ErrorMessage
		binding.FinishedAt = &now
		binding, _ = s.bindings.Update(ctx, binding)

	case "expired":
		now := time.Now().UTC()
		binding.Status = domain.BindingStatusExpired
		binding.FinishedAt = &now
		binding, _ = s.bindings.Update(ctx, binding)
	}

	return toBindingDetail(binding), nil
}

func toBindingDetail(b domain.ChannelBinding) BindingDetail {
	return BindingDetail{
		BindingID:        b.ID,
		Status:           b.Status,
		ChannelType:      b.ChannelType,
		ChannelAccountID: b.ChannelAccountID,
		ExpiresAt:        b.ExpiresAt,
		ErrorMessage:     b.ErrorMessage,
	}
}
