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
	if e.kind == feedUser {
		if sid := strings.TrimSpace(e.sourceID); sid != "" {
			return "user:" + normalizeOpSourceID(sid)
		}
	}
	if key := taskResponseKeyFromEntry(e); key != "" {
		return "task:" + key
	}
	if e.kind == feedThinking && strings.TrimSpace(e.sourceID) != "" {
		return "thinking:" + strings.TrimSpace(e.sourceID)
	}
	if e.kind == feedAgent && !e.isText {
		if sid := strings.TrimSpace(e.sourceID); sid != "" {
			return "op:" + normalizeOpSourceID(sid)
		}
		if e.data != nil {
			if opID := strings.TrimSpace(e.data["opId"]); opID != "" {
				return "op:" + opID
			}
		}
	}
	return ""
}

// normalizeOpSourceID strips the team-mode run prefix from a sourceID so that
// live-streamed IDs ("runID|opID") and polled IDs ("runID:runID|opID") resolve
// to the same canonical value ("runID|opID").
func normalizeOpSourceID(sid string) string {
	// Polled team-mode format: "outerRunID:innerRunID|opID"
	// Strip everything up to and including the first ":" when a "|" follows.
	if idx := strings.Index(sid, ":"); idx >= 0 {
		rest := sid[idx+1:]
		if strings.Contains(rest, "|") {
			return rest
		}
	}
	return sid
}

// isBridgeOp returns true if the entry is a code_exec bridge operation,
// identified by explicit action or tag markers in its data.
func isBridgeOp(e feedEntry) bool {
	if e.data == nil {
		return false
	}
	if strings.TrimSpace(e.data["action"]) == "code_exec_bridge" {
		return true
	}
	if strings.TrimSpace(e.data["tag"]) == "code_exec_bridge" {
		return true
	}
	return false
}

// absorbBridgeOp merges a bridge operation into its parent code_exec entry.
// Plan writes promote their data; write ops are stored in bridgeWriteOps;
// non-write ops increment childCount.
func absorbBridgeOp(parent *feedEntry, bridge feedEntry) {
	if isActivityPlanWrite(bridge.opKind, bridge.path) {
		if len(bridge.planItems) > 0 {
			parent.planItems = bridge.planItems
		}
		if bridge.planDetailsTitle != "" {
			parent.planDetailsTitle = bridge.planDetailsTitle
		}
		return
	}
	if isWriteOp(bridge.opKind) {
		parent.bridgeWriteOps = append(parent.bridgeWriteOps, bridge)
		return
	}
	parent.childCount++
	if parent.childCount == 1 {
		parent.bridgeSingleOpKind = bridge.opKind
		parent.bridgeSingleData = bridge.data
		parent.bridgeSingleText = bridge.text
		parent.bridgeSinglePath = bridge.path
	} else if parent.childCount == 2 {
		parent.bridgeSingleOpKind = ""
		parent.bridgeSingleData = nil
		parent.bridgeSingleText = ""
		parent.bridgeSinglePath = ""
	}
}

// groupBridgeOpsInEntries collapses bridge tool calls into their parent
// code_exec entries in a flat slice. Detection uses explicit markers and
// temporal overlap (entry started while code_exec was running).
func groupBridgeOpsInEntries(entries []feedEntry) []feedEntry {
	if len(entries) == 0 {
		return entries
	}
	result := make([]feedEntry, 0, len(entries))
	lastCodeExecIdx := -1

	for _, e := range entries {
		bridge := isBridgeOp(e)

		if !bridge && lastCodeExecIdx >= 0 {
			ce := result[lastCodeExecIdx]
			if !ce.finishedAt.IsZero() &&
				!e.timestamp.Before(ce.timestamp) &&
				!e.timestamp.After(ce.finishedAt) {
				bridge = true
			}
		}

		if bridge && lastCodeExecIdx >= 0 {
			absorbBridgeOp(&result[lastCodeExecIdx], e)
			continue
		}

		if strings.ToLower(strings.TrimSpace(e.opKind)) == "code_exec" {
			lastCodeExecIdx = len(result)
		} else if lastCodeExecIdx >= 0 {
			ce := result[lastCodeExecIdx]
			if !ce.finishedAt.IsZero() && e.timestamp.After(ce.finishedAt) {
				lastCodeExecIdx = -1
			}
		}
		result = append(result, e)
	}
	return result
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
