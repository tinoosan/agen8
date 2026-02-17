package runtime

import (
	"context"
	"encoding/json"
	"sort"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/opmeta"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// HostOperation defines a strategy for a single host op type.
type HostOperation interface {
	Op() string
	Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse
	FormatRequestText(req types.HostOpRequest, reqData map[string]string) string
	FormatResponseText(req types.HostOpRequest, resp types.HostOpResponse, reqData map[string]string, respData map[string]string) string
}

type HostOpRequestStoreFields interface {
	RequestStoreFields(req types.HostOpRequest) map[string]string
}

type HostOpResponseStoreFields interface {
	ResponseStoreFields(resp types.HostOpResponse) map[string]string
}

type HostOpDiffAfterResolver interface {
	ResolveAfter(req types.HostOpRequest, before string, fs *vfs.FS) (after string, ok bool)
}

type HostOpRequestEventEnricher interface {
	EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string)
}

type HostOpResponseEventEnricher interface {
	EnrichResponseEvent(req types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, storeResp map[string]string)
}

type hostOperationRegistry struct {
	byOp map[string]HostOperation
}

func newHostOperationRegistry(custom []HostOperation) *hostOperationRegistry {
	ops := defaultHostOperations()
	ops = append(ops, custom...)
	r := &hostOperationRegistry{byOp: make(map[string]HostOperation, len(ops))}
	for _, op := range ops {
		if op == nil {
			continue
		}
		name := strings.TrimSpace(op.Op())
		if name == "" {
			continue
		}
		r.byOp[name] = op
	}
	return r
}

func (r *hostOperationRegistry) Get(op string) HostOperation {
	if r == nil {
		return nil
	}
	return r.byOp[strings.TrimSpace(op)]
}

func defaultHostOperations() []HostOperation {
	return []HostOperation{
		fsListOperation{},
		fsReadOperation{},
		fsSearchOperation{},
		fsWriteOperation{},
		fsAppendOperation{},
		fsEditOperation{},
		fsPatchOperation{},
		shellExecOperation{},
		httpFetchOperation{},
		browserOperation{},
		traceRunOperation{},
		emailOperation{},
		noopOperation{},
		toolResultOperation{},
		agentFinalOperation{},
	}
}

type fsListOperation struct{}

