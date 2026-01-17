package app

import (
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/vfs"
	"github.com/tinoosan/workbench-core/internal/vfsutil"
)

type RefResolution struct {
	Attachments []agent.FileAttachment

	// AttachedSummaries are short "Attached: ..." strings suitable for UI events.
	AttachedSummaries []string

	// Unresolved contains tokens that could not be resolved.
	Unresolved []string

	// Ambiguous maps token -> candidate rel paths under the workdir.
	Ambiguous map[string][]string
}

// ExtractAtRefs finds @tokens inside free-form user input.
//
// Supported forms:
//   - Unquoted: @path/to/file.txt
//   - Quoted:   @"my file.md"  or @'my file.md'
//   - Smart quotes: @“my file.md” or @‘my file.md’
//
// Unquoted tokens are conservative: "@<path-like>" where <path-like> contains only
// letters/digits plus "._-/".
func ExtractAtRefs(userText string) []string {
	userText = strings.ReplaceAll(userText, "\r", "")
	out := make([]string, 0)
	seen := map[string]bool{}

	isTokChar := func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z':
			return true
		case r >= 'A' && r <= 'Z':
			return true
		case r >= '0' && r <= '9':
			return true
		case strings.ContainsRune("._-/", r):
			return true
		default:
			return false
		}
	}

	for i := 0; i < len(userText); i++ {
		if userText[i] != '@' {
			continue
		}
		j := i + 1
		if j >= len(userText) {
			continue
		}

		// Quoted token: @"..." / @'...' / @“...” / @‘...’
		if tok, end, ok := consumeAtQuotedToken(userText, j); ok {
			if tok != "" && !seen[tok] {
				seen[tok] = true
				out = append(out, tok)
			}
			i = end - 1
			continue
		}

		// Unquoted token.
		for j < len(userText) {
			r := rune(userText[j])
			if !isTokChar(r) {
				break
			}
			j++
		}
		if j == i+1 {
			continue
		}
		tok := strings.TrimSpace(userText[i+1 : j])
		if tok == "" {
			continue
		}
		if !seen[tok] {
			seen[tok] = true
			out = append(out, tok)
		}
		i = j - 1
	}
	return out
}

func consumeAtQuotedToken(s string, start int) (tok string, end int, ok bool) {
	if start >= len(s) {
		return "", start, false
	}
	open, openSize := utf8.DecodeRuneInString(s[start:])
	if open == utf8.RuneError && openSize == 1 {
		return "", start, false
	}
	close := rune(0)
	switch open {
	case '"':
		close = '"'
	case '\'':
		close = '\''
	case '“':
		close = '”'
	case '‘':
		close = '’'
	default:
		return "", start, false
	}
	// Allow optional @<quote> with no whitespace. start points at the quote.
	i := start + openSize
	for i < len(s) {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			i++
			continue
		}
		if r == close {
			raw := s[start+openSize : i]
			return strings.TrimSpace(raw), i + size, true
		}
		i += size
	}
	// No closing quote; treat as not-a-token.
	return "", start, false
}

// ResolveAtRefs resolves @tokens to bounded file attachments.
//
// Resolution order:
//  1. exact path under /workdir
//  2. artifact index match (e.g. recently written /workspace file)
//  3. fuzzy file search under workdir (auto-select only if unambiguous)
func ResolveAtRefs(fsys *vfs.FS, workdirBase string, artifacts *ArtifactIndex, userText string, maxFiles, maxBytesTotal, maxBytesPerFile int) (RefResolution, error) {
	if fsys == nil {
		return RefResolution{}, fmt.Errorf("fs is required")
	}
	tokens := ExtractAtRefs(userText)
	if len(tokens) == 0 {
		return RefResolution{}, nil
	}
	if maxFiles <= 0 {
		maxFiles = 6
	}
	if maxBytesTotal <= 0 {
		maxBytesTotal = 48 * 1024
	}
	if maxBytesPerFile <= 0 {
		maxBytesPerFile = 12 * 1024
	}

	var res RefResolution
	res.Ambiguous = make(map[string][]string)

	usedBytes := 0
	for _, tok := range tokens {
		if len(res.Attachments) >= maxFiles || usedBytes >= maxBytesTotal {
			break
		}

		att, ok, amb, err := resolveOneRef(fsys, workdirBase, artifacts, tok, maxBytesPerFile)
		if err != nil {
			return RefResolution{}, err
		}
		if ok {
			if usedBytes+att.BytesIncluded > maxBytesTotal {
				// Skip if it would exceed total budget.
				continue
			}
			usedBytes += att.BytesIncluded
			res.Attachments = append(res.Attachments, att)
			if att.DisplayName != "" {
				res.AttachedSummaries = append(res.AttachedSummaries, fmt.Sprintf("%s (%dB)", att.DisplayName, att.BytesIncluded))
			} else {
				res.AttachedSummaries = append(res.AttachedSummaries, fmt.Sprintf("%s (%dB)", att.VPath, att.BytesIncluded))
			}
			continue
		}
		if len(amb) != 0 {
			res.Ambiguous[tok] = amb
			continue
		}
		res.Unresolved = append(res.Unresolved, tok)
	}
	return res, nil
}

