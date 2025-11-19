package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

const (
	// DEKSize is the size of the data encryption key in bytes (AES-256)
	DEKSize = 32
	// NonceSize is the size of the nonce for GCM (12 bytes recommended)
	NonceSize = 12
)

// Crypto handles encryption/decryption operations
type Crypto struct {
	masterKey []byte
}

// NewCrypto creates a new Crypto instance with the master key from environment
func NewCrypto() (*Crypto, error) {
	masterKeyB64 := os.Getenv("NOT_ENV_MASTER_KEY")
	if masterKeyB64 == "" {
		return nil, fmt.Errorf("NOT_ENV_MASTER_KEY environment variable is required")
	}

	masterKey, err := base64.StdEncoding.DecodeString(masterKeyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode master key: %w", err)
	}

	if len(masterKey) != 32 {
		return nil, fmt.Errorf("master key must be 32 bytes (256 bits) when base64 decoded")
	}

	return &Crypto{masterKey: masterKey}, nil
}

// GenerateDEK generates a new data encryption key
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, DEKSize)
	if _, err := io.ReadFull(rand.Reader, dek); err != nil {
		return nil, fmt.Errorf("failed to generate DEK: %w", err)
	}
	return dek, nil
}

// EncryptDEK encrypts a data encryption key with the master key
func (c *Crypto) EncryptDEK(dek []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(c.masterKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, dek, nil)
	return ciphertext, nonce, nil
}

// DecryptDEK decrypts a data encryption key with the master key
func (c *Crypto) DecryptDEK(encryptedDEK []byte, nonce []byte) ([]byte, error) {
	block, err := aes.NewCipher(c.masterKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	dek, err := gcm.Open(nil, nonce, encryptedDEK, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK: %w", err)
	}

	return dek, nil
}

// EncryptValue encrypts a value using a data encryption key
func EncryptValue(value string, dek []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, NonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, []byte(value), nil)
	return ciphertext, nonce, nil
}

// DecryptValue decrypts a value using a data encryption key
func DecryptValue(encryptedValue []byte, nonce []byte, dek []byte) (string, error) {
	block, err := aes.NewCipher(dek)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, encryptedValue, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt value: %w", err)
	}

	return string(plaintext), nil
}

