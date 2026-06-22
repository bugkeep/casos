package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"

	"github.com/beego/beego"
)

func managedNodeSecretKey() string {
	return beego.AppConfig.String("managedNodeSecretKey")
}

func EncryptManagedNodePassword(password string) (string, error) {
	key := managedNodeSecretKey()
	if key == "" {
		return "", fmt.Errorf("managedNodeSecretKey not set in app.conf; configure it before using managed node deployment")
	}
	block, err := aes.NewCipher(normalizeAESKey(key))
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
	ciphertext := gcm.Seal(nonce, nonce, []byte(password), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func DecryptManagedNodePassword(ciphertext string) (string, error) {
	key := managedNodeSecretKey()
	if key == "" {
		return "", fmt.Errorf("managedNodeSecretKey not set in app.conf; configure it before using managed node deployment")
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(normalizeAESKey(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	nonce, data := raw[:nonceSize], raw[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, data, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}

func normalizeAESKey(key string) []byte {
	buf := make([]byte, 32)
	copy(buf, []byte(key))
	return buf
}
