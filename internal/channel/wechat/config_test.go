package wechat

import "testing"

func TestLoadConfigUsesOfficialBaseURLByDefault(t *testing.T) {
	t.Setenv("WECHAT_REFERENCE_BASE_URL", "")
	t.Setenv("WECHAT_AUTH_TOKEN", "")

	cfg := LoadConfig()
	if cfg.ReferenceBaseURL != "https://ilinkai.weixin.qq.com" {
		t.Fatalf("unexpected default base url: %s", cfg.ReferenceBaseURL)
	}
}
