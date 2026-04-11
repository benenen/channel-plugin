package wechat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHTTPClientCreateBindingSession(t *testing.T) {
	var gotPath string
	var gotBotType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotBotType = r.URL.Query().Get("bot_type")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"qrcode":"qr_token_1","qrcode_url":"weixin://qr_token_1","expires_at":"2026-04-11T00:00:00Z"}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(Config{ReferenceBaseURL: ts.URL})
	result, err := client.CreateBindingSession(context.Background(), "bind_1")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/ilink/bot/get_bot_qrcode" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotBotType != "3" {
		t.Fatalf("unexpected bot_type: %s", gotBotType)
	}
	if result.QRCode != "qr_token_1" {
		t.Fatalf("unexpected qrcode: %s", result.QRCode)
	}
}

func TestHTTPClientCreateBindingSessionReturnsErrorWhenBotTypeMissing(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("bot_type") == "" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"err_msg":"missing bot_type","ret":1}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"qrcode":"qr_token_1","qrcode_url":"weixin://qr_token_1"}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(Config{ReferenceBaseURL: ts.URL})
	result, err := client.CreateBindingSession(context.Background(), "bind_1")
	if err != nil {
		t.Fatal(err)
	}
	if result.QRCode == "" {
		t.Fatal("expected qrcode")
	}
}

func TestHTTPClientGetBindingSession(t *testing.T) {
	var gotPath string
	var gotQRCode string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQRCode = r.URL.Query().Get("qrcode")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"confirmed","qrcode":"qr_token_1","openid":"wxid_1","nickname":"bot-user","expires_at":"2026-04-11T00:00:00Z"}`))
	}))
	defer ts.Close()

	client := NewHTTPClient(Config{ReferenceBaseURL: ts.URL})
	result, err := client.GetBindingSession(context.Background(), "qr_token_1")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/ilink/bot/get_qrcode_status" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
	if gotQRCode != "qr_token_1" {
		t.Fatalf("unexpected qrcode query: %s", gotQRCode)
	}
	if result.OpenID != "wxid_1" {
		t.Fatalf("unexpected openid: %s", result.OpenID)
	}
}
