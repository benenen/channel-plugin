package cmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/benenen/myclaw/internal/config"
)

func TestRunServerReturnsFailureWhenConfigLoadFails(t *testing.T) {
	originalLoadConfig := loadConfig
	loadConfig = func() (config.Config, error) {
		return config.Config{}, errors.New("boom")
	}
	defer func() {
		loadConfig = originalLoadConfig
	}()

	var stderr bytes.Buffer

	exitCode := RunServer(&stderr)

	if exitCode != 1 {
		t.Fatalf("exitCode = %d, want 1", exitCode)
	}
	if !strings.Contains(stderr.String(), "load config: boom") {
		t.Fatalf("stderr = %q, want load config error", stderr.String())
	}
}

func TestServiceURLUsesLocalhostForWildcardAddress(t *testing.T) {
	got := serviceURL(":8080")

	if got != "http://localhost:8080" {
		t.Fatalf("serviceURL(:8080) = %q, want %q", got, "http://localhost:8080")
	}
}

func TestServiceURLPreservesExplicitHost(t *testing.T) {
	got := serviceURL("127.0.0.1:9090")

	if got != "http://127.0.0.1:9090" {
		t.Fatalf("serviceURL(127.0.0.1:9090) = %q, want %q", got, "http://127.0.0.1:9090")
	}
}
