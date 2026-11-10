// Package crypto AES-256-GCM 加密/解密，与 TS encryptionService.ts 完全兼容
//
// 格式: v2 = "2:base64(iv):base64(authTag):base64(ciphertext)"
// 密钥派生: PBKDF2(jwtSecret, "encryption-service-v2-salt", 100000, 32, sha512)
// IV 长度: 16 字节（与 TS 一致，非标准 GCM 12 字节）
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/pbkdf2"

	"itops-backend/internal/config"
)

const (
	keyVersionCurrent = 2
	pbkdf2Iterations  = 100000
	keyLength         = 32 // AES-256
	ivLength          = 16 // 与 TS IV_LENGTH 一致
	salt              = "encryption-service-v2-salt"
)

var encryptionKey []byte

// Init 从 JWT_SECRET 派生加密密钥
func Init() error {
	if config.Env == nil || config.Env.JWTSecret == "" {
		return errors.New("JWT_SECRET is required for encryption")
	}
	encryptionKey = pbkdf2.Key([]byte(config.Env.JWTSecret), []byte(salt), pbkdf2Iterations, keyLength, sha512.New)
	return nil
}

// Encrypt 加密明文，返回 "2:iv:authTag:ciphertext" 格式
func Encrypt(plaintext string) string {
	if plaintext == "" {
		return ""
	}
	if encryptionKey == nil {
		if err := Init(); err != nil {
			return ""
		}
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return ""
	}
	// 使用 16 字节 nonce（与 TS 一致）
	gcm, err := cipher.NewGCMWithNonceSize(block, ivLength)
	if err != nil {
		return ""
	}

	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return ""
	}

	ciphertext := gcm.Seal(nil, iv, []byte(plaintext), nil)
	// GCM Seal 返回 ciphertext||tag，tag 在末尾，长度为 gcm.Overhead()=16
	tagStart := len(ciphertext) - gcm.Overhead()
	encryptedData := ciphertext[:tagStart]
	authTag := ciphertext[tagStart:]

	return fmt.Sprintf("%d:%s:%s:%s",
		keyVersionCurrent,
		base64.StdEncoding.EncodeToString(iv),
		base64.StdEncoding.EncodeToString(authTag),
		base64.StdEncoding.EncodeToString(encryptedData),
	)
}

// Decrypt 解密 "2:iv:authTag:ciphertext" 或旧格式 "iv:authTag:ciphertext"
func Decrypt(encryptedString string) (string, error) {
	if encryptedString == "" {
		return "", nil
	}

	parts := strings.Split(encryptedString, ":")
	if len(parts) == 4 && parts[0] == fmt.Sprintf("%d", keyVersionCurrent) {
		return decryptV2(parts)
	}
	// v1 格式不支持的临时提示（需要 legacy key from DB）
	return "", errors.New("unsupported encryption format (v1 legacy key not available)")
}

func decryptV2(parts []string) (string, error) {
	if encryptionKey == nil {
		if err := Init(); err != nil {
			return "", err
		}
	}

	iv, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid iv: %w", err)
	}
	authTag, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid authTag: %w", err)
	}
	encryptedData, err := base64.StdEncoding.DecodeString(parts[3])
	if err != nil {
		return "", fmt.Errorf("invalid ciphertext: %w", err)
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCMWithNonceSize(block, ivLength)
	if err != nil {
		return "", err
	}

	// Go GCM Open 需要 ciphertext||tag 拼接
	ciphertextWithTag := append(encryptedData, authTag...)
	plaintext, err := gcm.Open(nil, iv, ciphertextWithTag, nil)
	if err != nil {
		return "", fmt.Errorf("decryption failed: %w", err)
	}
	return string(plaintext), nil
}
