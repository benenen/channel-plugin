package app

import (
	"context"
	"testing"

	"github.com/benenen/myclaw/internal/channel/wechat"
	"github.com/benenen/myclaw/internal/domain"
	"github.com/benenen/myclaw/internal/security"
	"github.com/benenen/myclaw/internal/store/repositories"
	"github.com/benenen/myclaw/internal/testutil"
)

func newTestBindingService(t *testing.T) (*BindingService, *wechat.FakeProvider) {
	t.Helper()
	db := testutil.OpenTestDB(t)
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	cipher, _ := security.NewCipher(key)
	provider := wechat.NewFakeProvider()
	svc := NewBindingService(
		repositories.NewUserRepository(db),
		repositories.NewChannelBindingRepository(db),
		repositories.NewChannelAccountRepository(db),
		cipher,
		provider,
	)
	return svc, provider
}

func TestCreateBindingCreatesUserAndReturnsQRReady(t *testing.T) {
	svc, _ := newTestBindingService(t)
	ctx := context.Background()

	got, err := svc.CreateBinding(ctx, CreateBindingInput{
		ExternalUserID: "u_123",
		ChannelType:    "wechat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.BindingStatusQRReady {
		t.Fatalf("expected qr_ready, got: %s", got.Status)
	}
	if got.QRCodePayload == "" {
		t.Fatal("expected qr_code_payload")
	}
}

func TestGetBindingDetailRefreshesProviderAndConfirmsAccount(t *testing.T) {
	svc, provider := newTestBindingService(t)
	ctx := context.Background()

	created, err := svc.CreateBinding(ctx, CreateBindingInput{
		ExternalUserID: "u_123",
		ChannelType:    "wechat",
	})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate provider confirming the binding
	binding, _ := svc.bindings.GetByID(ctx, created.BindingID)
	provider.SimulateConfirm(binding.ProviderBindingRef)

	got, err := svc.GetBindingDetail(ctx, created.BindingID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != domain.BindingStatusConfirmed {
		t.Fatalf("expected confirmed, got: %s", got.Status)
	}
	if got.ChannelAccountID == "" {
		t.Fatal("expected channel_account_id after confirmation")
	}
}

func TestCreateBindingRejectsEmptyInput(t *testing.T) {
	svc, _ := newTestBindingService(t)
	ctx := context.Background()

	_, err := svc.CreateBinding(ctx, CreateBindingInput{})
	if err != domain.ErrInvalidArg {
		t.Fatalf("expected ErrInvalidArg, got: %v", err)
	}
}
