package resources

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/tinoosan/agen8/pkg/types"
)

const defaultSearchMaxSizeBytes int64 = 2 * 1024 * 1024

type compiledSearchRequest struct {
	query           string
	pattern         string
	limit           int
	includeGlob     string
	excludeGlobs    []string
	previewLines    int
	includeMetadata bool
	maxSizeBytes    int64
	regex           *regexp.Regexp
	queryLower      string
	includeMatcher  *regexp.Regexp
	excludeMatchers []*regexp.Regexp
}

type rankedSearchResult struct {
	result types.SearchResult
}

func compileSearchRequest(req types.SearchRequest) (compiledSearchRequest, error) {
	spec := compiledSearchRequest{
		query:           strings.TrimSpace(req.Query),
		pattern:         strings.TrimSpace(req.Pattern),
		limit:           req.Limit,
		includeGlob:     strings.TrimSpace(req.Glob),
		excludeGlobs:    cloneTrimmedStrings(req.Exclude),
		previewLines:    req.PreviewLines,
		includeMetadata: req.IncludeMetadata,
		maxSizeBytes:    req.MaxSizeBytes,
	}
	if spec.limit <= 0 {
		spec.limit = 5
	}
	if spec.maxSizeBytes <= 0 {
		spec.maxSizeBytes = defaultSearchMaxSizeBytes
	}
	if spec.pattern != "" {
		re, err := regexp.Compile(spec.pattern)
		if err != nil {
			return compiledSearchRequest{}, fmt.Errorf("invalid pattern: %w", err)
		}
		spec.regex = re
	} else {
		spec.queryLower = strings.ToLower(spec.query)
	}
	if spec.includeGlob != "" {
		matcher, err := globMatcher(spec.includeGlob)
		if err != nil {
			return compiledSearchRequest{}, fmt.Errorf("invalid glob %q: %w", spec.includeGlob, err)
		}
		spec.includeMatcher = matcher
	}
	if len(spec.excludeGlobs) != 0 {
		spec.excludeMatchers = make([]*regexp.Regexp, 0, len(spec.excludeGlobs))
		for _, glob := range spec.excludeGlobs {
			matcher, err := globMatcher(glob)
			if err != nil {
				return compiledSearchRequest{}, fmt.Errorf("invalid exclude glob %q: %w", glob, err)
			}
			spec.excludeMatchers = append(spec.excludeMatchers, matcher)
		}
	}
	return spec, nil
}

func (s compiledSearchRequest) effectiveQuery() string {
	if s.pattern != "" {
		return s.pattern
	}
	return s.query
}

func (s compiledSearchRequest) usesRegex() bool {
	return s.regex != nil
}

func (s compiledSearchRequest) rgMaxFileSize() string {
	return strconv.FormatInt(s.maxSizeBytes, 10)
}

func (s compiledSearchRequest) matchLine(line string) (bool, int) {
	if s.regex != nil {
		idxs := s.regex.FindAllStringIndex(line, -1)
		if len(idxs) == 0 {
			return false, 0
		}
		return true, len(idxs)
	}
	if s.queryLower == "" {
		return false, 0
	}
	count := strings.Count(strings.ToLower(line), s.queryLower)
	if count == 0 {
		return false, 0
	}
	return true, count
}

func (s compiledSearchRequest) allowRelativePath(rel string) bool {
	rel = normalizeSearchPath(rel)
	if rel == "" {
		return false
	}
	if s.includeMatcher != nil && !s.includeMatcher.MatchString(rel) {
		return false
	}
	for _, matcher := range s.excludeMatchers {
		if matcher.MatchString(rel) {
			return false
		}
	}
	return true
}

func (s compiledSearchRequest) shouldSkipSize(size int64) bool {
	return s.maxSizeBytes > 0 && size > s.maxSizeBytes
}

