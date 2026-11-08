package memory

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// SyntheticKeyWrapper is for synthetic integration environments. It is not a KMS.
type SyntheticKeyWrapper struct{ key []byte }

func NewSyntheticKeyWrapper(hexKey string) (*SyntheticKeyWrapper, error) {
	key, err := hex.DecodeString(hexKey)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("%w: synthetic wrapping key must be 32 bytes", ErrInvalid)
	}
	return &SyntheticKeyWrapper{key: key}, nil
}
func (w *SyntheticKeyWrapper) Wrap(_ context.Context, value, aad []byte) ([]byte, error) {
	return seal(w.key, value, aad)
}
func (w *SyntheticKeyWrapper) Unwrap(_ context.Context, value, aad []byte) ([]byte, error) {
	return open(w.key, value, aad)
}
func seal(key, value, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return nil, err
	}
	return aead.Seal(nonce, nonce, value, aad), nil
}
func open(key, value, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(value) < aead.NonceSize() {
		return nil, fmt.Errorf("memory ciphertext invalid")
	}
	return aead.Open(nil, value[:aead.NonceSize()], value[aead.NonceSize():], aad)
}
