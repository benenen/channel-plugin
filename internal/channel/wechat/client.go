package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client interface {
	CreateBindingSession(ctx context.Context, bindingID string) (CreateSessionResult, error)
	GetBindingSession(ctx context.Context, providerRef string) (GetSessionResult, error)
}

type CreateSessionResult struct {
	ProviderBindingRef string    `json:"provider_binding_ref"`
	QRCodePayload      string    `json:"qr_code_payload"`
	ExpiresAt          time.Time `json:"expires_at"`
}

type GetSessionResult struct {
	Status            string          `json:"status"`
	QRCodePayload     string          `json:"qr_code_payload"`
	ExpiresAt         time.Time       `json:"expires_at"`
	AccountUID        string          `json:"account_uid"`
	DisplayName       string          `json:"display_name"`
	AvatarURL         string          `json:"avatar_url"`
	CredentialPayload json.RawMessage `json:"credential_payload"`
	CredentialVersion int             `json:"credential_version"`
	ErrorMessage      string          `json:"error_message"`
}

type HTTPClient struct {
	baseURL   string
	authToken string
	client    *http.Client
}

func NewHTTPClient(cfg Config) *HTTPClient {
	return &HTTPClient{
		baseURL:   cfg.ReferenceBaseURL,
		authToken: cfg.AuthToken,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *HTTPClient) CreateBindingSession(ctx context.Context, bindingID string) (CreateSessionResult, error) {
	body, _ := json.Marshal(map[string]string{"binding_id": bindingID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/wechat/bindings", bytes.NewReader(body))
	if err != nil {
		return CreateSessionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return CreateSessionResult{}, fmt.Errorf("create binding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return CreateSessionResult{}, fmt.Errorf("create binding: status %d, body: %s", resp.StatusCode, b)
	}

	var result CreateSessionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return CreateSessionResult{}, fmt.Errorf("decode create binding response: %w", err)
	}
	return result, nil
}

func (c *HTTPClient) GetBindingSession(ctx context.Context, providerRef string) (GetSessionResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/wechat/bindings/"+providerRef, nil)
	if err != nil {
		return GetSessionResult{}, err
	}
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return GetSessionResult{}, fmt.Errorf("get binding request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return GetSessionResult{}, fmt.Errorf("get binding: status %d, body: %s", resp.StatusCode, b)
	}

	var result GetSessionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return GetSessionResult{}, fmt.Errorf("decode get binding response: %w", err)
	}
	return result, nil
}
