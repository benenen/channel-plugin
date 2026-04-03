package config

import (
	"encoding/base64"
	"testing"
)

func TestLoadConfigRequiresMasterKey(t *testing.T) {
	t.Setenv("CHANNEL_MASTER_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsInvalidMasterKeyLength(t *testing.T) {
	t.Setenv("CHANNEL_MASTER_KEY", base64.StdEncoding.EncodeToString([]byte("short")))

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid key length error")
	}
}
