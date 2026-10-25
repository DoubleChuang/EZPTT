package vault

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func newKey(t *testing.T) (*Key, string) {
	t.Helper()
	encoded, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	key, err := NewKey(encoded)
	if err != nil {
		t.Fatalf("NewKey: %v", err)
	}
	return key, encoded
}

func TestRoundTrip(t *testing.T) {
	key, _ := newKey(t)
	plaintext := []byte("alice,secret1\nbob,secret2\n")

	sealed, err := key.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !IsEncrypted(sealed) {
		t.Fatal("encrypted payload does not carry the magic header")
	}
	if bytes.Contains(sealed, []byte("secret1")) {
		t.Fatal("the ciphertext contains the plaintext password")
	}

	opened, err := key.Decrypt(sealed)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(opened, plaintext) {
		t.Fatalf("round trip = %q", opened)
	}
}

func TestNonceIsRandomPerFile(t *testing.T) {
	key, _ := newKey(t)
	first, _ := key.Encrypt([]byte("same input"))
	second, _ := key.Encrypt([]byte("same input"))
	if bytes.Equal(first, second) {
		t.Fatal("two encryptions of the same input are identical: nonce is reused")
	}
}

func TestWrongKeyFails(t *testing.T) {
	key, _ := newKey(t)
	other, _ := newKey(t)

	sealed, _ := key.Encrypt([]byte("alice,secret"))
	if _, err := other.Decrypt(sealed); err == nil {
		t.Fatal("a different key must fail the authentication check")
	}
}

func TestTamperedCiphertextFails(t *testing.T) {
	key, _ := newKey(t)
	sealed, _ := key.Encrypt([]byte("alice,secret"))
	sealed[len(sealed)-1] ^= 0xff
	if _, err := key.Decrypt(sealed); err == nil {
		t.Fatal("a modified ciphertext must be rejected")
	}
}

func TestPlaintextIsNotEncrypted(t *testing.T) {
	key, _ := newKey(t)
	if _, err := key.Decrypt([]byte("alice,secret")); err == nil {
		t.Fatal("plaintext must not decrypt")
	}
	if IsEncrypted([]byte("alice,secret")) {
		t.Fatal("plaintext detected as encrypted")
	}
}

func TestNewKeyValidation(t *testing.T) {
	if _, err := NewKey(""); err == nil {
		t.Fatal("empty key accepted")
	}
	if _, err := NewKey("not base64!!"); err == nil {
		t.Fatal("invalid base64 accepted")
	}
	short := make([]byte, 16)
	if _, err := NewKey(base64.StdEncoding.EncodeToString(short)); err == nil {
		t.Fatal("16-byte key accepted")
	}
}

func TestReadKeyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "key")
	encoded, _ := GenerateKey()
	if err := os.WriteFile(path, []byte(encoded+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadKeyFile(path)
	if err != nil {
		t.Fatalf("ReadKeyFile: %v", err)
	}
	if got != encoded {
		t.Fatalf("key = %q", got)
	}

	// A key file readable by others is a configuration error, not a warning.
	loose := filepath.Join(dir, "loose")
	if err := os.WriteFile(loose, []byte(encoded), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadKeyFile(loose); err == nil || !strings.Contains(err.Error(), "600") {
		t.Fatalf("world-readable key file accepted: %v", err)
	}
}
