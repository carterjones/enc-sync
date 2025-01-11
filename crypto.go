package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"math/big"
)

func generateRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	bytes := make([]byte, length)
	for i := range bytes {
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		bytes[i] = charset[num.Int64()]
	}
	return string(bytes), nil
}

func getSHA512Hash(text string) string {
	hash := sha512.Sum512([]byte(text))
	return hex.EncodeToString(hash[:])
}

func encryptAES(plainText, key string) (string, error) {
	// Convert key and plainText to byte slices
	keyBytes := []byte(key)
	plainBytes := []byte(plainText)

	// Create a new AES cipher using the key
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	// Create a GCM (Galois/Counter Mode) instance
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Create a nonce (number used once)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// Encrypt the plainText using GCM
	cipherBytes := gcm.Seal(nonce, nonce, plainBytes, nil)

	// Return the encrypted data as a base64-encoded string
	return base64.StdEncoding.EncodeToString(cipherBytes), nil
}

func decryptAES(encryptedText, key string) (string, error) {
	// Convert key to byte slice
	keyBytes := []byte(key)

	// Decode the base64-encoded encrypted text
	cipherBytes, err := base64.StdEncoding.DecodeString(encryptedText)
	if err != nil {
		return "", err
	}

	// Create a new AES cipher using the key
	block, err := aes.NewCipher(keyBytes)
	if err != nil {
		return "", err
	}

	// Create a GCM instance
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// Separate the nonce and the actual encrypted data
	nonceSize := gcm.NonceSize()
	if len(cipherBytes) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, cipherText := cipherBytes[:nonceSize], cipherBytes[nonceSize:]

	// Decrypt the data
	plainBytes, err := gcm.Open(nil, nonce, cipherText, nil)
	if err != nil {
		return "", err
	}

	return string(plainBytes), nil
}
