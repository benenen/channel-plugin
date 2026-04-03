package wechat

import "os"

type Config struct {
	ReferenceBaseURL string
	AuthToken        string
}

func LoadConfig() Config {
	return Config{
		ReferenceBaseURL: getEnvOrDefault("WECHAT_REFERENCE_BASE_URL", "http://localhost:9090"),
		AuthToken:        os.Getenv("WECHAT_AUTH_TOKEN"),
	}
}

func getEnvOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
