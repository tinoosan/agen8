package resources

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

var (
	rgOnce      sync.Once
	rgAvailable bool
)

func hasRipgrep() bool {
	rgOnce.Do(func() {
		_, err := exec.LookPath("rg")
		rgAvailable = err == nil
	})
	return rgAvailable
}

type rgFilesWithMatchesOpts struct {
	Dir         string
	Query       string
	RegexOK     bool
	MaxFilesize string
	Globs       []string
	MaxFiles    int
}

func rgFilesWithMatches(ctx context.Context, opts rgFilesWithMatchesOpts) ([]string, bool, error) {
	if !hasRipgrep() {
		return nil, false, nil
	}
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		return nil, true, fmt.Errorf("rg dir is required")
	}
	query := strings.TrimSpace(opts.Query)
	if query == "" {
		return nil, true, fmt.Errorf("rg query is required")
	}
	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 1000
	}

	args := []string{
		"--color=never",
		"--no-messages",
		"--no-ignore",
		"--hidden",
		"--files-with-matches",
	}
	if strings.TrimSpace(opts.MaxFilesize) != "" {
		args = append(args, "--max-filesize", opts.MaxFilesize)
	}
	for _, g := range opts.Globs {
		g = strings.TrimSpace(g)
		if g == "" {
			continue
		}
		args = append(args, "--glob", g)
	}
	if !opts.RegexOK {
		// Fixed-string fallback should match our non-regex path (case-insensitive substring).
		args = append(args, "-F", "--ignore-case")
	}
	args = append(args, query, ".")

	cmd := exec.CommandContext(ctx, "rg", args...)
	cmd.Dir = dir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			// ripgrep uses exit code 1 for "no matches".
			if ee.ExitCode() == 1 {
				return nil, true, nil
			}
		}
		// Any other error: treat as non-fatal and allow caller to fallback.
		return nil, true, fmt.Errorf("rg failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	outLines := strings.Split(stdout.String(), "\n")
	files := make([]string, 0, min(maxFiles, len(outLines)))
	seen := map[string]struct{}{}
	for _, ln := range outLines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		if _, ok := seen[ln]; ok {
			continue
		}
		seen[ln] = struct{}{}
		files = append(files, ln)
		if len(files) >= maxFiles {
			break
		}
	}
	return files, true, nil
}

type bestMatch struct {
	score float64
	line  int
	text  string
}

func compileSearchQuery(query string) (*regexp.Regexp, bool, string) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, false, ""
	}
	re, err := regexp.Compile(query)
	if err != nil {
		return nil, false, strings.ToLower(query)
	}
	return re, true, strings.ToLower(query)
}