func bestMatchInFile(ctx context.Context, absPath string, spec compiledSearchRequest) (bestMatch, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return bestMatch{}, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)

	lineN := 0
	lines := make([]string, 0, 128)
	bestIndex := -1
	var best bestMatch
	for sc.Scan() {
		if ctx != nil {
			select {
			case <-ctx.Done():
				return best, ctx.Err()
			default:
			}
		}

		lineN++
		ln := sc.Text()
		lines = append(lines, ln)
		match, matchN := spec.matchLine(ln)
		if !match {
			continue
		}
		score := float64(matchN)
		if score > best.score {
			best = bestMatch{
				score: score,
				line:  lineN,
				text:  strings.TrimSpace(ln),
			}
			bestIndex = lineN - 1
		}
	}
	if err := sc.Err(); err != nil {
		return best, err
	}
	if best.score <= 0 || bestIndex < 0 {
		return best, nil
	}
	if spec.previewLines > 0 {
		start := max(0, bestIndex-spec.previewLines)
		end := min(len(lines), bestIndex+spec.previewLines+1)
		best.previewBefore = trimPreviewLines(lines[start:bestIndex])
		best.previewAfter = trimPreviewLines(lines[bestIndex+1 : end])
	}
	return best, nil
}

func buildSearchResult(title, vfsPath string, match bestMatch, absPath string, spec compiledSearchRequest) (types.SearchResult, error) {
	result := types.SearchResult{
		Title:   title,
		Path:    vfsPath,
		Snippet: fmt.Sprintf("%s:%d: %s", title, match.line, match.text),
		Score:   match.score,
	}
	if spec.previewLines > 0 {
		result.PreviewBefore = append([]string(nil), match.previewBefore...)
		result.PreviewMatch = match.text
		result.PreviewAfter = append([]string(nil), match.previewAfter...)
	}
	if spec.includeMetadata {
		info, err := os.Stat(absPath)
		if err != nil {
			return types.SearchResult{}, err
		}
		size := info.Size()
		mtime := info.ModTime().Unix()
		result.SizeBytes = &size
		result.Mtime = &mtime
	}
	return result, nil
}

func finalizeSearchResults(results []types.SearchResult, limit int) types.SearchResponse {
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score != results[j].Score {
			return results[i].Score > results[j].Score
		}
		return results[i].Path < results[j].Path
	})
	total := len(results)
	if limit <= 0 {
		limit = 5
	}
	truncated := total > limit
	if truncated {
		results = results[:limit]
	}
	return types.SearchResponse{
		Results:   results,
		Total:     total,
		Returned:  len(results),
		Truncated: truncated,
	}
}

func cloneTrimmedStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, 0, len(in))
	for _, item := range in {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = append(out, item)
	}
	return out
}

func normalizeSearchPath(p string) string {
	p = strings.TrimSpace(strings.ReplaceAll(p, "\\", "/"))
	p = strings.TrimPrefix(p, "./")
	p = strings.TrimPrefix(p, "/")
	return path.Clean("/" + p)[1:]
}

func trimPreviewLines(lines []string) []string {
	if len(lines) == 0 {
		return nil
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, strings.TrimSpace(line))
	}
	return out
}

func includeGlobs(glob string) []string {
	glob = strings.TrimSpace(glob)
	if glob == "" {
		return nil
	}
	return []string{glob}
}

func globMatcher(pattern string) (*regexp.Regexp, error) {
	pattern = normalizeSearchPath(pattern)
	if pattern == "" {
		return nil, fmt.Errorf("pattern is required")
	}
	var b strings.Builder
	b.WriteString("^")
	for i := 0; i < len(pattern); i++ {
		ch := pattern[i]
		switch ch {
		case '*':
			if i+1 < len(pattern) && pattern[i+1] == '*' {
				b.WriteString(".*")
				i++
				continue
			}
			b.WriteString(`[^/]*`)
		case '?':
			b.WriteString(`[^/]`)
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			b.WriteByte('\\')
			b.WriteByte(ch)
		default:
			b.WriteByte(ch)
		}
	}
	b.WriteString("$")
	return regexp.Compile(b.String())
}
