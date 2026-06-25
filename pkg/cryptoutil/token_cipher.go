package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

var (
	ErrInvalidKey   = errors.New("token encryption key must be 32 bytes")
	ErrInvalidToken = errors.New("invalid encrypted token payload")
)

// TokenCipher encrypts Meta access tokens at rest (AES-256-GCM).
type TokenCipher struct {
	key []byte
}

func NewTokenCipherFromBase64(encoded string) (*TokenCipher, error) {
	if encoded == "" {
		return nil, ErrInvalidKey
	}
	key, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode token encryption key: %w", err)
	}
	if len(key) != 32 {
		return nil, ErrInvalidKey
	}
	return &TokenCipher{key: key}, nil
}

func (c *TokenCipher) Encrypt(plaintext string) ([]byte, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, []byte(plaintext), nil), nil
}

func (c *TokenCipher) Decrypt(ciphertext []byte) (string, error) {
	block, err := aes.NewCipher(c.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertext) < gcm.NonceSize() {
		return "", ErrInvalidToken
	}
	nonce, payload := ciphertext[:gcm.NonceSize()], ciphertext[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, payload, nil)
	if err != nil {
		return "", ErrInvalidToken
	}
	return string(plain), nil
}
