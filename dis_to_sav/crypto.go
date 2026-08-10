package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
)

// encryptAESGCM replicates gitdev.devops.krungthai.com/payment/common/pkg.git/crypto's
// Service.Encrypt exactly: AES-256-GCM, random nonce prepended to the ciphertext,
// base64-standard-encoded. This must stay byte-for-byte compatible with that
// implementation, or the savedb-consumer service won't be able to decrypt what this
// tool inserts.
//
// base64Key is the same base64-encoded 32-byte key the service reads from
// crypto.encryption_key in its config — pass the value for whichever environment
// the generated SQL will be run against (SIT/UAT), never a real PRD key.
func encryptAESGCM(plaintext string, base64Key string) (string, error) {
	if len(strings.TrimSpace(plaintext)) == 0 {
		return "", nil
	}
	if len(strings.TrimSpace(base64Key)) == 0 {
		return "", fmt.Errorf("encryption_key is empty — set it in config.json to the target environment's crypto.encryption_key")
	}

	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return "", fmt.Errorf("invalid encryption_key (not valid base64): %w", err)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("invalid encryption_key size: expected 32 bytes, got %d", len(key))
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
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// decryptAESGCM is the inverse of encryptAESGCM — kept only to self-verify this
// file's encryption during development/testing (e.g. a throwaway round-trip check).
// Not wired into any command.
func decryptAESGCM(ciphertext string, base64Key string) (string, error) {
	if len(strings.TrimSpace(ciphertext)) == 0 {
		return "", nil
	}
	key, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return "", fmt.Errorf("invalid encryption_key (not valid base64): %w", err)
	}
	if len(key) != 32 {
		return "", fmt.Errorf("invalid encryption_key size: expected 32 bytes, got %d", len(key))
	}
	ciphertextByte, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(ciphertextByte) < gcm.NonceSize() {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, ciphertextByte[:gcm.NonceSize()], ciphertextByte[gcm.NonceSize():], nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
