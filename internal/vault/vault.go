// Package vault encrypts the account file at rest.
//
// The cipher is AES-256-GCM with a random 96-bit nonce per file; the key is 32
// bytes, base64-encoded, and is expected to live outside the data directory
// (a 0600 file mounted read-only, or an environment variable). Wire format:
//
//	"EZPTT1" || nonce (12 bytes) || GCM ciphertext+tag
//
// AES-GCM authenticates the ciphertext, so a wrong key or an edited file is a
// hard error rather than a silently mangled account list.
package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

// Magic marks an encrypted file and carries the format version.
const Magic = "EZPTT1"

// KeySize is the AES-256 key length.
const KeySize = 32

// ErrNotEncrypted is returned when the file does not start with Magic.
var ErrNotEncrypted = errors.New("檔案不是 EZPTT 加密格式")

// Key wraps the AEAD built from the raw key.
type Key struct {
	aead cipher.AEAD
}

// NewKey parses a base64-encoded 32-byte key.
func NewKey(encoded string) (*Key, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return nil, errors.New("金鑰是空的")
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("金鑰不是合法的 base64: %w", err)
	}
	if len(raw) != KeySize {
		return nil, fmt.Errorf("金鑰長度 %d bytes，必須是 %d bytes", len(raw), KeySize)
	}
	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("建立 AES cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("建立 GCM: %w", err)
	}
	return &Key{aead: aead}, nil
}

// GenerateKey returns a fresh base64-encoded 32-byte key.
func GenerateKey() (string, error) {
	raw := make([]byte, KeySize)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("產生金鑰: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

// ReadKeyFile reads a base64 key from path, which should be mode 0600.
func ReadKeyFile(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("讀取金鑰檔 %s: %w", path, err)
	}
	if mode := info.Mode().Perm(); mode&0o077 != 0 {
		return "", fmt.Errorf("金鑰檔 %s 權限是 %#o，必須是 600", path, mode)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("讀取金鑰檔 %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// Encrypt returns Magic || nonce || ciphertext.
func (k *Key) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, k.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("產生 nonce: %w", err)
	}
	out := make([]byte, 0, len(Magic)+len(nonce)+len(plaintext)+k.aead.Overhead())
	out = append(out, Magic...)
	out = append(out, nonce...)
	return k.aead.Seal(out, nonce, plaintext, nil), nil
}

// Decrypt reverses Encrypt. A wrong key fails the authentication check.
func (k *Key) Decrypt(raw []byte) ([]byte, error) {
	if !IsEncrypted(raw) {
		return nil, ErrNotEncrypted
	}
	body := raw[len(Magic):]
	nonceSize := k.aead.NonceSize()
	if len(body) < nonceSize+k.aead.Overhead() {
		return nil, errors.New("加密檔被截斷")
	}
	nonce, ciphertext := body[:nonceSize], body[nonceSize:]
	plaintext, err := k.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.New("解密失敗：金鑰不對，或檔案被修改過")
	}
	return plaintext, nil
}

// IsEncrypted reports whether raw carries the EZPTT magic header.
func IsEncrypted(raw []byte) bool {
	return len(raw) >= len(Magic) && string(raw[:len(Magic)]) == Magic
}
