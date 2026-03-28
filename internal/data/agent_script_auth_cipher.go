package data

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
)

const scriptAuthNonceSize = 12

// scriptAuthCipher AES-256-GCM，密码字段落库加密。
type scriptAuthCipher struct {
	gcm cipher.AEAD
}

func newScriptAuthCipher(key []byte) (*scriptAuthCipher, error) {
	if len(key) != 32 {
		return nil, errors.New("agent script auth: AES key must be 32 bytes")
	}
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	return &scriptAuthCipher{gcm: g}, nil
}

func (c *scriptAuthCipher) encryptString(plain string) (string, error) {
	if c == nil {
		return "", errors.New("agent script auth: cipher nil")
	}
	nonce := make([]byte, scriptAuthNonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := c.gcm.Seal(nil, nonce, []byte(plain), nil)
	buf := append(nonce, sealed...)
	return base64.StdEncoding.EncodeToString(buf), nil
}

func (c *scriptAuthCipher) decryptString(b64 string) (string, error) {
	if c == nil {
		return "", errors.New("agent script auth: cipher nil")
	}
	raw, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	if len(raw) < scriptAuthNonceSize+1 {
		return "", errors.New("agent script auth: ciphertext too short")
	}
	nonce, ct := raw[:scriptAuthNonceSize], raw[scriptAuthNonceSize:]
	pt, err := c.gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(pt), nil
}
