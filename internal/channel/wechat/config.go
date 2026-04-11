package wechat

import "os"

type Config struct {
	ReferenceBaseURL string
	AuthToken        string
}

func LoadConfig() Config {
	return Config{
		ReferenceBaseURL: getEnvOrDefault("WECHAT_REFERENCE_BASE_URL", "https://ilinkai.weixin.qq.com"),
		AuthToken:        os.Getenv("WECHAT_AUTH_TOKEN"),
	}
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
