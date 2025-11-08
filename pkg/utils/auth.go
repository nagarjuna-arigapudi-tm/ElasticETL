package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"strings"
)

// Default encryption key - in production, this should be configurable
const defaultEncryptionKey = "elasticetl-default-key-32-chars"

// ProcessBasicAuthPassword processes the password based on the password type
func ProcessBasicAuthPassword(password, passwordType, passkey string) (string, error) {
	// Default to PLAIN_TEXT if password_type is not specified
	if passwordType == "" {
		passwordType = "PLAIN_TEXT"
	}

	switch strings.ToUpper(passwordType) {
	case "PLAIN_TEXT":
		return password, nil

	case "PLAIN_TEXT_BASE64":
		return decodeBase64(password)

	case "ENCRYPTED":
		key := getEncryptionKey(passkey)
		return decryptAES(password, key)

	case "ENCRYPTED_BASE64":
		decoded, err := decodeBase64(password)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64: %w", err)
		}
		key := getEncryptionKey(passkey)
		return decryptAES(decoded, key)

	case "ENV_VAR":
		return getEnvVar(password)

	case "ENV_VAR_BASE64":
		envValue, err := getEnvVar(password)
		if err != nil {
			return "", err
		}
		return decodeBase64(envValue)

	case "ENV_VAR_ENCRYPTED":
		envValue, err := getEnvVar(password)
		if err != nil {
			return "", err
		}
		key := getEncryptionKey(passkey)
		return decryptAES(envValue, key)

	case "ENV_VAR_ENCRYPTED_BASE64":
		envValue, err := getEnvVar(password)
		if err != nil {
			return "", err
		}
		decoded, err := decodeBase64(envValue)
		if err != nil {
			return "", fmt.Errorf("failed to decode base64 from env var: %w", err)
		}
		key := getEncryptionKey(passkey)
		return decryptAES(decoded, key)

	default:
		return "", fmt.Errorf("unsupported password type: %s", passwordType)
	}
}

// decodeBase64 decodes a base64 encoded string
func decodeBase64(encoded string) (string, error) {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64: %w", err)
	}
	return string(decoded), nil
}

// getEnvVar retrieves an environment variable
func getEnvVar(varName string) (string, error) {
	value := os.Getenv(varName)
	if value == "" {
		return "", fmt.Errorf("environment variable %s is not set or empty", varName)
	}
	return value, nil
}

// getEncryptionKey returns the encryption key, using default if passkey is empty
func getEncryptionKey(passkey string) string {
	if passkey == "" {
		return defaultEncryptionKey
	}
	// Ensure key is exactly 32 bytes for AES-256
	if len(passkey) < 32 {
		// Pad with default key if too short
		padded := passkey + defaultEncryptionKey
		return padded[:32]
	}
	return passkey[:32]
}

// encryptAES encrypts plaintext using AES-GCM
func EncryptAES(plaintext, key string) (string, error) {
	keyBytes := []byte(key)
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("key must be exactly 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptAES decrypts ciphertext using AES-GCM
func decryptAES(ciphertext, key string) (string, error) {
	keyBytes := []byte(key)
	if len(keyBytes) != 32 {
		return "", fmt.Errorf("key must be exactly 32 bytes for AES-256")
	}

	// Decode base64 ciphertext
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode base64 ciphertext: %w", err)
	}

	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, cipherData := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}

	return string(plaintext), nil
}

// CreateBasicAuthHeader creates a basic authentication header value
func CreateBasicAuthHeader(username, password string) string {
	auth := username + ":" + password
	encoded := base64.StdEncoding.EncodeToString([]byte(auth))
	return "Basic " + encoded
}
