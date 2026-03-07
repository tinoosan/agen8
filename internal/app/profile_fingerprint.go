package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tinoosan/agen8/pkg/config"
)

const profileFingerprintMetadataKey = "profileFingerprint"

func resolveProfileFingerprint(cfg config.Config, requested string) (string, error) {
	_, profileDir, err := resolveProfileRef(cfg, strings.TrimSpace(requested))
	if err != nil {
		return "", err
	}
	return profileFingerprintForDir(profileDir)
}

func profileFingerprintForDir(profileDir string) (string, error) {
	profileDir = strings.TrimSpace(profileDir)
	if profileDir == "" {
		return "", fmt.Errorf("profile dir is required")
	}
	raw, err := os.ReadFile(filepath.Join(profileDir, "profile.yaml"))
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}
