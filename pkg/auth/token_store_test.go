package auth

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestFileTokenStore_SaveLoadDelete(t *testing.T) {
	store := NewFileTokenStore(t.TempDir())
	rec := OAuthTokenRecord{
		AccessToken:   "acc",
		RefreshToken:  "ref",
		ExpiresAtUnix: time.Now().Add(time.Hour).UnixMilli(),
		AccountID:     "acct_123",
	}
	if err := store.Save(rec); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AccessToken != "acc" || loaded.RefreshToken != "ref" || loaded.AccountID != "acct_123" {
		t.Fatalf("unexpected loaded record: %+v", loaded)
	}
	info, err := os.Stat(store.Path())
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o want 600", info.Mode().Perm())
	}
	if err := store.Delete(); err != nil {
		t.Fatalf("delete: %v", err)
	}
	_, err = store.Load()
	if !errors.Is(err, ErrTokenNotFound) {
		t.Fatalf("want ErrTokenNotFound, got %v", err)
	}
}

func TestFileTokenStore_CorruptedJSON(t *testing.T) {
	store := NewFileTokenStore(t.TempDir())
	if err := os.MkdirAll(filepath.Dir(store.Path()), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(store.Path(), []byte("{bad json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := store.Load()
	if err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestFileTokenStore_ConcurrentSaveMaintainsReadableState(t *testing.T) {
	store := NewFileTokenStore(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			_ = store.Save(OAuthTokenRecord{
				AccessToken:   "acc",
				RefreshToken:  "ref",
				ExpiresAtUnix: time.Now().Add(time.Hour).UnixMilli(),
				AccountID:     "acct_" + string(rune('a'+i)),
			})
		}()
	}
	wg.Wait()
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.AccessToken == "" || loaded.RefreshToken == "" || loaded.AccountID == "" {
		t.Fatalf("unexpected loaded record after concurrent writes: %+v", loaded)
	}
}
