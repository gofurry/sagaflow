package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

const secretBoxPrefix = "v1:"

type SecretBox struct {
	aead cipher.AEAD
}

func NewSecretBox(secret string) (*SecretBox, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrCredentialEncryption
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &SecretBox{aead: aead}, nil
}

func (b *SecretBox) Encrypt(plaintext string) (string, error) {
	if b == nil {
		return "", ErrCredentialEncryption
	}
	if plaintext == "" {
		return "", fmt.Errorf("%w: plaintext is required", ErrInvalidInput)
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := b.aead.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return secretBoxPrefix + base64.StdEncoding.EncodeToString(payload), nil
}

func (b *SecretBox) Decrypt(encoded string) (string, error) {
	if b == nil {
		return "", ErrCredentialEncryption
	}
	if !strings.HasPrefix(encoded, secretBoxPrefix) {
		return "", fmt.Errorf("%w: unsupported encrypted secret format", ErrInvalidInput)
	}
	payload, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(encoded, secretBoxPrefix))
	if err != nil {
		return "", fmt.Errorf("%w: decode encrypted secret: %v", ErrCredentialEncryption, err)
	}
	nonceSize := b.aead.NonceSize()
	if len(payload) <= nonceSize {
		return "", fmt.Errorf("%w: encrypted secret is malformed", ErrCredentialEncryption)
	}
	nonce := payload[:nonceSize]
	ciphertext := payload[nonceSize:]
	plaintext, err := b.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: decrypt encrypted secret: %v", ErrCredentialEncryption, err)
	}
	return string(plaintext), nil
}
