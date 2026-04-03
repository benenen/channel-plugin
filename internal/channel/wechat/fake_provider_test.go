package wechat

import (
	"context"
	"testing"

	"github.com/benenen/channel-plugin/internal/channel"
)

func TestFakeProviderCreateAndRefreshBinding(t *testing.T) {
	provider := NewFakeProvider()
	ctx := context.Background()

	created, err := provider.CreateBinding(ctx, channel.CreateBindingRequest{
		BindingID:   "bind_1",
		ChannelType: "wechat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.ProviderBindingRef == "" {
		t.Fatal("expected provider binding ref")
	}

	refreshed, err := provider.RefreshBinding(ctx, channel.RefreshBindingRequest{
		ProviderBindingRef: created.ProviderBindingRef,
	})
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.ProviderStatus != "qr_ready" {
		t.Fatalf("unexpected status: %s", refreshed.ProviderStatus)
	}
}

func TestFakeProviderSimulateConfirm(t *testing.T) {
	provider := NewFakeProvider()
	ctx := context.Background()

	created, _ := provider.CreateBinding(ctx, channel.CreateBindingRequest{
		BindingID:   "bind_2",
		ChannelType: "wechat",
	})

	provider.SimulateConfirm(created.ProviderBindingRef)

	refreshed, _ := provider.RefreshBinding(ctx, channel.RefreshBindingRequest{
		ProviderBindingRef: created.ProviderBindingRef,
	})
	if refreshed.ProviderStatus != "confirmed" {
		t.Fatalf("expected confirmed, got: %s", refreshed.ProviderStatus)
	}
	if refreshed.AccountUID == "" {
		t.Fatal("expected account_uid on confirmed")
	}
	if len(refreshed.CredentialPayload) == 0 {
		t.Fatal("expected credential payload on confirmed")
	}
}

func TestFakeProviderBuildRuntimeConfig(t *testing.T) {
	provider := NewFakeProvider()
	ctx := context.Background()

	cfg, err := provider.BuildRuntimeConfig(ctx, channel.BuildRuntimeConfigRequest{
		AccountUID:        "wxid_test",
		CredentialPayload: []byte(`{"session":"x"}`),
		CredentialVersion: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if cfg["credential_blob"] == nil {
		t.Fatal("expected credential_blob in runtime config")
	}
}
