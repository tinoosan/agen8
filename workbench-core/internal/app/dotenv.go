package app

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// loadDotEnvFromDir loads environment variables from a .env file in dir.
//
// Behavior is intentionally conservative:
//   - Missing .env is not an error.
//   - Existing environment variables win (override=false semantics).
func loadDotEnvFromDir(dir string) error {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	return loadDotEnvFile(filepath.Join(dir, ".env"), false)
}

func loadDotEnvFile(path string, override bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	// Allow reasonably long lines (tokens).
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		val = strings.TrimSpace(val)

		// Strip simple inline comments for unquoted values: KEY=value # comment
		if val != "" && !strings.HasPrefix(val, "\"") && !strings.HasPrefix(val, "'") {
			if idx := strings.Index(val, " #"); idx >= 0 {
				val = strings.TrimSpace(val[:idx])
			}
		}

		// Handle quoted values.
		if len(val) >= 2 && val[0] == '"' && val[len(val)-1] == '"' {
			val = unescapeDoubleQuoted(val[1 : len(val)-1])
		} else if len(val) >= 2 && val[0] == '\'' && val[len(val)-1] == '\'' {
			val = val[1 : len(val)-1]
		}

		if !override {
			if _, exists := os.LookupEnv(key); exists {
				continue
			}
		}
		_ = os.Setenv(key, val)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read .env: %w", err)
	}
	return nil
}

func unescapeDoubleQuoted(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(s))
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !escaped {
			if c == '\\' {
				escaped = true
				continue
			}
			b.WriteByte(c)
			continue
		}
		escaped = false
		switch c {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case '\\':
			b.WriteByte('\\')
		case '"':
			b.WriteByte('"')
		default:
			// Unknown escape: keep it as-is.
			b.WriteByte(c)
		}
	}
	if escaped {
		// Trailing backslash: keep it.
		b.WriteByte('\\')
	}
	return b.String()
}
