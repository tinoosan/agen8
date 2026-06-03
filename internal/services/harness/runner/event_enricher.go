package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	harness "github.com/tinoosan/agen8-mcp-server/internal/services/harness/domain"
)

const (
	maxShellDiffFiles       = 12
	maxShellDiffFileBytes   = 256 * 1024
	maxShellPatchPreviewLen = 12_000
)

type harnessEventEnricher interface {
	Enrich(ctx context.Context, ev harness.Event) []harness.Event
}

func defaultHarnessEventEnrichers(workdir string) []harnessEventEnricher {
	enrichers := make([]harnessEventEnricher, 0, 1)
	if shell := newShellDiffEnricher(workdir); shell != nil {
		enrichers = append(enrichers, shell)
	}
	return enrichers
}

type shellDiffEnricher struct {
	repoRoot    string
	scopePrefix string
	snapshots   map[string]shellDiffSnapshot
}

type shellDiffSnapshot struct {
	beforeState map[string]shellDirtyEntry
	beforeText  map[string]string
}

type shellDirtyEntry struct {
	untracked bool
}

type shellDiffChange struct {
	Path           string
	Op             string
	WriteMode      string
	PatchPreview   string
	PatchTruncated bool
	LinesAdded     int
	LinesRemoved   int
}

func newShellDiffEnricher(workdir string) *shellDiffEnricher {
	root, err := gitRepoRoot(context.Background(), workdir)
	if err != nil {
		return nil
	}
	scopePrefix := normalizeRepoPath(scopePrefixFromWorkdir(root, workdir))
	return &shellDiffEnricher{
		repoRoot:    root,
		scopePrefix: scopePrefix,
		snapshots:   map[string]shellDiffSnapshot{},
	}
}

func (e *shellDiffEnricher) Enrich(ctx context.Context, ev harness.Event) []harness.Event {
	if e == nil {
		return []harness.Event{ev}
	}
	if !isShellHarnessEvent(ev) {
		return []harness.Event{ev}
	}
	callID := strings.TrimSpace(ev.ToolCallID)
	if callID == "" {
		return []harness.Event{ev}
	}
	status := normalizeHarnessToolStatus(ev.Data["status"], ev.Type)

	switch ev.Type {
	case harness.EventToolCall:
		if status == "pending" {
			if err := e.captureSnapshot(ctx, callID); err != nil {
				return []harness.Event{withDiffCaptureError(ev, err)}
			}
		}
		return []harness.Event{ev}
	case harness.EventToolResult:
		// Some harnesses emit output-only in-progress frames without a separate
		// tool_call frame. Start a snapshot on first pending frame when needed.
		if status == "pending" {
			if _, ok := e.snapshots[callID]; !ok {
				if err := e.captureSnapshot(ctx, callID); err != nil {
					return []harness.Event{withDiffCaptureError(ev, err)}
				}
			}
			return []harness.Event{ev}
		}
		if status != "success" && status != "error" {
			return []harness.Event{ev}
		}
		changes, err := e.captureChanges(ctx, callID)
		if err != nil {
			return []harness.Event{withDiffCaptureError(ev, err)}
		}
		if len(changes) == 0 {
			return []harness.Event{ev}
		}
		out := []harness.Event{ev}
		out = append(out, synthesizeShellDiffEvents(ev, changes)...)
		return out
	default:
		return []harness.Event{ev}
	}
}

func (e *shellDiffEnricher) captureSnapshot(ctx context.Context, callID string) error {
	if _, exists := e.snapshots[callID]; exists {
		return nil
	}
	state, err := readGitDirtyState(ctx, e.repoRoot, e.scopePrefix)
	if err != nil {
		return err
	}
	beforeText := make(map[string]string, len(state))
	for path := range state {
		absPath := filepath.Join(e.repoRoot, filepath.FromSlash(path))
		text, ok, readErr := readTextWorktree(absPath)
		if readErr != nil {
			return fmt.Errorf("read baseline file %q: %w", path, readErr)
		}
		if ok {
			beforeText[path] = text
		}
	}
	e.snapshots[callID] = shellDiffSnapshot{
		beforeState: state,
		beforeText:  beforeText,
	}
	return nil
}