func resolveOneRef(fsys *vfs.FS, workdirBase string, artifacts *ArtifactIndex, token string, maxBytes int) (agent.FileAttachment, bool, []string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return agent.FileAttachment{}, false, nil, nil
	}
	if strings.HasPrefix(token, "/") {
		return agent.FileAttachment{}, false, nil, nil
	}

	if strings.EqualFold(token, "last") {
		if vp, ok := artifacts.Resolve("last"); ok {
			return readAttachment(fsys, token, vp, displayNameForVPath(vp), maxBytes)
		}
		return agent.FileAttachment{}, false, nil, nil
	}

	if clean, _, err := vfsutil.NormalizeResourceSubpath(token); err == nil && clean != "" && clean != "." {
		// 1) exact under /workdir
		vp := "/workdir/" + clean
		if att, ok, err := tryReadVPath(fsys, token, vp, clean, maxBytes); err != nil {
			return agent.FileAttachment{}, false, nil, err
		} else if ok {
			return att, true, nil, nil
		}
	}

	// 2) artifact index (scratch outputs).
	if vp, ok := artifacts.Resolve(token); ok {
		return readAttachment(fsys, token, vp, displayNameForVPath(vp), maxBytes)
	}

	// 3) fuzzy search under workdir.
	cands, err := fuzzyFindWorkdir(workdirBase, token, 40, 5000)
	if err != nil {
		return agent.FileAttachment{}, false, nil, nil
	}
	if len(cands) == 1 {
		rel := cands[0]
		vp := "/workdir/" + rel
		if att, ok, err := tryReadVPath(fsys, token, vp, rel, maxBytes); err != nil {
			return agent.FileAttachment{}, false, nil, err
		} else if ok {
			return att, true, nil, nil
		}
	}
	if len(cands) > 1 {
		return agent.FileAttachment{}, false, cands, nil
	}

	return agent.FileAttachment{}, false, nil, nil
}

func tryReadVPath(fsys *vfs.FS, token, vpath, display string, maxBytes int) (agent.FileAttachment, bool, error) {
	b, err := fsys.Read(vpath)
	if err != nil {
		return agent.FileAttachment{}, false, nil
	}
	return buildAttachment(token, vpath, display, b, maxBytes), true, nil
}

func readAttachment(fsys *vfs.FS, token, vpath, display string, maxBytes int) (agent.FileAttachment, bool, []string, error) {
	b, err := fsys.Read(vpath)
	if err != nil {
		return agent.FileAttachment{}, false, nil, nil
	}
	att := buildAttachment(token, vpath, display, b, maxBytes)
	return att, true, nil, nil
}

func buildAttachment(token, vpath, display string, full []byte, maxBytes int) agent.FileAttachment {
	if maxBytes <= 0 {
		maxBytes = 12 * 1024
	}
	incl := full
	tr := false
	if len(incl) > maxBytes {
		incl = incl[:maxBytes]
		tr = true
	}
	return agent.FileAttachment{
		Token:         token,
		VPath:         vpath,
		DisplayName:   display,
		Content:       string(incl),
		BytesTotal:    len(full),
		BytesIncluded: len(incl),
		Truncated:     tr,
	}
}

func displayNameForVPath(vpath string) string {
	vpath = strings.TrimSpace(vpath)
	switch {
	case strings.HasPrefix(vpath, "/workdir/"):
		return strings.TrimPrefix(vpath, "/workdir/")
	case strings.HasPrefix(vpath, "/workspace/"):
		return strings.TrimPrefix(vpath, "/workspace/")
	case strings.HasPrefix(vpath, "/results/"):
		return strings.TrimPrefix(vpath, "/results/")
	default:
		return vpath
	}
}

func fuzzyFindWorkdir(baseDir string, token string, maxCandidates int, maxVisited int) ([]string, error) {
	baseDir = strings.TrimSpace(baseDir)
	token = strings.TrimSpace(token)
	if baseDir == "" || token == "" {
		return nil, nil
	}
	if strings.HasPrefix(token, "/") {
		return nil, nil
	}
	if _, _, err := vfsutil.NormalizeResourceSubpath(token); err != nil {
		// Treat invalid paths as non-resolvable; do not walk.
		return nil, nil
	}

	wantBase := strings.Contains(token, "/") == false
	want := token
	if wantBase {
		want = path.Base(token)
	}

	cands := make([]string, 0, 8)
	visited := 0

	err := filepath.WalkDir(baseDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories to keep search bounded and less noisy.
			if p != baseDir && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		visited++
		if maxVisited > 0 && visited > maxVisited {
			return fs.SkipAll
		}

		rel, err := filepath.Rel(baseDir, p)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)

		if wantBase {
			if path.Base(rel) == want {
				cands = append(cands, rel)
			}
		} else {
			if strings.HasSuffix(rel, want) {
				cands = append(cands, rel)
			}
		}
		if maxCandidates > 0 && len(cands) >= maxCandidates {
			return fs.SkipAll
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(cands)
	return cands, nil
}
