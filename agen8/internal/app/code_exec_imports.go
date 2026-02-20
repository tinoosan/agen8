package app

import (
	"sort"
	"strings"
)

func resolveCodeExecRequiredImports(configRequired []string) []string {
	set := map[string]struct{}{}
	for _, mod := range configRequired {
		mod = strings.TrimSpace(mod)
		if mod == "" {
			continue
		}
		set[mod] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for mod := range set {
		out = append(out, mod)
	}
	sort.Strings(out)
	return out
}

func parseMissingPythonModules(err error) []string {
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	marker := "missing python module(s):"
	idx := strings.Index(lower, marker)
	if idx < 0 {
		return nil
	}
	raw := strings.TrimSpace(msg[idx+len(marker):])
	if raw == "" {
		return nil
	}
	set := map[string]struct{}{}
	for _, part := range strings.Split(raw, ",") {
		mod := strings.TrimSpace(part)
		if mod == "" {
			continue
		}
		set[mod] = struct{}{}
	}
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for mod := range set {
		out = append(out, mod)
	}
	sort.Strings(out)
	return out
}
