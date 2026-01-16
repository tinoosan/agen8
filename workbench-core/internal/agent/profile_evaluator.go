package agent

import (
	"regexp"
	"strings"
)

// ProfileEvaluator is a deterministic host policy gate for global user profile updates.
//
// The profile store is shared across runs and sessions, so we keep acceptance rules
// intentionally strict to avoid "leaking" run-specific notes into global state.
//
// Accepted format (v0)
//
// Updates must be composed of one or more "key: value" lines (optionally prefixed by "- "):
//
//	birthday: 1994-11-27
//	timezone: America/New_York
//	- preferred_editor: vim
//
// Anything else is rejected with reason "not_profile".
type ProfileEvaluator struct {
	// MaxBytes caps the size of an update the host will consider.
	MaxBytes int

	// DenyRegex blocks obvious secret patterns (API keys, bearer tokens, etc).
	DenyRegex []*regexp.Regexp
}

// DefaultProfileEvaluator returns the default profile update policy.
func DefaultProfileEvaluator() *ProfileEvaluator {
	// Reuse the same deny patterns as MemoryEvaluator (secrets are secrets in any scope).
	me := DefaultMemoryEvaluator()
	return &ProfileEvaluator{
		MaxBytes:  1024,
		DenyRegex: me.DenyRegex,
	}
}

var profileKVLineRE = regexp.MustCompile(`^\s*(?:-\s*)?[A-Za-z][A-Za-z0-9 _-]{0,40}\s*:\s+\S+`)

var forbiddenProfileKeys = map[string]bool{
	"RULE":    true,
	"NOTE":    true,
	"OBS":     true,
	"LEARNED": true,
}

// Evaluate checks whether a profile update should be committed to /profile/profile.md.
//
// It returns:
//   - accepted: whether the host should commit the update
//   - reason: a short machine-readable reason (useful for auditing)
//   - cleaned: the normalized text to commit (when accepted)
func (e *ProfileEvaluator) Evaluate(update string) (accepted bool, reason string, cleaned string) {
	if e == nil {
		return false, "evaluator_missing", ""
	}
	update = strings.TrimSpace(update)
	if update == "" {
		return false, "empty", ""
	}
	maxBytes := e.MaxBytes
	if maxBytes <= 0 {
		maxBytes = 1024
	}
	if len(update) > maxBytes {
		return false, "too_large", ""
	}

	for _, re := range e.DenyRegex {
		if re != nil && re.MatchString(update) {
			return false, "denied_pattern", ""
		}
	}

	lines := strings.Split(update, "\n")
	seen := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		seen++
		if !profileKVLineRE.MatchString(line) {
			return false, "not_profile", ""
		}
		key := strings.TrimSpace(line)
		key = strings.TrimPrefix(key, "-")
		key = strings.TrimSpace(key)
		if idx := strings.Index(key, ":"); idx > 0 {
			k := strings.ToUpper(strings.TrimSpace(key[:idx]))
			if forbiddenProfileKeys[k] {
				return false, "not_profile", ""
			}
		}
	}
	if seen == 0 {
		return false, "empty", ""
	}
	return true, "accepted", strings.TrimSpace(update) + "\n"
}
