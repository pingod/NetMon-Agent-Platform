// Package crypto AES-256-GCM 加密/解密，与 TS encryptionService.ts 完全兼容
//
// 格式: v2 = "2:base64(iv):base64(authTag):base64(ciphertext)"
// 密钥派生: PBKDF2(secret, "encryption-service-v2-salt", 100000, 32, sha512)
// IV 长度: 16 字节（与 TS 一致，非标准 GCM 12 字节）
//
// secret 优先取 ENCRYPTION_KEY（长度需 >= 32 字节）；未设置时回退 JWT_SECRET，
// 从而兼容存量密文。
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/pbkdf2"

	"itops-backend/internal/config"
	"itops-backend/internal/logger"
)

const (
	keyVersionCurrent = 2
	pbkdf2Iterations  = 100000
	keyLength         = 32 // AES-256
	ivLength          = 16 // 与 TS IV_LENGTH 一致
	salt              = "encryption-service-v2-salt"
	minKeySecretLen   = 32
)

var (
	initOnce      sync.Once
	initErr       error
	encryptionKey []byte
)

func deriveKey(secret string) []byte {
	return pbkdf2.Key([]byte(secret), []byte(salt), pbkdf2Iterations, keyLength, sha512.New)
}

// initKey 根据配置派生密钥（仅执行一次）。
//
// 优先使用 config.Env（main 启动时已加载）；在单元测试 / 独立工具中 config 可能未加载，
// 此时回退读取环境变量；仍为空则生成本次进程有效的随机密钥，避免把“配置缺失”
// 静默变成空口令落库（调用方会拿到明确的加密失败或本次进程内的可用密文）。
func initKey() error {
	var encKey, jwtSecret string
	if config.Env != nil {
		encKey = config.Env.EncryptionKey
		jwtSecret = config.Env.JWTSecret
	} else {
		encKey = os.Getenv("ENCRYPTION_KEY")
		jwtSecret = os.Getenv("JWT_SECRET")
	}

	if encKey != "" {
		if len(encKey) < minKeySecretLen {
			return fmt.Errorf("ENCRYPTION_KEY must be at least %d bytes", minKeySecretLen)
		}
		encryptionKey = deriveKey(encKey)
		return nil
	}

	if jwtSecret == "" {
		b := make([]byte, 32)
		if _, err := rand.Read(b); err != nil {
			return fmt.Errorf("generate ephemeral encryption key: %w", err)
		}
		logger.Warnf("Neither ENCRYPTION_KEY nor JWT_SECRET is available; using an ephemeral key (existing ciphertext cannot be decrypted)")
		encryptionKey = deriveKey(hex.EncodeToString(b))
		return nil
	}
	logger.Warnf("ENCRYPTION_KEY not set, deriving encryption key from JWT_SECRET (compatibility mode)")
	encryptionKey = deriveKey(jwtSecret)
	return nil
}

// Init 从 ENCRYPTION_KEY（回退 JWT_SECRET）派生加密密钥，幂等。
func Init() error {
	initOnce.Do(func() {
		initErr = initKey()
	})
	return initErr
}

// Encrypt 加密明文，返回 "2:iv:authTag:ciphertext" 格式。
// 失败时返回错误，调用方不得把空字符串写入数据库。
func Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}
	if err := Init(); err != nil {
		return "", err
	}

	block, err := aes.NewCipher(encryptionKey)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}
	// 使用 16 字节 nonce（与 TS 一致）
	gcm, err := cipher.NewGCMWithNonceSize(block, ivLength)
	if err != nil {
		return "", fmt.Errorf("create gcm: %w", err)
	}

	iv := make([]byte, ivLength)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate iv: %w", err)
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
	), nil
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
	if err := Init(); err != nil {
		return "", err
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
