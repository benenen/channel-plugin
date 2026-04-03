package security

import (
	"strings"
	"testing"
)

func TestGenerateAppKeyReturnsPrefixAndHash(t *testing.T) {
	key, prefix, hash, err := GenerateAppKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "appk_") {
		t.Fatalf("unexpected key prefix: %s", key)
	}
	if len(prefix) != 8 {
		t.Fatalf("unexpected prefix length: %d", len(prefix))
	}
	if len(hash) == 0 {
		t.Fatal("hash is empty")
	}
}

func TestHashAppKeyIsConsistent(t *testing.T) {
	h1 := HashAppKey("appk_test123")
	h2 := HashAppKey("appk_test123")
	if h1 != h2 {
		t.Fatal("hash not consistent")
	}
}
