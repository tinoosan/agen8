package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/opformat"
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

func resolveOperationForResponse(r *hostOperationRegistry, req types.HostOpRequest, resp types.HostOpResponse) HostOperation {
	if r == nil {
		return nil
	}
	if op := r.Get(resp.Op); op != nil {
		return op
	}
	respOp := strings.TrimSpace(resp.Op)
	switch {
	case strings.HasPrefix(respOp, "browser."):
		if op := r.Get(types.HostOpBrowser); op != nil {
			return op
		}
	case respOp == "task_create":
		if op := r.Get(types.HostOpToolResult); op != nil {
			return op
		}
	}
	if op := r.Get(req.Op); op != nil {
		return op
	}
	return nil
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
		codeExecOperation{},
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
	return opformat.FormatRequestText(reqData)
}
func (fsListOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}

type fsReadOperation struct{}

func (fsReadOperation) Op() string { return types.HostOpFSRead }
func (fsReadOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsReadOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (fsReadOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
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
	return opformat.FormatRequestText(reqData)
}
func (fsSearchOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (fsSearchOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	if strings.TrimSpace(req.Query) != "" {
		reqData["query"] = strings.TrimSpace(req.Query)
		storeReq["query"] = strings.TrimSpace(req.Query)
	}
	if req.Limit != 0 {
		reqData["limit"] = strconv.Itoa(req.Limit)
		storeReq["limit"] = strconv.Itoa(req.Limit)
	}
}
func (fsSearchOperation) EnrichResponseEvent(_ types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, _ map[string]string) {
	respData["results"] = strconv.Itoa(len(resp.Results))
}

type fsWriteOperation struct{}

func (fsWriteOperation) Op() string { return types.HostOpFSWrite }
func (fsWriteOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsWriteOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (fsWriteOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (fsWriteOperation) ResolveAfter(req types.HostOpRequest, _ string, _ *vfs.FS) (string, bool) {
	return req.Text, true
}
func (fsWriteOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, _ map[string]string) {
	if strings.TrimSpace(req.Text) == "" {
		return
	}
	p, tr, red, n, isJSON := fsWriteTextPreviewForEvent(req.Path, req.Text)
	if p != "" {
		reqData["textPreview"] = p
	}
	if tr {
		reqData["textTruncated"] = "true"
	}
	if red {
		reqData["textRedacted"] = "true"
	}
	if n != 0 {
		reqData["textBytes"] = strconv.Itoa(n)
	}
	if isJSON {
		reqData["textIsJSON"] = "true"
	}
}

type fsAppendOperation struct{}

func (fsAppendOperation) Op() string { return types.HostOpFSAppend }
func (fsAppendOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsAppendOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (fsAppendOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (fsAppendOperation) ResolveAfter(req types.HostOpRequest, before string, _ *vfs.FS) (string, bool) {
	return before + req.Text, true
}
func (fsAppendOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, _ map[string]string) {
	if strings.TrimSpace(req.Text) == "" {
		return
	}
	p, tr, red, n, isJSON := fsWriteTextPreviewForEvent(req.Path, req.Text)
	if p != "" {
		reqData["textPreview"] = p
	}
	if tr {
		reqData["textTruncated"] = "true"
	}
	if red {
		reqData["textRedacted"] = "true"
	}
	if n != 0 {
		reqData["textBytes"] = strconv.Itoa(n)
	}
	if isJSON {
		reqData["textIsJSON"] = "true"
	}
}

type fsEditOperation struct{}

func (fsEditOperation) Op() string { return types.HostOpFSEdit }
func (fsEditOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsEditOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (fsEditOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
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
	return opformat.FormatRequestText(reqData)
}
func (fsPatchOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (fsPatchOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, _ map[string]string) {
	if strings.TrimSpace(req.Text) == "" {
		return
	}
	p, tr, red, n, _ := fsWriteTextPreviewForEvent(req.Path, req.Text)
	if p != "" {
		reqData["patchPreview"] = p
	}
	if tr {
		reqData["patchTruncated"] = "true"
	}
	if red {
		reqData["patchRedacted"] = "true"
	}
	if n != 0 {
		reqData["patchBytes"] = strconv.Itoa(n)
	}
}

type shellExecOperation struct{}

func (shellExecOperation) Op() string { return types.HostOpShellExec }
func (shellExecOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (shellExecOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (shellExecOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
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
func (shellExecOperation) EnrichResponseEvent(_ types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, storeResp map[string]string) {
	respData["exitCode"] = strconv.Itoa(resp.ExitCode)
	respData["vfsPathTranslated"] = fmtBool(resp.VFSPathTranslated)
	storeResp["vfsPathTranslated"] = fmtBool(resp.VFSPathTranslated)
	if mounts := strings.TrimSpace(resp.VFSPathMounts); mounts != "" {
		respData["vfsPathMounts"] = mounts
		storeResp["vfsPathMounts"] = mounts
	}
	respData["scriptPathNormalized"] = fmtBool(resp.ScriptPathNormalized)
	anti := strings.TrimSpace(resp.ScriptAntiPattern)
	if anti == "" {
		anti = "none"
	}
	respData["scriptAntiPattern"] = anti
	storeResp["scriptPathNormalized"] = fmtBool(resp.ScriptPathNormalized)
	storeResp["scriptAntiPattern"] = anti
	if resp.Stdout != "" {
		s, tr := capBytes(resp.Stdout, 1000)
		respData["stdout"] = s
		if tr {
			respData["stdoutTruncated"] = "true"
		}
	}
	if resp.Stderr != "" {
		s, tr := capBytes(resp.Stderr, 1000)
		respData["stderr"] = s
		if tr {
			respData["stderrTruncated"] = "true"
		}
	}
	if strings.TrimSpace(resp.Warning) != "" {
		if s, tr := capBytes(resp.Warning, 300); s != "" {
			respData["warning"] = s
			if tr {
				respData["warningTruncated"] = "true"
			}
		}
	}
}

type codeExecOperation struct{}

func (codeExecOperation) Op() string { return types.HostOpCodeExec }
func (codeExecOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (codeExecOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (codeExecOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (codeExecOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	if lang := strings.TrimSpace(req.Language); lang != "" {
		reqData["language"] = lang
		storeReq["language"] = lang
	}
	if cwd := strings.TrimSpace(req.Cwd); cwd != "" {
		reqData["cwd"] = cwd
		storeReq["cwd"] = cwd
	}
	if req.TimeoutMs > 0 {
		reqData["timeoutMs"] = strconv.Itoa(req.TimeoutMs)
		storeReq["timeoutMs"] = strconv.Itoa(req.TimeoutMs)
	}
	if req.MaxBytes > 0 {
		reqData["maxBytes"] = strconv.Itoa(req.MaxBytes)
		storeReq["maxBytes"] = strconv.Itoa(req.MaxBytes)
	}
	if len(req.Input) > 0 {
		var ext struct {
			MaxToolCalls *int `json:"maxToolCalls"`
		}
		if err := json.Unmarshal(req.Input, &ext); err == nil && ext.MaxToolCalls != nil {
			reqData["maxToolCalls"] = strconv.Itoa(*ext.MaxToolCalls)
			storeReq["maxToolCalls"] = strconv.Itoa(*ext.MaxToolCalls)
		}
	}
	if req.Code != "" {
		reqData["code"] = req.Code
		storeReq["code"] = req.Code
		reqData["codeBytes"] = strconv.Itoa(len(req.Code))
		storeReq["codeBytes"] = strconv.Itoa(len(req.Code))
	}
}
func (codeExecOperation) EnrichResponseEvent(_ types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, storeResp map[string]string) {
	if resp.ExitCode != 0 {
		respData["exitCode"] = strconv.Itoa(resp.ExitCode)
		storeResp["exitCode"] = strconv.Itoa(resp.ExitCode)
	}
	if strings.TrimSpace(resp.Stdout) != "" {
		if p, tr := capBytes(resp.Stdout, 1000); p != "" {
			respData["stdout"] = p
			storeResp["stdout"] = p
			if tr {
				respData["stdoutTruncated"] = "true"
				storeResp["stdoutTruncated"] = "true"
			}
		}
	}
	if strings.TrimSpace(resp.Stderr) != "" {
		if p, tr := capBytes(resp.Stderr, 1000); p != "" {
			respData["stderr"] = p
			storeResp["stderr"] = p
			if tr {
				respData["stderrTruncated"] = "true"
				storeResp["stderrTruncated"] = "true"
			}
		}
	}
	outputPreview := ""
	if len(resp.Text) == 0 {
		if v := strings.TrimSpace(resp.Stdout); v != "" {
			if p, _ := capBytes(v, 300); p != "" {
				respData["outputPreview"] = p
				storeResp["outputPreview"] = p
			}
		} else if v := strings.TrimSpace(resp.Stderr); v != "" {
			if p, _ := capBytes(v, 300); p != "" {
				respData["outputPreview"] = p
				storeResp["outputPreview"] = p
			}
		}
		return
	}
	var payload struct {
		Result          json.RawMessage `json:"result"`
		ToolCallCount   int             `json:"toolCallCount"`
		RuntimeMs       int64           `json:"runtimeMs"`
		ResultTruncated bool            `json:"resultTruncated"`
		StdoutTruncated bool            `json:"stdoutTruncated"`
		StderrTruncated bool            `json:"stderrTruncated"`
	}
	if err := json.Unmarshal([]byte(resp.Text), &payload); err != nil {
		return
	}
	respData["toolCallCount"] = strconv.Itoa(payload.ToolCallCount)
	storeResp["toolCallCount"] = strconv.Itoa(payload.ToolCallCount)
	respData["runtimeMs"] = strconv.FormatInt(payload.RuntimeMs, 10)
	storeResp["runtimeMs"] = strconv.FormatInt(payload.RuntimeMs, 10)

	if len(payload.Result) > 0 && string(payload.Result) != "null" {
		resultStr := strings.TrimSpace(string(payload.Result))
		var decoded any
		if err := json.Unmarshal(payload.Result, &decoded); err == nil {
			switch v := decoded.(type) {
			case string:
				resultStr = v
			default:
				if b, err := json.MarshalIndent(v, "", "  "); err == nil {
					resultStr = string(b)
				}
			}
		}
		if p, tr := capBytes(resultStr, 2000); p != "" {
			respData["result"] = p
			storeResp["result"] = p
			outputPreview = p
			if tr {
				respData["resultPreviewTruncated"] = "true"
				storeResp["resultPreviewTruncated"] = "true"
			}
		}
	}
	if payload.ResultTruncated {
		respData["resultTruncated"] = "true"
		storeResp["resultTruncated"] = "true"
	}
	if payload.StdoutTruncated {
		respData["stdoutTruncated"] = "true"
		storeResp["stdoutTruncated"] = "true"
	}
	if payload.StderrTruncated {
		respData["stderrTruncated"] = "true"
		storeResp["stderrTruncated"] = "true"
	}
	if payload.ResultTruncated || payload.StdoutTruncated || payload.StderrTruncated {
		respData["truncated"] = "true"
		storeResp["truncated"] = "true"
	}
	if strings.TrimSpace(outputPreview) == "" {
		if v := strings.TrimSpace(respData["stdout"]); v != "" {
			outputPreview = v
		} else if v := strings.TrimSpace(respData["stderr"]); v != "" {
			outputPreview = v
		}
	}
	if strings.TrimSpace(outputPreview) != "" {
		if p, tr := capBytes(outputPreview, 300); p != "" {
			respData["outputPreview"] = p
			storeResp["outputPreview"] = p
			if tr {
				respData["outputPreviewTruncated"] = "true"
				storeResp["outputPreviewTruncated"] = "true"
			}
		}
	}
}

type httpFetchOperation struct{}

func (httpFetchOperation) Op() string { return types.HostOpHTTPFetch }
func (httpFetchOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (httpFetchOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (httpFetchOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
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
	return opformat.FormatRequestText(reqData)
}
func (browserOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (browserOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	if len(req.Input) == 0 {
		return
	}
	var bReq struct {
		Action      string   `json:"action"`
		SessionID   string   `json:"sessionId"`
		URL         string   `json:"url"`
		WaitType    string   `json:"waitType"`
		State       string   `json:"state"`
		SleepMs     *int     `json:"sleepMs"`
		Selector    string   `json:"selector"`
		WaitFor     string   `json:"waitFor"`
		Attribute   string   `json:"attribute"`
		Kind        string   `json:"kind"`
		Mode        string   `json:"mode"`
		MaxClicks   *int     `json:"maxClicks"`
		AutoDismiss *bool    `json:"autoDismiss"`
		TimeoutMs   *int     `json:"timeoutMs"`
		Headless    *bool    `json:"headless"`
		FullPage    *bool    `json:"fullPage"`
		UserAgent   string   `json:"userAgent"`
		ViewportW   *int     `json:"viewportWidth"`
		ViewportH   *int     `json:"viewportHeight"`
		ExpectPopup *bool    `json:"expectPopup"`
		SetActive   *bool    `json:"setActive"`
		PageID      string   `json:"pageId"`
		Key         string   `json:"key"`
		DX          *int     `json:"dx"`
		DY          *int     `json:"dy"`
		Value       string   `json:"value"`
		Values      []string `json:"values"`
		FilePath    string   `json:"filePath"`
		Filename    string   `json:"filename"`
		// NOTE: We intentionally do not log "text" to avoid leaking secrets.
	}
	if err := json.Unmarshal(req.Input, &bReq); err != nil {
		return
	}
	if a := strings.TrimSpace(bReq.Action); a != "" {
		reqData["action"] = a
		storeReq["action"] = a
	}
	if sid := strings.TrimSpace(bReq.SessionID); sid != "" {
		reqData["sessionId"] = sid
		storeReq["sessionId"] = sid
	}
	if u := strings.TrimSpace(bReq.URL); u != "" {
		reqData["url"] = u
		storeReq["url"] = u
	}
	if sel := strings.TrimSpace(bReq.Selector); sel != "" {
		if p, tr := capBytes(singleLine(sel), 200); p != "" {
			reqData["selector"] = p
			storeReq["selector"] = p
			if tr {
				reqData["selectorTruncated"] = "true"
				storeReq["selectorTruncated"] = "true"
			}
		}
	}
	if wf := strings.TrimSpace(bReq.WaitFor); wf != "" {
		if p, tr := capBytes(singleLine(wf), 200); p != "" {
			reqData["waitFor"] = p
			storeReq["waitFor"] = p
			if tr {
				reqData["waitForTruncated"] = "true"
				storeReq["waitForTruncated"] = "true"
			}
		}
	}
	if attr := strings.TrimSpace(bReq.Attribute); attr != "" {
		reqData["attribute"] = attr
		storeReq["attribute"] = attr
	}
	if k := strings.TrimSpace(bReq.Kind); k != "" {
		reqData["kind"] = k
		storeReq["kind"] = k
	}
	if mo := strings.TrimSpace(bReq.Mode); mo != "" {
		reqData["mode"] = mo
		storeReq["mode"] = mo
	}
	if bReq.MaxClicks != nil {
		reqData["maxClicks"] = strconv.Itoa(*bReq.MaxClicks)
		storeReq["maxClicks"] = strconv.Itoa(*bReq.MaxClicks)
	}
	if strings.TrimSpace(bReq.WaitType) != "" {
		reqData["waitType"] = strings.TrimSpace(bReq.WaitType)
		storeReq["waitType"] = strings.TrimSpace(bReq.WaitType)
	}
	if strings.TrimSpace(bReq.State) != "" {
		reqData["state"] = strings.TrimSpace(bReq.State)
		storeReq["state"] = strings.TrimSpace(bReq.State)
	}
	if bReq.SleepMs != nil {
		reqData["sleepMs"] = strconv.Itoa(*bReq.SleepMs)
		storeReq["sleepMs"] = strconv.Itoa(*bReq.SleepMs)
	}
	if bReq.TimeoutMs != nil {
		reqData["timeoutMs"] = strconv.Itoa(*bReq.TimeoutMs)
		storeReq["timeoutMs"] = strconv.Itoa(*bReq.TimeoutMs)
	}
	if bReq.AutoDismiss != nil {
		reqData["autoDismiss"] = fmtBool(*bReq.AutoDismiss)
		storeReq["autoDismiss"] = fmtBool(*bReq.AutoDismiss)
	}
	if bReq.Headless != nil {
		reqData["headless"] = fmtBool(*bReq.Headless)
		storeReq["headless"] = fmtBool(*bReq.Headless)
	}
	if bReq.FullPage != nil {
		reqData["fullPage"] = fmtBool(*bReq.FullPage)
		storeReq["fullPage"] = fmtBool(*bReq.FullPage)
	}
	if strings.TrimSpace(bReq.UserAgent) != "" {
		if p, tr := capBytes(singleLine(bReq.UserAgent), 160); p != "" {
			reqData["userAgent"] = p
			storeReq["userAgent"] = p
			if tr {
				reqData["userAgentTruncated"] = "true"
				storeReq["userAgentTruncated"] = "true"
			}
		}
	}
	if bReq.ViewportW != nil {
		reqData["viewportWidth"] = strconv.Itoa(*bReq.ViewportW)
		storeReq["viewportWidth"] = strconv.Itoa(*bReq.ViewportW)
	}
	if bReq.ViewportH != nil {
		reqData["viewportHeight"] = strconv.Itoa(*bReq.ViewportH)
		storeReq["viewportHeight"] = strconv.Itoa(*bReq.ViewportH)
	}
	if bReq.ExpectPopup != nil {
		reqData["expectPopup"] = fmtBool(*bReq.ExpectPopup)
		storeReq["expectPopup"] = fmtBool(*bReq.ExpectPopup)
	}
	if bReq.SetActive != nil {
		reqData["setActive"] = fmtBool(*bReq.SetActive)
		storeReq["setActive"] = fmtBool(*bReq.SetActive)
	}
	if strings.TrimSpace(bReq.PageID) != "" {
		reqData["pageId"] = strings.TrimSpace(bReq.PageID)
		storeReq["pageId"] = strings.TrimSpace(bReq.PageID)
	}
	if strings.TrimSpace(bReq.Key) != "" {
		reqData["key"] = strings.TrimSpace(bReq.Key)
		storeReq["key"] = strings.TrimSpace(bReq.Key)
	}
	if bReq.DX != nil {
		reqData["dx"] = strconv.Itoa(*bReq.DX)
		storeReq["dx"] = strconv.Itoa(*bReq.DX)
	}
	if bReq.DY != nil {
		reqData["dy"] = strconv.Itoa(*bReq.DY)
		storeReq["dy"] = strconv.Itoa(*bReq.DY)
	}
	if strings.TrimSpace(bReq.Value) != "" {
		reqData["value"] = strings.TrimSpace(bReq.Value)
		storeReq["value"] = strings.TrimSpace(bReq.Value)
	}
	if len(bReq.Values) != 0 {
		reqData["valuesCount"] = strconv.Itoa(len(bReq.Values))
		storeReq["valuesCount"] = strconv.Itoa(len(bReq.Values))
	}
	if strings.TrimSpace(bReq.FilePath) != "" {
		reqData["filePath"] = "<omitted>"
		storeReq["filePath"] = "<omitted>"
	}
	if strings.TrimSpace(bReq.Filename) != "" {
		reqData["filename"] = strings.TrimSpace(bReq.Filename)
		storeReq["filename"] = strings.TrimSpace(bReq.Filename)
	}
	reqData["text"] = "<omitted>"
	storeReq["text"] = "<omitted>"
}
func (browserOperation) EnrichResponseEvent(_ types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, storeResp map[string]string) {
	respData["browserOp"] = resp.Op
	if strings.TrimSpace(resp.Text) == "" {
		return
	}
	var mOut map[string]any
	if err := json.Unmarshal([]byte(resp.Text), &mOut); err == nil {
		if v, ok := mOut["sessionId"].(string); ok && strings.TrimSpace(v) != "" {
			respData["sessionId"] = strings.TrimSpace(v)
			storeResp["sessionId"] = strings.TrimSpace(v)
		}
		if v, ok := mOut["pageId"].(string); ok && strings.TrimSpace(v) != "" {
			respData["pageId"] = strings.TrimSpace(v)
			storeResp["pageId"] = strings.TrimSpace(v)
		}
		if v, ok := mOut["title"].(string); ok && strings.TrimSpace(v) != "" {
			if p, tr := capBytes(singleLine(v), 200); p != "" {
				respData["title"] = p
				if tr {
					respData["titleTruncated"] = "true"
				}
			}
		}
		if v, ok := mOut["url"].(string); ok && strings.TrimSpace(v) != "" {
			respData["url"] = strings.TrimSpace(v)
			storeResp["url"] = strings.TrimSpace(v)
		}
		if v, ok := mOut["path"].(string); ok && strings.TrimSpace(v) != "" {
			respData["path"] = strings.TrimSpace(v)
			storeResp["path"] = strings.TrimSpace(v)
		}
		if v, ok := mOut["count"].(float64); ok {
			respData["count"] = strconv.Itoa(int(v))
			storeResp["count"] = strconv.Itoa(int(v))
		}
		if v, ok := mOut["dismissCount"].(float64); ok {
			respData["dismissCount"] = strconv.Itoa(int(v))
			storeResp["dismissCount"] = strconv.Itoa(int(v))
		}
		if v, ok := mOut["suggestedFilename"].(string); ok && strings.TrimSpace(v) != "" {
			respData["suggestedFilename"] = strings.TrimSpace(v)
			storeResp["suggestedFilename"] = strings.TrimSpace(v)
		}
	} else if resp.Op == "browser.extract" || resp.Op == "browser.extract_links" || resp.Op == "browser.tab_list" {
		respData["bytes"] = strconv.Itoa(len(resp.Text))
		storeResp["bytes"] = strconv.Itoa(len(resp.Text))
	}

	if resp.Op == "browser.extract" || resp.Op == "browser.extract_links" {
		var arr []any
		if err := json.Unmarshal([]byte(resp.Text), &arr); err == nil {
			respData["items"] = strconv.Itoa(len(arr))
			storeResp["items"] = strconv.Itoa(len(arr))
		}
	}
}

type traceRunOperation struct{}

func (traceRunOperation) Op() string { return types.HostOpTrace }
func (traceRunOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (traceRunOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (traceRunOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (traceRunOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, _ map[string]string) {
	reqData["traceAction"] = req.Action
	if len(req.Input) > 0 {
		reqData["traceInput"] = string(req.Input)
	}
}
func (traceRunOperation) EnrichResponseEvent(_ types.HostOpRequest, resp types.HostOpResponse, respData map[string]string, _ map[string]string) {
	if resp.Text == "" {
		return
	}
	s, tr := capBytes(resp.Text, 1000)
	respData["output"] = s
	if tr {
		respData["outputTruncated"] = "true"
	}
}

type emailOperation struct{}

func (emailOperation) Op() string { return types.HostOpEmail }
func (emailOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (emailOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (emailOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (emailOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	if len(req.Input) == 0 {
		return
	}
	var emailReq struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
	}
	if err := json.Unmarshal(req.Input, &emailReq); err != nil {
		return
	}
	if to := strings.TrimSpace(emailReq.To); to != "" {
		if p, tr := capBytes(singleLine(to), 200); p != "" {
			reqData["to"] = p
			storeReq["to"] = p
			if tr {
				reqData["toTruncated"] = "true"
				storeReq["toTruncated"] = "true"
			}
		}
	}
	if subject := strings.TrimSpace(emailReq.Subject); subject != "" {
		if p, tr := capBytes(singleLine(subject), 200); p != "" {
			reqData["subject"] = p
			storeReq["subject"] = p
			if tr {
				reqData["subjectTruncated"] = "true"
				storeReq["subjectTruncated"] = "true"
			}
		}
	}
}

type noopOperation struct{}

func (noopOperation) Op() string { return types.HostOpNoop }
func (noopOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (noopOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (noopOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
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
	return opformat.FormatRequestText(reqData)
}
func (toolResultOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
func (toolResultOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, storeReq map[string]string) {
	if strings.TrimSpace(req.Tag) != "task_create" {
		return
	}
	reqData["op"] = "task_create"
	storeReq["op"] = "task_create"
	if len(req.Input) == 0 {
		return
	}
	var payload struct {
		Goal       string `json:"goal"`
		TaskID     string `json:"taskId"`
		ChildRunID string `json:"childRunId"`
	}
	if err := json.Unmarshal(req.Input, &payload); err != nil {
		return
	}
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
func (toolResultOperation) EnrichResponseEvent(req types.HostOpRequest, _ types.HostOpResponse, respData map[string]string, storeResp map[string]string) {
	if strings.TrimSpace(req.Tag) != "task_create" {
		return
	}
	respData["op"] = "task_create"
	storeResp["op"] = "task_create"
}

type agentFinalOperation struct{}

func (agentFinalOperation) Op() string { return types.HostOpFinal }
func (agentFinalOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (agentFinalOperation) FormatRequestText(_ types.HostOpRequest, reqData map[string]string) string {
	return opformat.FormatRequestText(reqData)
}
func (agentFinalOperation) FormatResponseText(_ types.HostOpRequest, _ types.HostOpResponse, _ map[string]string, respData map[string]string) string {
	return opformat.FormatResponseText(respData)
}
