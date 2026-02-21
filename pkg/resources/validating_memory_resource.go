package resources

import (
	"context"
	"errors"
	"fmt"
	iofs "io/fs"
	"strings"
	"time"

	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
)

type ValidatingMemoryResource struct {
	inner vfs.Resource
}

type parsedMemoryLine struct {
	clock    string
	category string
	content  string
}

var validMemoryCategories = []string{
	"preference",
	"correction",
	"decision",
	"pattern",
	"constraint",
	"blocker",
	"handoff",
	"context",
}

func NewValidatingMemoryResource(inner vfs.Resource) *ValidatingMemoryResource {
	return &ValidatingMemoryResource{inner: inner}
}

func (r *ValidatingMemoryResource) SupportsNestedList() bool {
	if r == nil || r.inner == nil {
		return false
	}
	return r.inner.SupportsNestedList()
}

func (r *ValidatingMemoryResource) List(subpath string) ([]vfs.Entry, error) {
	if r == nil || r.inner == nil {
		return nil, fmt.Errorf("memory resource is nil")
	}
	return r.inner.List(subpath)
}

func (r *ValidatingMemoryResource) Read(subpath string) ([]byte, error) {
	if r == nil || r.inner == nil {
		return nil, fmt.Errorf("memory resource is nil")
	}
	return r.inner.Read(subpath)
}

func (r *ValidatingMemoryResource) Write(subpath string, data []byte) error {
	if r == nil || r.inner == nil {
		return fmt.Errorf("memory resource is nil")
	}
	return r.inner.Write(subpath, data)
}

func (r *ValidatingMemoryResource) Append(subpath string, data []byte) error {
	if r == nil || r.inner == nil {
		return fmt.Errorf("memory resource is nil")
	}
	existing := ""
	if b, err := r.inner.Read(subpath); err == nil {
		existing = string(b)
	} else if !errors.Is(err, iofs.ErrNotExist) {
		return err
	}

	raw := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(raw, "\n")
	validLines := make([]string, 0, len(lines))
	current := existing
	for idx, ln := range lines {
		if strings.TrimSpace(ln) == "" {
			continue
		}
		entry, err := parseMemoryLine(ln)
		if err != nil {
			return fmt.Errorf("invalid memory line %d: %w", idx+1, err)
		}
		if isDuplicateMemoryLine(entry, current) {
			continue
		}
		canonical := formatMemoryLine(entry)
		validLines = append(validLines, canonical)
		if current != "" && !strings.HasSuffix(current, "\n") {
			current += "\n"
		}
		current += canonical + "\n"
	}
	if len(validLines) == 0 {
		return nil
	}
	return r.inner.Append(subpath, []byte(strings.Join(validLines, "\n")+"\n"))
}

func (r *ValidatingMemoryResource) Search(ctx context.Context, subpath string, query string, limit int) ([]types.SearchResult, error) {
	if r == nil || r.inner == nil {
		return nil, fmt.Errorf("memory resource is nil")
	}
	searchable, ok := r.inner.(vfs.Searchable)
	if !ok {
		return nil, fmt.Errorf("search not supported")
	}
	return searchable.Search(ctx, subpath, query, limit)
}

func parseMemoryLine(line string) (parsedMemoryLine, error) {
	parts := strings.SplitN(strings.TrimSpace(line), "|", 3)
	if len(parts) != 3 {
		return parsedMemoryLine{}, fmt.Errorf("memory line must match HH:MM | category | content")
	}
	clock := strings.TrimSpace(parts[0])
	if _, err := time.Parse("15:04", clock); err != nil {
		return parsedMemoryLine{}, fmt.Errorf("invalid memory timestamp %q: %w", clock, err)
	}
	category := strings.ToLower(strings.TrimSpace(parts[1]))
	if !isValidCategory(category) {
		return parsedMemoryLine{}, fmt.Errorf("invalid memory category %q; valid categories: %s", category, strings.Join(validMemoryCategories, ", "))
	}
	content := strings.TrimSpace(parts[2])
	if content == "" {
		return parsedMemoryLine{}, fmt.Errorf("memory content is required")
	}
	return parsedMemoryLine{clock: clock, category: category, content: content}, nil
}

func formatMemoryLine(line parsedMemoryLine) string {
	return fmt.Sprintf("%s | %s | %s", line.clock, line.category, strings.TrimSpace(line.content))
}

func isDuplicateMemoryLine(line parsedMemoryLine, existingContent string) bool {
	target := dedupMemoryKey(line.category, line.content)
	if target == "" {
		return false
	}
	lines := strings.Split(strings.ReplaceAll(existingContent, "\r\n", "\n"), "\n")
	for _, ln := range lines {
		parsed, err := parseMemoryLine(ln)
		if err != nil {
			continue
		}
		if dedupMemoryKey(parsed.category, parsed.content) == target {
			return true
		}
	}
	return false
}

func dedupMemoryKey(category, content string) string {
	cat := strings.ToLower(strings.TrimSpace(category))
	ct := strings.ToLower(strings.TrimSpace(content))
	if cat == "" || ct == "" {
		return ""
	}
	return cat + "|" + ct
}

func isValidCategory(candidate string) bool {
	for _, c := range validMemoryCategories {
		if candidate == c {
			return true
		}
	}
	return false
}
