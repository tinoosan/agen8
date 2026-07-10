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
	if err := validateCredentialKeyDir(dir); err != nil {
		return nil, err
	}
	// #nosec G302 -- this is a directory; owner execute permission is required
	// to traverse it, and group/other permissions remain fully denied.
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, fmt.Errorf("credential key dir permissions: %w", err)
	}
	key, err := readLocalDataKey(dir)
	if err == nil {
		if len(key) != keySize {
			return nil, fmt.Errorf("credential data key has invalid length")
		}
		if err := chmodLocalDataKey(dir); err != nil {
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
	file, err := createLocalDataKey(dir)
	if err != nil {
		return nil, fmt.Errorf("credential key file create: %w", err)
	}
	defer file.Close()
	if _, err := file.Write(key); err != nil {
		return nil, fmt.Errorf("credential key file write: %w", err)
	}
	return key, nil
}

func validateCredentialKeyDir(dir string) error {
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("credential key dir stat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credential key dir must not be a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("credential key dir is not a directory")
	}
	return nil
}

func readLocalDataKey(dir string) ([]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	linkInfo, err := root.Lstat(keyFileName)
	if err != nil {
		return nil, err
	}
	if linkInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("credential key file must not be a symlink")
	}
	file, err := root.Open(keyFileName)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("credential key file must be regular")
	}
	return io.ReadAll(file)
}

func createLocalDataKey(dir string) (*os.File, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return root.OpenFile(keyFileName, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
}

func chmodLocalDataKey(dir string) error {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer root.Close()
	return root.Chmod(keyFileName, 0o600)
}
