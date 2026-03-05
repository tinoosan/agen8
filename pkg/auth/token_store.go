package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrTokenNotFound = errors.New("oauth token not found")

// OAuthTokenRecord is persisted locally for chatgpt_account provider.
type OAuthTokenRecord struct {
	Provider       string `json:"provider"`
	AccessToken    string `json:"access_token"`
	RefreshToken   string `json:"refresh_token"`
	ExpiresAtUnix  int64  `json:"expires_at_unix_ms"`
	AccountID      string `json:"account_id"`
	TokenType      string `json:"token_type"`
	UpdatedAtUnix  int64  `json:"updated_at_unix_ms"`
}

func (r OAuthTokenRecord) ExpiresAt() time.Time {
	if r.ExpiresAtUnix <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(r.ExpiresAtUnix).UTC()
}

func (r OAuthTokenRecord) UpdatedAt() time.Time {
	if r.UpdatedAtUnix <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(r.UpdatedAtUnix).UTC()
}

type FileTokenStore struct {
	path string
}

func NewFileTokenStore(dataDir string) *FileTokenStore {
	dataDir = strings.TrimSpace(dataDir)
	if dataDir == "" {
		dataDir = "."
	}
	p := filepath.Join(dataDir, "auth", "chatgpt_oauth.json")
	return &FileTokenStore{path: p}
}

func (s *FileTokenStore) Path() string {
	if s == nil {
		return ""
	}
	return s.path
}

func (s *FileTokenStore) Load() (OAuthTokenRecord, error) {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return OAuthTokenRecord{}, ErrTokenNotFound
	}
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return OAuthTokenRecord{}, ErrTokenNotFound
		}
		return OAuthTokenRecord{}, fmt.Errorf("read token store: %w", err)
	}
	var rec OAuthTokenRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return OAuthTokenRecord{}, fmt.Errorf("parse token store: %w", err)
	}
	return rec, nil
}

func (s *FileTokenStore) Save(rec OAuthTokenRecord) error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return fmt.Errorf("token store path is empty")
	}
	rec.Provider = ProviderChatGPTAccount
	if strings.TrimSpace(rec.TokenType) == "" {
		rec.TokenType = "Bearer"
	}
	rec.UpdatedAtUnix = time.Now().UTC().UnixMilli()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir auth dir: %w", err)
	}
	body, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal token store: %w", err)
	}
	body = append(body, '\n')

	tmp, err := os.CreateTemp(dir, ".chatgpt_oauth_*.tmp")
	if err != nil {
		return fmt.Errorf("create temp token file: %w", err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("chmod temp token file: %w", err)
	}
	if _, err := tmp.Write(body); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write temp token file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync temp token file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp token file: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace token store: %w", err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		return fmt.Errorf("chmod token store: %w", err)
	}
	return nil
}

func (s *FileTokenStore) Delete() error {
	if s == nil || strings.TrimSpace(s.path) == "" {
		return nil
	}
	err := os.Remove(s.path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove token store: %w", err)
	}
	return nil
}