func (fsListOperation) Op() string { return types.HostOpFSList }
func (fsListOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsListOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsListOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type fsReadOperation struct{}

func (fsReadOperation) Op() string { return types.HostOpFSRead }
func (fsReadOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsReadOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsReadOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (fsReadOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, _ map[string]string) {
	if req.MaxBytes != 0 {
		reqData["maxBytes"] = strconv.Itoa(req.MaxBytes)
	}
}

type fsSearchOperation struct{}

func (fsSearchOperation) Op() string { return types.HostOpFSSearch }
func (fsSearchOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsSearchOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsSearchOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type fsWriteOperation struct{}

func (fsWriteOperation) Op() string { return types.HostOpFSWrite }
func (fsWriteOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsWriteOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsWriteOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (fsWriteOperation) ResolveAfter(req types.HostOpRequest, _ string, _ *vfs.FS) (string, bool) {
	return req.Text, true
}

type fsAppendOperation struct{}

func (fsAppendOperation) Op() string { return types.HostOpFSAppend }
func (fsAppendOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsAppendOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsAppendOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (fsAppendOperation) ResolveAfter(req types.HostOpRequest, before string, _ *vfs.FS) (string, bool) {
	return before + req.Text, true
}

type fsEditOperation struct{}

func (fsEditOperation) Op() string { return types.HostOpFSEdit }
func (fsEditOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsEditOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsEditOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (fsEditOperation) ResolveAfter(req types.HostOpRequest, _ string, fs *vfs.FS) (string, bool) {
	if fs == nil {
		return "", true
	}
	if b, err := fs.Read(req.Path); err == nil {
		return string(b), true
	}
	return "", true
}

type fsPatchOperation struct{}

func (fsPatchOperation) Op() string { return types.HostOpFSPatch }
func (fsPatchOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsPatchOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (fsPatchOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type shellExecOperation struct{}

func (shellExecOperation) Op() string { return types.HostOpShellExec }
func (shellExecOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (shellExecOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (shellExecOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (shellExecOperation) RequestStoreFields(req types.HostOpRequest) map[string]string {
	return shellArgsToFields(req.Argv, req.Cwd)
}
func (shellExecOperation) ResponseStoreFields(resp types.HostOpResponse) map[string]string {
	fields := map[string]string{
		"exitCode": strconv.Itoa(resp.ExitCode),
	}
	fields["vfsPathTranslated"] = fmtBool(resp.VFSPathTranslated)
	fields["vfsPathMounts"] = strings.TrimSpace(resp.VFSPathMounts)
	fields["scriptPathNormalized"] = fmtBool(resp.ScriptPathNormalized)
	anti := strings.TrimSpace(resp.ScriptAntiPattern)
	if anti == "" {
		anti = "none"
	}
	fields["scriptAntiPattern"] = anti
	return fields
}

type httpFetchOperation struct{}

func (httpFetchOperation) Op() string { return types.HostOpHTTPFetch }
func (httpFetchOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (httpFetchOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (httpFetchOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (httpFetchOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	reqData["url"] = req.URL
	method := strings.TrimSpace(req.Method)
	if method == "" {
		method = "GET"
	} else {
		method = strings.ToUpper(method)
	}
	reqData["method"] = method
	storeReq["url"] = req.URL
	storeReq["method"] = method
	if body := strings.TrimSpace(req.Body); body != "" {
		if looksSensitiveText(body) {
			reqData["body"] = "<omitted>"
			storeReq["body"] = "<omitted>"
			return
		}
		if preview, truncated := capBytes(body, maxHTTPBodyPreviewBytes); preview != "" {
			reqData["body"] = preview
			storeReq["body"] = preview
			if truncated {
				reqData["bodyTruncated"] = "true"
				storeReq["bodyTruncated"] = "true"
			}
		}
	}
}
func (httpFetchOperation) EnrichResponseEvent(_ types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, storeResp map[string]string) {
	respData["status"] = strconv.Itoa(resp.Status)
	if resp.FinalURL != "" {
		respData["finalUrl"] = resp.FinalURL
		storeResp["finalUrl"] = resp.FinalURL
	}
	if resp.Body != "" {
		s, tr := capBytes(resp.Body, 1000)
		respData["body"] = s
		if tr {
			respData["bodyTruncated"] = "true"
		}
	}
	storeResp["status"] = strconv.Itoa(resp.Status)
}

type browserOperation struct{}

func (browserOperation) Op() string { return types.HostOpBrowser }
func (browserOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (browserOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (browserOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type traceRunOperation struct{}

func (traceRunOperation) Op() string { return types.HostOpTrace }
func (traceRunOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (traceRunOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (traceRunOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type emailOperation struct{}

func (emailOperation) Op() string { return types.HostOpEmail }
func (emailOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (emailOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (emailOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type noopOperation struct{}

func (noopOperation) Op() string { return types.HostOpNoop }
func (noopOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (noopOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (noopOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}
func (noopOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	action := strings.TrimSpace(req.Action)
	tag := strings.TrimSpace(req.Tag)
	if action != "" {
		reqData["noopAction"] = action
		storeReq["noopAction"] = action
	}
	if tag == "task_create" {
		reqData["op"] = "task_create"
		storeReq["op"] = "task_create"
		if len(req.Input) > 0 {
			var payload struct {
				Goal       string `json:"goal"`
				TaskID     string `json:"taskId"`
				ChildRunID string `json:"childRunId"`
			}
			if err := json.Unmarshal(req.Input, &payload); err == nil {
				if g := strings.TrimSpace(payload.Goal); g != "" {
					if p, tr := capBytes(singleLine(g), 240); p != "" {
						reqData["goal"] = p
						storeReq["goal"] = p
						if tr {
							reqData["goalTruncated"] = "true"
							storeReq["goalTruncated"] = "true"
						}
					}
				}
				if id := strings.TrimSpace(payload.TaskID); id != "" {
					reqData["taskId"] = id
					storeReq["taskId"] = id
				}
				if cid := strings.TrimSpace(payload.ChildRunID); cid != "" {
					reqData["childRunId"] = cid
					storeReq["childRunId"] = cid
				}
			}
		}
		return
	}
	if action != "agent_spawn" {
		return
	}

	// Reclassify noop -> agent_spawn for UI and activity indexing.
	reqData["op"] = "agent_spawn"
	storeReq["op"] = "agent_spawn"

	if len(req.Input) == 0 {
		return
	}
	var payload struct {
		Goal               string   `json:"goal"`
		Model              string   `json:"model"`
		RequestedMaxTokens int      `json:"requestedMaxTokens"`
		MaxTokens          int      `json:"maxTokens"`
		BackgroundCount    int      `json:"backgroundCount"`
		BackgroundPreview  []string `json:"backgroundPreview"`
		CurrentDepth       int      `json:"currentDepth"`
		MaxDepth           int      `json:"maxDepth"`
	}
	if err := json.Unmarshal(req.Input, &payload); err != nil {
		return
	}
	if goal := strings.TrimSpace(payload.Goal); goal != "" {
		if p, tr := capBytes(singleLine(goal), 240); p != "" {
			reqData["goal"] = p
			storeReq["goal"] = p
			if tr {
				reqData["goalTruncated"] = "true"
				storeReq["goalTruncated"] = "true"
			}
		}
	}
	if model := strings.TrimSpace(payload.Model); model != "" {
		reqData["model"] = model
		storeReq["model"] = model
	}
	if payload.RequestedMaxTokens > 0 {
		v := strconv.Itoa(payload.RequestedMaxTokens)
		reqData["requestedMaxTokens"] = v
		storeReq["requestedMaxTokens"] = v
	}
	if payload.MaxTokens > 0 {
		v := strconv.Itoa(payload.MaxTokens)
		reqData["maxTokens"] = v
		storeReq["maxTokens"] = v
	}
	if payload.BackgroundCount > 0 {
		v := strconv.Itoa(payload.BackgroundCount)
		reqData["backgroundCount"] = v
		storeReq["backgroundCount"] = v
	}
	if len(payload.BackgroundPreview) > 0 {
		if b, err := json.Marshal(payload.BackgroundPreview); err == nil {
			reqData["backgroundPreview"] = string(b)
			storeReq["backgroundPreview"] = string(b)
		}
	}
	reqData["currentDepth"] = strconv.Itoa(payload.CurrentDepth)
	storeReq["currentDepth"] = strconv.Itoa(payload.CurrentDepth)
	reqData["maxDepth"] = strconv.Itoa(payload.MaxDepth)
	storeReq["maxDepth"] = strconv.Itoa(payload.MaxDepth)
}
func (noopOperation) EnrichResponseEvent(req types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, storeResp map[string]string) {
	action := strings.TrimSpace(req.Action)
	tag := strings.TrimSpace(req.Tag)
	if tag == "task_create" {
		respData["op"] = "task_create"
		storeResp["op"] = "task_create"
	}
	if action == "agent_spawn" {
		respData["op"] = "agent_spawn"
		storeResp["op"] = "agent_spawn"
	}
	if out := strings.TrimSpace(resp.Text); out != "" {
		if p, tr := capBytes(out, 1200); p != "" {
			respData["output"] = p
			respData["outputPreview"] = p
			if tr {
				respData["outputTruncated"] = "true"
			}
		}
	}
}

type toolResultOperation struct{}

func (toolResultOperation) Op() string { return types.HostOpToolResult }
func (toolResultOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (toolResultOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (toolResultOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

type agentFinalOperation struct{}

func (agentFinalOperation) Op() string { return types.HostOpFinal }
func (agentFinalOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (agentFinalOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return formatOpRequestText(reqData)
}
func (agentFinalOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return formatOpResponseText(respData)
}

func formatOpRequestText(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	tag := strings.TrimSpace(d["tag"])

	if tag == "task_create" || op == "task_create" {
		return "Create task"
	}

	switch op {
	case "browser":
		action := strings.TrimSpace(d["action"])
		if action == "" {
			return "browse"
		}
		desc := "browse." + action
		target := ""
		switch action {
		case "navigate":
			target = strings.TrimSpace(d["url"])
		case "click", "type", "hover", "check", "uncheck", "upload", "download", "select":
			target = strings.TrimSpace(d["selector"])
		case "extract", "extract_links":
			target = strings.TrimSpace(d["selector"])
			if target == "" {
				target = "page"
			}
		default:
			target = firstNonEmptyForDisplay(
				strings.TrimSpace(d["url"]),
				strings.TrimSpace(d["selector"]),
			)
		}
		if target != "" {
			return desc + " " + singleLinePreviewForDisplay(target, 120)
		}
		return desc
	case "email":
		to := singleLinePreviewForDisplay(strings.TrimSpace(d["to"]), 48)
		subject := singleLinePreviewForDisplay(strings.TrimSpace(d["subject"]), 72)
		if to != "" && subject != "" {
			return "Email " + to + ": " + subject
		}
		if to != "" {
			return "Email " + to
		}
		return "Email"
	case "agent_spawn":
		return opmeta.FormatRequestTitle(d)
	default:
		if isSharedOpRequestTitleOp(op) {
			return opmeta.FormatRequestTitle(d)
		}
		return compactKVForDisplay(d, []string{"op", "path"})
	}
}

func formatOpResponseText(d map[string]string) string {
	op := strings.TrimSpace(d["op"])
	tag := strings.TrimSpace(d["tag"])
	ok := strings.TrimSpace(d["ok"])
	errStr := strings.TrimSpace(d["err"])

	prefix := "✓"
	if ok != "true" {
		prefix = "✗"
	}

	if tag == "task_create" {
		if ok == "true" {
			return prefix + " " + strings.TrimSpace(d["text"])
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " task creation failed"
	}

	switch op {
	case "fs_read":
		tr := strings.TrimSpace(d["truncated"])
		if ok == "true" && tr == "true" {
			return prefix + " truncated"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	case "shell_exec":
		exitCode := strings.TrimSpace(d["exitCode"])
		if ok == "true" {
			if exitCode != "" {
				return prefix + " exit " + exitCode
			}
			return prefix + " ok"
		}
		if exitCode != "" && errStr != "" {
			return prefix + " exit " + exitCode + ": " + errStr
		}
		if exitCode != "" {
			return prefix + " exit " + exitCode
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " failed"
	case "http_fetch":
		status := strings.TrimSpace(d["status"])
		if ok == "true" {
			if status != "" {
				return prefix + " " + status
			}
			return prefix + " ok"
		}
		if status != "" && errStr != "" {
			return prefix + " " + status + ": " + errStr
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		if status != "" {
			return prefix + " " + status
		}
		return prefix + " failed"
	case "fs_search":
		results := strings.TrimSpace(d["results"])
		if ok == "true" && results != "" {
			if results == "1" {
				return prefix + " 1 result"
			}
			return prefix + " " + results + " results"
		}
		if ok != "true" && errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	case "email":
		if ok == "true" {
			return prefix + " sent"
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " failed"
	case "agent_spawn":
		if ok == "true" {
			return prefix + " child completed"
		}
		if errStr != "" {
			return prefix + " " + errStr
		}
		return prefix + " child failed"
	default:
		if strings.HasPrefix(op, "browser.") || op == "browser" {
			if ok != "true" {
				if errStr != "" {
					return prefix + " " + errStr
				}
				return prefix + " browser failed"
			}
			browserOp := strings.TrimPrefix(firstNonEmptyForDisplay(strings.TrimSpace(d["browserOp"]), op), "browser.")
			title := strings.TrimSpace(d["title"])
			count := firstNonEmptyForDisplay(strings.TrimSpace(d["items"]), strings.TrimSpace(d["count"]))
			switch browserOp {
			case "navigate", "back", "forward", "reload":
				if title != "" {
					return prefix + " navigated " + strconv.Quote(singleLinePreviewForDisplay(title, 80))
				}
				return prefix + " navigated"
			case "extract", "extract_links", "tab_list":
				if count != "" {
					return prefix + " extracted " + count + " items"
				}
				return prefix + " extracted"
			case "click":
				return prefix + " clicked"
			case "type":
				return prefix + " typed"
			case "screenshot", "pdf":
				return prefix + " captured"
			case "start":
				return prefix + " browser started"
			case "close":
				return prefix + " browser closed"
			default:
				if browserOp != "" {
					return prefix + " " + strings.ReplaceAll(browserOp, "_", " ")
				}
				return prefix + " browser ok"
			}
		}
		if errStr != "" && ok != "true" {
			return prefix + " " + errStr
		}
		return prefix + " ok"
	}
}

func isSharedOpRequestTitleOp(op string) bool {
	switch strings.TrimSpace(op) {
	case "fs_list", "fs_read", "fs_search", "fs_write", "fs_append", "fs_edit", "fs_patch", "shell_exec", "http_fetch", "trace_run", "agent_spawn", "task_create":
		return true
	default:
		return false
	}
}

func singleLinePreviewForDisplay(s string, max int) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	if max < 2 {
		return s[:max]
	}
	return s[:max-1] + "…"
}

func firstNonEmptyForDisplay(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func compactKVForDisplay(m map[string]string, ordered []string) string {
	if len(m) == 0 {
		return ""
	}
	var keys []string
	if len(ordered) != 0 {
		keys = ordered
	} else {
		keys = make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}

	parts := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, k := range keys {
		seen[k] = true
		if v := strings.TrimSpace(m[k]); v != "" {
			parts = append(parts, k+"="+v)
		}
	}
	if len(ordered) != 0 {
		rest := make([]string, 0, len(m))
		for k := range m {
			if seen[k] {
				continue
			}
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range rest {
			if v := strings.TrimSpace(m[k]); v != "" {
				parts = append(parts, k+"="+v)
			}
		}
	}

	return strings.Join(parts, " ")
}
