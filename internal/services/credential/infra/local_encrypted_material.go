package infra

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	keyDirName  = "credentials"
	keyFileName = "local_encrypted.key"
	keySize     = 32
	nonceSize   = 12
)

func encryptLocal(dataDir string, plaintext []byte) ([]byte, error) {
	key, err := localDataKey(dataDir)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("credential material cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credential material gcm: %w", err)
	}
	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("credential material nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, len(nonce)+len(ciphertext))
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

func decryptLocal(dataDir string, payload []byte) ([]byte, error) {
	if len(payload) < nonceSize {
		return nil, fmt.Errorf("credential material payload is invalid")
	}
	key, err := localDataKey(dataDir)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("credential material cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("credential material gcm: %w", err)
	}
	plaintext, err := gcm.Open(nil, payload[:nonceSize], payload[nonceSize:], nil)
	if err != nil {
		return nil, fmt.Errorf("credential material decrypt: %w", err)
	}
	return plaintext, nil
}

func localDataKey(dataDir string) ([]byte, error) {
	dir := filepath.Join(dataDir, keyDirName)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("credential key dir: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("credential key dir permissions: %w", err)
	}
	path := filepath.Join(dir, keyFileName)
	key, err := os.ReadFile(path)
	if err == nil {
		if len(key) != keySize {
			return nil, fmt.Errorf("credential data key has invalid length")
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return nil, fmt.Errorf("credential key file permissions: %w", err)
		}
		return key, nil
	}
	if !os.IsNotExist(err) {
		return nil, fmt.Errorf("credential key file read: %w", err)
	}
	key = make([]byte, keySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("credential data key generate: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, fmt.Errorf("credential key file create: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(key); err != nil {
		return nil, fmt.Errorf("credential key file write: %w", err)
	}
	return key, nil
}
