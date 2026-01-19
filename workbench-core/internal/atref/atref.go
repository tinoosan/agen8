package atref

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

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
	close := atQuoteClose(open)
	if close == 0 {
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

func atQuoteClose(open rune) rune {
	switch open {
	case '"':
		return '"'
	case '\'':
		return '\''
	case '“':
		return '”'
	case '‘':
		return '’'
	default:
		return 0
	}
}

// ActiveAtTokenAtEnd finds the last @token that is currently being edited at the end
// of the input and returns:
// - query: token text (without leading @ and without surrounding quotes)
// - replaceStart/replaceEnd: rune indices to replace with the selected @ref
func ActiveAtTokenAtEnd(input string) (query string, replaceStart int, replaceEnd int, ok bool) {
	rs := []rune(input)
	n := len(rs)
	if n == 0 {
		return "", 0, 0, false
	}

	for i := n - 1; i >= 0; i-- {
		if rs[i] != '@' {
			continue
		}
		// Require a token boundary before '@' to avoid matching email-like text.
		if i > 0 && !unicode.IsSpace(rs[i-1]) {
			continue
		}

		// "@": empty query.
		if i+1 >= n {
			return "", i, n, true
		}

		open := rs[i+1]
		if close := atQuoteClose(open); close != 0 {
			// Quoted token @"..."/@'...'/@“...”/@‘...’
			start := i + 2
			if start > n {
				start = n
			}
			// Find a closing quote.
			closeIdx := -1
			for j := start; j < n; j++ {
				if rs[j] == close {
					closeIdx = j
					break
				}
			}
			if closeIdx == -1 {
				// Still typing (no closing quote); active token must be at end.
				return string(rs[start:]), i, n, true
			}
			// If the quote closed before the end, treat it as not the active token.
			if closeIdx != n-1 {
				continue
			}
			return string(rs[start:closeIdx]), i, n, true
		}

		// Unquoted token: consume until whitespace.
		j := i + 1
		for j < n && !unicode.IsSpace(rs[j]) {
			j++
		}
		// Active token must reach end-of-input.
		if j != n {
			continue
		}
		return string(rs[i+1 : n]), i, n, true
	}
	return "", 0, 0, false
}

func FormatAtRef(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "@"
	}
	// Quote only when needed (spaces/tabs/newlines).
	if strings.ContainsAny(rel, " \t\n") {
		// Prefer a quote that doesn't appear in the path.
		if strings.Contains(rel, "'") && !strings.Contains(rel, `"`) {
			return `@"` + rel + `"`
		}
		return `@'` + rel + `'`
	}
	return "@" + rel
}

