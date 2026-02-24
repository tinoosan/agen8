package coordinator

import "strings"

var semanticToolResultTags = map[string]struct{}{
	"task_create": {},
	"task_review": {},
	"obsidian":    {},
	"soul_update": {},
}

func normalizeFeedEntry(entry *feedEntry) *feedEntry {
	if entry == nil {
		return nil
	}
	if entry.data != nil {
		if strings.TrimSpace(entry.sourceID) == "" {
			if opID := strings.TrimSpace(entry.data["opId"]); opID != "" {
				entry.sourceID = opID
			}
		}
	}
	entry.opKind = canonicalSemanticOp(entry.opKind, entry.data)
	entry.identityKey = feedEntryIdentityKey(*entry)
	return entry
}

func canonicalSemanticOp(kind string, data map[string]string) string {
	k := strings.TrimSpace(strings.ToLower(kind))
	if data == nil {
		return k
	}
	tag := strings.TrimSpace(strings.ToLower(data["tag"]))
	op := strings.TrimSpace(strings.ToLower(data["op"]))

	if k == "tool_result" || op == "tool_result" || k == "" {
		if _, ok := semanticToolResultTags[tag]; ok {
			return tag
		}
		if op != "" && op != "tool_result" {
			return op
		}
	}
	if k == "" {
		return op
	}
	return k
}

func feedEntryIdentityKey(e feedEntry) string {
	if v := strings.TrimSpace(e.identityKey); v != "" {
		return v
	}
	if key := taskResponseKeyFromEntry(e); key != "" {
		return "task:" + key
	}
	if e.kind == feedThinking && strings.TrimSpace(e.sourceID) != "" {
		return "thinking:" + strings.TrimSpace(e.sourceID)
	}
	if e.kind == feedAgent && !e.isText {
		if sid := strings.TrimSpace(e.sourceID); sid != "" {
			return "op:" + sid
		}
		if e.data != nil {
			if opID := strings.TrimSpace(e.data["opId"]); opID != "" {
				return "op:" + opID
			}
		}
	}
	return ""
}

func dedupeFeedEntriesByIdentity(entries []feedEntry) []feedEntry {
	if len(entries) == 0 {
		return entries
	}
	seen := make(map[string]int, len(entries))
	out := make([]feedEntry, 0, len(entries))
	for _, entry := range entries {
		e := entry
		normalizeFeedEntry(&e)
		key := strings.TrimSpace(e.identityKey)
		if key == "" {
			out = append(out, e)
			continue
		}
		if idx, ok := seen[key]; ok {
			out[idx] = e
			continue
		}
		seen[key] = len(out)
		out = append(out, e)
	}
	return out
}
