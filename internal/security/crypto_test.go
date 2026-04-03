package security

import (
	"crypto/rand"
	"testing"
)

func mustKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatal(err)
	}
	return key
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewCipher(mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"session":"x"}`)
	ciphertext, err := c.Encrypt(plaintext)
	if err != nil {
		t.Fatal(err)
	}
	roundTrip, err := c.Decrypt(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if string(roundTrip) != string(plaintext) {
		t.Fatal("round trip mismatch")
	}
}

func TestDecryptRejectsTamperedData(t *testing.T) {
	c, err := NewCipher(mustKey(t))
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, _ := c.Encrypt([]byte("secret"))
	ciphertext[len(ciphertext)-1] ^= 0xff
	if _, err := c.Decrypt(ciphertext); err == nil {
		t.Fatal("expected error on tampered data")
	}
}
