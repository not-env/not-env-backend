package crypto

import (
	"encoding/base64"
	"os"
	"testing"
)

func TestGenerateDEK(t *testing.T) {
	dek1, err := GenerateDEK()
	if err != nil {
		t.Fatalf("Failed to generate DEK: %v", err)
	}

	dek2, err := GenerateDEK()
	if err != nil {
		t.Fatalf("Failed to generate DEK: %v", err)
	}

	if len(dek1) != DEKSize {
		t.Errorf("DEK1 size is %d, expected %d", len(dek1), DEKSize)
	}

	if len(dek2) != DEKSize {
		t.Errorf("DEK2 size is %d, expected %d", len(dek2), DEKSize)
	}

	// DEKs should be different
	if string(dek1) == string(dek2) {
		t.Error("Generated DEKs are identical, they should be random")
	}
}

func TestEncryptDecryptDEK(t *testing.T) {
	// Generate a test master key
	masterKey := make([]byte, 32)
	for i := range masterKey {
		masterKey[i] = byte(i)
	}
	masterKeyB64 := base64.StdEncoding.EncodeToString(masterKey)

	os.Setenv("NOT_ENV_MASTER_KEY", masterKeyB64)
	defer os.Unsetenv("NOT_ENV_MASTER_KEY")

	crypto, err := NewCrypto()
	if err != nil {
		t.Fatalf("Failed to create crypto: %v", err)
	}

	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("Failed to generate DEK: %v", err)
	}

	encryptedDEK, nonce, err := crypto.EncryptDEK(dek)
	if err != nil {
		t.Fatalf("Failed to encrypt DEK: %v", err)
	}

	if len(encryptedDEK) == 0 {
		t.Error("Encrypted DEK is empty")
	}

	if len(nonce) != NonceSize {
		t.Errorf("Nonce size is %d, expected %d", len(nonce), NonceSize)
	}

	decryptedDEK, err := crypto.DecryptDEK(encryptedDEK, nonce)
	if err != nil {
		t.Fatalf("Failed to decrypt DEK: %v", err)
	}

	if string(decryptedDEK) != string(dek) {
		t.Error("Decrypted DEK does not match original")
	}
}

func TestEncryptDecryptValue(t *testing.T) {
	dek, err := GenerateDEK()
	if err != nil {
		t.Fatalf("Failed to generate DEK: %v", err)
	}

	testValue := "test-secret-value-123"
	encryptedValue, nonce, err := EncryptValue(testValue, dek)
	if err != nil {
		t.Fatalf("Failed to encrypt value: %v", err)
	}

	if len(encryptedValue) == 0 {
		t.Error("Encrypted value is empty")
	}

	if len(nonce) != NonceSize {
		t.Errorf("Nonce size is %d, expected %d", len(nonce), NonceSize)
	}

	decryptedValue, err := DecryptValue(encryptedValue, nonce, dek)
	if err != nil {
		t.Fatalf("Failed to decrypt value: %v", err)
	}

	if decryptedValue != testValue {
		t.Errorf("Decrypted value is %q, expected %q", decryptedValue, testValue)
	}
}

func TestNewCrypto(t *testing.T) {
	// Test missing master key
	os.Unsetenv("NOT_ENV_MASTER_KEY")
	_, err := NewCrypto()
	if err == nil {
		t.Error("Expected error when NOT_ENV_MASTER_KEY is missing")
	}

	// Test invalid base64
	os.Setenv("NOT_ENV_MASTER_KEY", "invalid-base64!")
	_, err = NewCrypto()
	if err == nil {
		t.Error("Expected error when master key is invalid base64")
	}

	// Test wrong size
	smallKey := base64.StdEncoding.EncodeToString([]byte("small"))
	os.Setenv("NOT_ENV_MASTER_KEY", smallKey)
	_, err = NewCrypto()
	if err == nil {
		t.Error("Expected error when master key is wrong size")
	}

	// Test valid key
	validKey := make([]byte, 32)
	for i := range validKey {
		validKey[i] = byte(i)
	}
	validKeyB64 := base64.StdEncoding.EncodeToString(validKey)
	os.Setenv("NOT_ENV_MASTER_KEY", validKeyB64)
	crypto, err := NewCrypto()
	if err != nil {
		t.Fatalf("Failed to create crypto with valid key: %v", err)
	}
	if crypto == nil {
		t.Error("Crypto is nil")
	}

	os.Unsetenv("NOT_ENV_MASTER_KEY")
}
