package store

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/config"
	"github.com/tinoosan/agen8/pkg/fsutil"
	"github.com/tinoosan/agen8/pkg/validate"
)

// DiskMemoryStore implements the daily memory file layout:
//
//	data/memory/
//	  MEMORY.MD                (master instructions, read-only to agent)
//	  YYYY-MM-DD-memory.md     (daily files; only today is writable)
//
// It intentionally removes the old staging/update/commits pattern.
type DiskMemoryStore struct {
	DiskStore
}

var ErrMemoryWriteOnlyToday = errors.New("can only write to today's memory file")

// NewDiskMemoryStore constructs a DiskMemoryStore under cfg.DataDir.
func NewDiskMemoryStore(cfg config.Config) (*DiskMemoryStore, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	baseDir := fsutil.GetMemoryDir(cfg.DataDir)
	return NewDiskMemoryStoreFromDir(baseDir)
}

// NewDiskMemoryStoreFromDir constructs a DiskMemoryStore rooted at baseDir.
func NewDiskMemoryStoreFromDir(baseDir string) (*DiskMemoryStore, error) {
	if err := validate.NonEmpty("baseDir", baseDir); err != nil {
		return nil, err
	}
	s := &DiskMemoryStore{
		DiskStore: DiskStore{Dir: baseDir},
	}
	if err := s.ensure(); err != nil {
		return nil, err
	}
	return s, nil
}

// BaseDir returns the on-disk directory containing memory files.
func (s *DiskMemoryStore) BaseDir() string {
	if s == nil {
		return ""
	}
	return s.Dir
}

// ReadMemory returns the memory contents for the given date (format YYYY-MM-DD).
// If date is empty, it defaults to today (local time).
func (s *DiskMemoryStore) ReadMemory(ctx context.Context, date string) (string, error) {
	path, err := s.dailyPath(date)
	if err != nil {
		return "", err
	}
	if err := s.ensureDaily(path); err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// WriteMemory replaces the memory file for the given date.
func (s *DiskMemoryStore) WriteMemory(ctx context.Context, date string, text string) error {
	path, err := s.dailyPath(date)
	if err != nil {
		return err
	}
	if err := s.ensureTodayWritable(path); err != nil {
		return err
	}
	if err := s.ensure(); err != nil {
		return err
	}
	if err := fsutil.WriteFileAtomic(path, []byte(text), 0644); err != nil {
		return err
	}
	return nil
}

// AppendMemory appends text to the memory file for the given date.
func (s *DiskMemoryStore) AppendMemory(ctx context.Context, date string, text string) error {
	path, err := s.dailyPath(date)
	if err != nil {
		return err
	}
	if err := s.ensureTodayWritable(path); err != nil {
		return err
	}
	if err := s.ensureDaily(path); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		return err
	}
	return nil
}

// ListMemoryFiles lists all files under the memory directory.
func (s *DiskMemoryStore) ListMemoryFiles(ctx context.Context) ([]string, error) {
	if err := s.ensure(); err != nil {
		return nil, err
	}
	des, err := os.ReadDir(s.Dir)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(des))
	for _, de := range des {
		if de.IsDir() {
			continue
		}
		out = append(out, de.Name())
	}
	sort.Strings(out)
	return out, nil
}

// ensure guarantees the base directory and master instructions file exist.
func (s *DiskMemoryStore) ensure() error {
	if s == nil {
		return fmt.Errorf("disk memory store is nil")
	}
	if err := validate.NonEmpty("memory dir", s.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(s.Dir, 0755); err != nil {
		return err
	}

	// Ensure master instructions file.
	masterPath := filepath.Join(s.Dir, "MEMORY.MD")
	if _, err := os.Stat(masterPath); err != nil {
		if err := os.WriteFile(masterPath, []byte(defaultMemoryMaster()), 0644); err != nil {
			return fmt.Errorf("write master memory file: %w", err)
		}
	}

	// Ensure today's file exists (empty is fine).
	today := time.Now().Format("2006-01-02") + "-memory.md"
	return s.ensureDaily(filepath.Join(s.Dir, today))
}

func (s *DiskMemoryStore) ensureDaily(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return os.WriteFile(path, []byte{}, 0644)
		}
		return err
	}
	return nil
}

func (s *DiskMemoryStore) dailyPath(date string) (string, error) {
	d := strings.TrimSpace(date)
	if d == "" {
		d = time.Now().Format("2006-01-02")
	}
	if _, err := time.Parse("2006-01-02", d); err != nil {
		return "", fmt.Errorf("invalid memory date %q: %w", date, err)
	}
	filename := fmt.Sprintf("%s-memory.md", d)
	return filepath.Join(s.Dir, filename), nil
}

func (s *DiskMemoryStore) ensureTodayWritable(path string) error {
	base := filepath.Base(strings.TrimSpace(path))
	today := time.Now().Format("2006-01-02") + "-memory.md"
	if base != today {
		return fmt.Errorf("%w: %s", ErrMemoryWriteOnlyToday, today)
	}
	return nil
}

func defaultMemoryMaster() string {
	return `# Agen8 Memory Master

Daily memory files keep operational notes the agent writes directly.

Principles
- Bias to action: write when in doubt; small, frequent notes beat none.
- Only today is writable; past days are immutable.
- Keep entries factual and concise; no conversational tone.

Format (append to today's file)
- Timestamp: HH:MM (24h)
- Category: [preference|correction|decision|pattern|context]
- Entry: Brief description with WHY, not just WHAT.

Examples
- 09:10 | preference | User prefers \"rg\" over \"grep\" for searches.
- 11:25 | correction | API base URL is https://api.acme.test, not prod.
- 14:40 | decision   | Adopted daily memory files; staging removed.

Privacy
- Never store secrets, tokens, or credentials.
- Summarize sensitive data rather than copying raw values.
`
}