func (e *shellDiffEnricher) captureChanges(ctx context.Context, callID string) ([]shellDiffChange, error) {
	snap, ok := e.snapshots[callID]
	if !ok {
		return nil, nil
	}
	delete(e.snapshots, callID)

	afterState, err := readGitDirtyState(ctx, e.repoRoot, e.scopePrefix)
	if err != nil {
		return nil, err
	}
	paths := unionShellPaths(snap.beforeState, afterState)
	if len(paths) == 0 {
		return nil, nil
	}
	sort.Strings(paths)

	changes := make([]shellDiffChange, 0, len(paths))
	for _, path := range paths {
		beforeText, beforeKnown, err := e.beforeTextForPath(ctx, snap, path)
		if err != nil {
			return nil, err
		}
		afterText, afterKnown, err := readTextWorktree(filepath.Join(e.repoRoot, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		if !beforeKnown || !afterKnown {
			continue
		}
		if beforeText == afterText {
			continue
		}

		patchPreview, patchTruncated, linesAdded, linesRemoved := buildPatchPreview(path, beforeText, afterText)
		if strings.TrimSpace(patchPreview) == "" {
			continue
		}

		writeMode := "modified"
		op := "edit_file"
		if beforeText == "" && afterText != "" {
			writeMode = "created"
			op = "write_file"
		} else if beforeText != "" && afterText == "" {
			writeMode = "deleted"
			op = "edit_file"
		}

		changes = append(changes, shellDiffChange{
			Path:           path,
			Op:             op,
			WriteMode:      writeMode,
			PatchPreview:   patchPreview,
			PatchTruncated: patchTruncated,
			LinesAdded:     linesAdded,
			LinesRemoved:   linesRemoved,
		})
		if len(changes) >= maxShellDiffFiles {
			break
		}
	}
	return changes, nil
}

func (e *shellDiffEnricher) beforeTextForPath(ctx context.Context, snap shellDiffSnapshot, path string) (string, bool, error) {
	entry, existedBefore := snap.beforeState[path]
	if existedBefore {
		if text, ok := snap.beforeText[path]; ok {
			return text, true, nil
		}
		if entry.untracked {
			// Untracked file existed before but could not be captured (binary/large).
			return "", false, nil
		}
		// Tracked file was already dirty before this command and we could not
		// capture its baseline content safely.
		return "", false, nil
	}
	// File was clean before this command; resolve baseline from HEAD.
	text, ok, err := readTextFromGitHead(ctx, e.repoRoot, path)
	return text, ok, err
}

func synthesizeShellDiffEvents(base harness.Event, changes []shellDiffChange) []harness.Event {
	if len(changes) == 0 {
		return nil
	}
	status := normalizeHarnessToolStatus(base.Data["status"], harness.EventToolResult)
	if status == "" {
		status = "success"
	}
	sourceType := strings.TrimSpace(base.Data["sourceType"])
	if sourceType == "" {
		sourceType = "cli"
	}
	sourceID := strings.TrimSpace(base.Data["sourceId"])
	events := make([]harness.Event, 0, len(changes))
	for i, change := range changes {
		data := map[string]string{
			"status":         status,
			"sourceType":     sourceType,
			"sourceId":       sourceID,
			"op":             change.Op,
			"action":         "edit",
			"path":           change.Path,
			"writeMode":      change.WriteMode,
			"patchPreview":   change.PatchPreview,
			"patchTruncated": strconv.FormatBool(change.PatchTruncated),
			"linesAdded":     strconv.Itoa(change.LinesAdded),
			"linesRemoved":   strconv.Itoa(change.LinesRemoved),
		}
		if turnID := strings.TrimSpace(base.Data["turnId"]); turnID != "" {
			data["turnId"] = turnID
		}
		inputPayload, err := json.Marshal(map[string]string{
			"path": change.Path,
		})
		if err == nil {
			data["input"] = string(inputPayload)
		}
		result := strings.TrimSpace(change.PatchPreview)
		if result != "" {
			data["result"] = result
			data["outputPreview"] = truncateForField(result, 1200)
			if len(result) <= 4000 {
				data["outputFull"] = result
			}
		}
		events = append(events, harness.Event{
			Type:       harness.EventToolResult,
			TurnID:     strings.TrimSpace(base.TurnID),
			ToolCallID: fmt.Sprintf("%s:file:%d", strings.TrimSpace(base.ToolCallID), i+1),
			ToolName:   change.Op,
			Text:       result,
			Data:       data,
		})
	}
	return events
}

func withDiffCaptureError(ev harness.Event, err error) harness.Event {
	if err == nil {
		return ev
	}
	out := ev
	out.Data = cloneStringMap(ev.Data)
	out.Data["diffCaptureError"] = truncateForField(strings.TrimSpace(err.Error()), 512)
	return out
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isShellHarnessEvent(ev harness.Event) bool {
	op := normalizeHarnessToolOp(ev.Data["op"])
	if op == "" {
		op = normalizeHarnessToolOp(ev.ToolName)
	}
	return op == "shell_exec" || op == "bash"
}

func normalizeHarnessToolStatus(status string, eventType harness.EventType) string {
	normalized := strings.ToLower(strings.TrimSpace(status))
	switch normalized {
	case "in_progress", "running", "pending":
		return "pending"
	case "completed", "success", "ok", "done":
		return "success"
	case "failed", "error", "failure":
		return "error"
	}
	switch eventType {
	case harness.EventToolCall:
		return "pending"
	case harness.EventToolResult:
		return "success"
	default:
		return ""
	}
}

func truncateForField(value string, max int) string {
	trimmed := strings.TrimSpace(value)
	if max <= 0 || len(trimmed) <= max {
		return trimmed
	}
	return trimmed[:max]
}

func normalizeHarnessToolOp(name string) string {
	op := strings.TrimSpace(name)
	if op == "" {
		return ""
	}
	if idx := strings.LastIndex(op, "/"); idx >= 0 && idx < len(op)-1 {
		op = op[idx+1:]
	}
	if idx := strings.LastIndex(op, ":"); idx >= 0 && idx < len(op)-1 {
		op = op[idx+1:]
	}
	op = strings.ToLower(strings.TrimSpace(op))
	op = strings.ReplaceAll(op, "-", "_")
	op = strings.ReplaceAll(op, " ", "_")
	switch {
	case strings.HasPrefix(op, "bash."),
		strings.HasPrefix(op, "shell_exec."),
		strings.HasPrefix(op, "shell."):
		return "bash"
	case strings.HasPrefix(op, "http."):
		return "http"
	}
	switch op {
	case "shell":
		return "shell_exec"
	case "functions.exec_command", "exec_command", "shell_command", "command_execution", "shell_exec", "bash":
		return "bash"
	case "functions.apply_patch":
		return "edit_file"
	case "write", "writefile", "create_file", "write_file":
		return "write_file"
	case "edit", "editfile", "apply_patch", "str_replace_editor", "multiedit", "multi_edit", "edit_file":
		return "edit_file"
	case "read", "readfile", "read_file":
		return "read_file"
	case "glob", "grep", "search_files":
		return "search_files"
	case "ls", "list_files", "list_dir", "list_directory":
		return "list_files"
	case "web.run", "web_run", "web", "websearch", "web_search":
		return "web_search"
	case "toolsearch", "tool_search", "tool_search_tool", "tool_search.tool":
		return "tool_search"
	default:
		return op
	}
}

func gitRepoRoot(ctx context.Context, workdir string) (string, error) {
	dir := strings.TrimSpace(workdir)
	if dir == "" {
		dir = "."
	}
	out, err := runGit(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	root := strings.TrimSpace(string(out))
	if root == "" {
		return "", fmt.Errorf("git repo root is empty")
	}
	return root, nil
}

func readGitDirtyState(ctx context.Context, repoRoot, scopePrefix string) (map[string]shellDirtyEntry, error) {
	out := map[string]shellDirtyEntry{}
	scopeArgs := gitScopeArgs(scopePrefix)
	trackedUnstagedArgs := append([]string{"diff", "--name-only", "-z"}, scopeArgs...)
	trackedUnstaged, err := runGitPathSet(ctx, repoRoot, trackedUnstagedArgs...)
	if err != nil {
		return nil, err
	}
	trackedStagedArgs := append([]string{"diff", "--name-only", "--cached", "-z"}, scopeArgs...)
	trackedStaged, err := runGitPathSet(ctx, repoRoot, trackedStagedArgs...)
	if err != nil {
		return nil, err
	}
	untrackedArgs := append([]string{"ls-files", "--others", "--exclude-standard", "-z"}, scopeArgs...)
	untracked, err := runGitPathSet(ctx, repoRoot, untrackedArgs...)
	if err != nil {
		return nil, err
	}
	ignoredWorkspace, err := readIgnoredWorkspaceFiles(ctx, repoRoot, scopePrefix)
	if err != nil {
		return nil, err
	}
	for path := range trackedUnstaged {
		out[path] = shellDirtyEntry{}
	}
	for path := range trackedStaged {
		out[path] = shellDirtyEntry{}
	}
	for path := range untracked {
		entry := out[path]
		entry.untracked = true
		out[path] = entry
	}
	for path := range ignoredWorkspace {
		entry := out[path]
		entry.untracked = true
		out[path] = entry
	}
	return out, nil
}

func gitScopeArgs(scopePrefix string) []string {
	prefix := normalizeRepoPath(scopePrefix)
	if prefix == "" {
		return []string{"--"}
	}
	return []string{"--", prefix}
}

func readIgnoredWorkspaceFiles(ctx context.Context, repoRoot, scopePrefix string) (map[string]struct{}, error) {
	pathspecs := ignoredWorkspacePathspecs(scopePrefix)
	if len(pathspecs) == 0 {
		return map[string]struct{}{}, nil
	}
	args := []string{"ls-files", "--others", "--ignored", "--exclude-standard", "-z", "--"}
	args = append(args, pathspecs...)
	return runGitPathSet(ctx, repoRoot, args...)
}

func ignoredWorkspacePathspecs(scopePrefix string) []string {
	prefix := normalizeRepoPath(scopePrefix)
	candidates := []string{"workspace", ".agen8/workspace"}
	out := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = normalizeRepoPath(candidate)
		if candidate == "" {
			continue
		}
		if prefix != "" {
			candidate = normalizeRepoPath(filepath.Join(prefix, candidate))
		}
		out = append(out, candidate)
	}
	return out
}

func runGitPathSet(ctx context.Context, repoRoot string, args ...string) (map[string]struct{}, error) {
	raw, err := runGit(ctx, repoRoot, args...)
	if err != nil {
		return nil, err
	}
	out := map[string]struct{}{}
	parts := bytes.Split(raw, []byte{0})
	for _, part := range parts {
		path := normalizeRepoPath(string(part))
		if path == "" {
			continue
		}
		out[path] = struct{}{}
	}
	return out, nil
}

func readTextFromGitHead(ctx context.Context, repoRoot, path string) (string, bool, error) {
	path = normalizeRepoPath(path)
	if path == "" {
		return "", false, nil
	}
	raw, err := runGit(ctx, repoRoot, "show", "--no-textconv", "HEAD:"+path)
	if err != nil {
		// Path might not exist in HEAD (for example newly created file).
		return "", true, nil
	}
	if len(raw) > maxShellDiffFileBytes {
		return "", false, nil
	}
	if bytes.IndexByte(raw, 0) >= 0 || !utf8.Valid(raw) {
		return "", false, nil
	}
	return normalizeText(raw), true, nil
}

func readTextWorktree(absPath string) (string, bool, error) {
	f, err := os.Open(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", true, nil
		}
		return "", false, err
	}
	defer f.Close()

	buf, err := io.ReadAll(io.LimitReader(f, maxShellDiffFileBytes+1))
	if err != nil {
		return "", false, err
	}
	if len(buf) > maxShellDiffFileBytes {
		return "", false, nil
	}
	if bytes.IndexByte(buf, 0) >= 0 || !utf8.Valid(buf) {
		return "", false, nil
	}
	return normalizeText(buf), true, nil
}

func normalizeText(raw []byte) string {
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func buildPatchPreview(path, before, after string) (preview string, truncated bool, added int, removed int) {
	path = normalizeRepoPath(path)
	if path == "" {
		return "", false, 0, 0
	}
	fromFile := "a/" + path
	toFile := "b/" + path
	if before == "" && after != "" {
		fromFile = "/dev/null"
	}
	if before != "" && after == "" {
		toFile = "/dev/null"
	}
	edits := myers.ComputeEdits(span.URIFromPath(path), before, after)
	ud := gotextdiff.ToUnified(fromFile, toFile, before, edits)
	diffText := strings.TrimSpace(fmt.Sprintf("%s", ud))
	if diffText == "" {
		return "", false, 0, 0
	}
	diffText, truncated = capStringBytes(diffText, maxShellPatchPreviewLen)
	added, removed = countPatchStats(diffText)
	return diffText, truncated, added, removed
}

func countPatchStats(patch string) (added, removed int) {
	for _, line := range strings.Split(patch, "\n") {
		if strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---") {
			continue
		}
		if strings.HasPrefix(line, "+") {
			added++
		}
		if strings.HasPrefix(line, "-") {
			removed++
		}
	}
	return added, removed
}

func capStringBytes(value string, limit int) (string, bool) {
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}

func normalizeRepoPath(path string) string {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return ""
	}
	trimmed = filepath.ToSlash(trimmed)
	trimmed = strings.TrimPrefix(trimmed, "./")
	return trimmed
}

func scopePrefixFromWorkdir(repoRoot, workdir string) string {
	root := strings.TrimSpace(repoRoot)
	dir := strings.TrimSpace(workdir)
	if root == "" || dir == "" {
		return ""
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return ""
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	rel, err := filepath.Rel(absRoot, absDir)
	if err != nil {
		return ""
	}
	rel = normalizeRepoPath(rel)
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}

func unionShellPaths(a, b map[string]shellDirtyEntry) []string {
	set := map[string]struct{}{}
	for path := range a {
		set[path] = struct{}{}
	}
	for path := range b {
		set[path] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for path := range set {
		out = append(out, path)
	}
	return out
}

func runGit(ctx context.Context, repoRoot string, args ...string) ([]byte, error) {
	cmdArgs := []string{"-C", strings.TrimSpace(repoRoot)}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.CommandContext(ctx, "git", cmdArgs...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("git %s: %s", strings.Join(args, " "), strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return out, nil
}
