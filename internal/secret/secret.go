// Package secret 提供对称加密能力，用于保护数据库中存储的 OCI API 私钥。
// 采用 AES-256-GCM，密钥由 config 统一提供。
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

// Sealer 持有一个 GCM 实例，可并发使用。
type Sealer struct {
	gcm cipher.AEAD
}

// NewSealer 用 32 字节密钥构造 Sealer。
func NewSealer(key []byte) (*Sealer, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return &Sealer{gcm: gcm}, nil
}

// Encrypt 加密明文并返回 base64 编码的密文（nonce 前置）。
func (s *Sealer) Encrypt(plaintext string) (string, error) {
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := s.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 Encrypt 产生的密文。空字符串直接返回空值，方便处理可选字段。
func (s *Sealer) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", errors.New("密文不是合法的 base64")
	}
	if len(raw) < s.gcm.NonceSize() {
		return "", errors.New("密文长度不足")
	}
	nonce, body := raw[:s.gcm.NonceSize()], raw[s.gcm.NonceSize():]
	plaintext, err := s.gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", errors.New("解密失败，加密密钥可能已变更")
	}
	return string(plaintext), nil
}
