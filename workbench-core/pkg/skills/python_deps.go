package skills

import (
	"regexp"
	"sort"
	"strings"
)

var pythonImportNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var nonPythonRequirementTokens = map[string]struct{}{
	"and":      {},
	"or":       {},
	"for":      {},
	"only":     {},
	"requires": {},
}

var nonPythonRequirementNames = map[string]struct{}{
	"python":  {},
	"python3": {},
	"bash":    {},
	"curl":    {},
	"jq":      {},
	"pandoc":  {},
	"launchd": {},
	"cron":    {},
	"macos":   {},
	"linux":   {},
	"windows": {},
}

// RequiredPythonImportsFromEntries extracts required Python modules from skill compatibility fields.
func RequiredPythonImportsFromEntries(entries []SkillEntry) []string {
	if len(entries) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for _, entry := range entries {
		if entry.Skill == nil {
			continue
		}
		for _, mod := range RequiredPythonImportsFromCompatibility(entry.Skill.Compatibility) {
			seen[mod] = struct{}{}
		}
	}
	return sortedStringSet(seen)
}

// RequiredPythonImportsFromCompatibility parses required Python imports from a compatibility string.
// It reads only the required segment and ignores optional dependencies.
func RequiredPythonImportsFromCompatibility(compatibility string) []string {
	compatibility = strings.TrimSpace(compatibility)
	if compatibility == "" {
		return nil
	}
	requiredSegment := requiredCompatibilitySegment(compatibility)
	if requiredSegment == "" {
		return nil
	}
	if !strings.Contains(strings.ToLower(requiredSegment), "python") {
		// Parse only Python dependency declarations for code_exec readiness.
		return nil
	}
	candidates := splitRequiredCandidates(requiredSegment)
	if len(candidates) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		token := normalizedCandidateFirstToken(candidate)
		if token == "" {
			continue
		}
		if _, skip := nonPythonRequirementTokens[token]; skip {
			continue
		}
		if _, skip := nonPythonRequirementNames[token]; skip {
			continue
		}
		if !pythonImportNamePattern.MatchString(token) {
			continue
		}
		seen[token] = struct{}{}
	}
	return sortedStringSet(seen)
}

func requiredCompatibilitySegment(compatibility string) string {
	lower := strings.ToLower(compatibility)
	idx := strings.Index(lower, "requires")
	if idx < 0 {
		return ""
	}
	segment := strings.TrimSpace(compatibility[idx+len("requires"):])
	if segment == "" {
		return ""
	}
	if dot := strings.Index(segment, "."); dot >= 0 {
		segment = segment[:dot]
	}
	if cut := firstCaseInsensitiveIndex(segment, []string{"optional", "supports", " no "}); cut >= 0 {
		segment = segment[:cut]
	}
	return strings.TrimSpace(segment)
}

func splitRequiredCandidates(requiredSegment string) []string {
	requiredSegment = strings.TrimSpace(requiredSegment)
	if requiredSegment == "" {
		return nil
	}
	parts := strings.Split(requiredSegment, ",")
	out := make([]string, 0, len(parts)*2)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		subparts := strings.Split(part, " and ")
		for _, sub := range subparts {
			sub = strings.TrimSpace(sub)
			if sub == "" {
				continue
			}
			out = append(out, sub)
		}
	}
	return out
}

func normalizedCandidateFirstToken(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	candidate = strings.NewReplacer(
		";", " ",
		":", " ",
		"-", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"{", " ",
		"}", " ",
		"/", " ",
	).Replace(candidate)
	fields := strings.Fields(candidate)
	if len(fields) == 0 {
		return ""
	}
	token := strings.TrimSpace(fields[0])
	token = strings.Trim(token, ".,")
	token = strings.ToLower(token)
	if token == "" {
		return ""
	}
	if cut := strings.IndexAny(token, "<>=!~"); cut >= 0 {
		token = strings.TrimSpace(token[:cut])
	}
	token = strings.TrimSpace(token)
	return token
}

func firstCaseInsensitiveIndex(s string, needles []string) int {
	sLower := strings.ToLower(s)
	cut := -1
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle == "" {
			continue
		}
		idx := strings.Index(sLower, needle)
		if idx < 0 {
			continue
		}
		if cut < 0 || idx < cut {
			cut = idx
		}
	}
	return cut
}

func sortedStringSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for item := range set {
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}
