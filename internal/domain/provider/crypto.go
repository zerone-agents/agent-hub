package provider

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
)

// Encrypt encrypts plaintext using AES-GCM with a hex-encoded 32-byte key.
// If hexKey is empty, plaintext is returned as-is (dev mode, no encryption).
func Encrypt(plaintext, hexKey string) (string, error) {
	if hexKey == "" {
		return plaintext, nil
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", errors.New("invalid encryption key: must be hex-encoded")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return "enc:" + hex.EncodeToString(ciphertext), nil
}

// Decrypt reverses Encrypt. If hexKey is empty or the ciphertext is not
// prefixed with "enc:", the input is returned as-is.
func Decrypt(stored, hexKey string) (string, error) {
	if hexKey == "" {
		return stored, nil
	}

	// Unencrypted values (created before encryption was enabled) pass through.
	if len(stored) < 4 || stored[:4] != "enc:" {
		return stored, nil
	}

	key, err := hex.DecodeString(hexKey)
	if err != nil {
		return "", errors.New("invalid encryption key: must be hex-encoded")
	}

	ciphertext, err := hex.DecodeString(stored[4:])
	if err != nil {
		return "", errors.New("invalid ciphertext format")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", errors.New("decryption failed: wrong key or corrupted data")
	}

	return string(plaintext), nil
}
