package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/pmezard/go-difflib/difflib"
	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/debuglog"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

type HostOpObserver interface {
	ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse)
}

type HostOpMiddleware interface {
	Handle(ctx context.Context, req types.HostOpRequest, next HostExecFunc) types.HostOpResponse
}

type HostExecFunc func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse

func (f HostExecFunc) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	return f(ctx, req)
}

type ExecutorOptions struct {
	Emit           func(ctx context.Context, ev events.Event)
	Model          string
	RunID          string
	SessionID      string
	FS             *vfs.FS
	Guard          func(req types.HostOpRequest) *types.HostOpResponse
	Observers      []HostOpObserver
	ArtifactObserve func(path string)
}

// ChainExecutor composes middleware around a base executor.
func ChainExecutor(base agent.HostExecutor, middleware ...HostOpMiddleware) agent.HostExecutor {
	if base == nil {
		return HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing"}
		})
	}
	if len(middleware) == 0 {
		return base
	}
	next := HostExecFunc(base.Exec)
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		inner := next
		next = HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
			return mw.Handle(ctx, req, inner)
		})
	}
	return next
}

// NewExecutor constructs an executor with the standard middleware chain.
func NewExecutor(base *agent.HostOpExecutor, opts ExecutorOptions) agent.HostExecutor {
	var seq uint64
	metaKey := opContextKey{}
	eventMW := &eventMiddleware{
		emit:      opts.Emit,
		model:     opts.Model,
		runID:     opts.RunID,
		sessionID: opts.SessionID,
		seq:       &seq,
		metaKey:   metaKey,
	}
	diffMW := &diffMiddleware{
		fs:     opts.FS,
		metaKey: metaKey,
	}
	observerMW := &observerMiddleware{
		observers:      opts.Observers,
		artifactObserve: opts.ArtifactObserve,
	}
	guardMW := &guardMiddleware{guard: opts.Guard}
	return ChainExecutor(base, eventMW, diffMW, observerMW, guardMW)
}

type opContextKey struct{}

type opContext struct {
	OpID         string
	ReqData      map[string]string
	StoreReq     map[string]string
	RespData     map[string]string
	StoreResp    map[string]string
	BeforeBytes  []byte
	HadBefore    bool
	PatchPreview string
	PatchTrunc   bool
	PatchRedact  bool
}

type eventMiddleware struct {
	emit      func(ctx context.Context, ev events.Event)
	model     string
	runID     string
	sessionID string
	seq       *uint64
	metaKey   opContextKey
}

func (m *eventMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next HostExecFunc) types.HostOpResponse {
	if m == nil || m.emit == nil {
		return next(ctx, req)
	}

	// Debug logs for specific context constructor reads.
	if req.Op == types.HostOpFSList && strings.TrimSpace(req.Path) == "/scratch" {
		debuglog.Log("context", "H9", "runtime:eventMiddleware", "fs_list_workspace", map[string]any{
			"model":     strings.TrimSpace(m.model),
			"runId":     strings.TrimSpace(m.runID),
			"sessionId": strings.TrimSpace(m.sessionID),
		})
	}
	if req.Op == types.HostOpFSRead && strings.TrimSpace(req.Path) == "/scratch/context_constructor_manifest.json" {
		debuglog.Log("context", "H9", "runtime:eventMiddleware", "fs_read_context_constructor_manifest", map[string]any{
			"model":     strings.TrimSpace(m.model),
			"runId":     strings.TrimSpace(m.runID),
			"sessionId": strings.TrimSpace(m.sessionID),
		})
	}
	if req.Op == types.HostOpFSRead && strings.TrimSpace(req.Path) == "/results/context_constructor_manifest.json" {
		debuglog.Log("context", "H10", "runtime:eventMiddleware", "fs_read_context_constructor_manifest_results", map[string]any{
			"model":     strings.TrimSpace(m.model),
			"runId":     strings.TrimSpace(m.runID),
			"sessionId": strings.TrimSpace(m.sessionID),
		})
	}

	opID := fmt.Sprintf("op-%d", atomic.AddUint64(m.seq, 1))
	meta := &opContext{
		OpID:     opID,
		ReqData:  map[string]string{},
		StoreReq: map[string]string{},
	}
	ctx = context.WithValue(ctx, m.metaKey, meta)

	reqData := meta.ReqData
	storeReq := meta.StoreReq
	reqData["opId"] = opID
	reqData["op"] = req.Op
	reqData["path"] = req.Path
	reqData["toolId"] = req.ToolID.String()
	reqData["actionId"] = req.ActionID
	storeReq["op"] = req.Op
	storeReq["path"] = req.Path
	storeReq["toolId"] = req.ToolID.String()
	storeReq["actionId"] = req.ActionID

	if req.Op == types.HostOpFSRead && req.MaxBytes != 0 {
		reqData["maxBytes"] = strconv.Itoa(req.MaxBytes)
	}
	if req.Op == types.HostOpToolRun && req.TimeoutMs != 0 {
		reqData["timeoutMs"] = strconv.Itoa(req.TimeoutMs)
	}
	if req.Op == types.HostOpToolRun && len(req.Input) != 0 {
		s, tr, n := toolRunInputForEvent(req.Input)
		if s != "" {
			reqData["input"] = s
		}
		if tr {
			reqData["inputTruncated"] = "true"
		}
		if n != 0 {
			reqData["inputBytes"] = strconv.Itoa(n)
		}
	}
	if fields := shellStoreFieldsFromInput(req); len(fields) != 0 {
		for k, v := range fields {
			storeReq[k] = v
			reqData[k] = v
		}
	}
	if req.Op == types.HostOpTrace {
		reqData["traceAction"] = req.Action
		if len(req.Input) > 0 {
			reqData["traceInput"] = string(req.Input)
		}
	}
	if req.Op == types.HostOpHTTPFetch {
		reqData["url"] = req.URL
		if req.Method != "" {
			reqData["method"] = req.Method
		}
	}
	if (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) && strings.TrimSpace(req.Text) != "" {
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
	if req.Op == types.HostOpFSPatch && strings.TrimSpace(req.Text) != "" {
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

	m.emit(ctx, events.Event{
		Type:      "agent.op.request",
		Message:   "Agent requested host op",
		Data:      reqData,
		StoreData: storeReq,
	})

	resp := next(ctx, req)

	meta.RespData = map[string]string{
		"opId": meta.OpID,
		"op":   resp.Op,
		"ok":   fmtBool(resp.Ok),
		"err":  resp.Error,
	}
	meta.StoreResp = map[string]string{
		"op":  resp.Op,
		"ok":  fmtBool(resp.Ok),
		"err": resp.Error,
	}

	if meta.PatchPreview != "" {
		meta.RespData["patchPreview"] = meta.PatchPreview
		if meta.PatchTrunc {
			meta.RespData["patchTruncated"] = "true"
		}
		if meta.PatchRedact {
			meta.RespData["patchRedacted"] = "true"
		}
	}

	if strings.TrimSpace(req.Path) != "" {
		meta.RespData["path"] = strings.TrimSpace(req.Path)
	}
	if strings.TrimSpace(req.ToolID.String()) != "" {
		meta.RespData["toolId"] = strings.TrimSpace(req.ToolID.String())
	}
	if strings.TrimSpace(req.ActionID) != "" {
		meta.RespData["actionId"] = strings.TrimSpace(req.ActionID)
	}
	if resp.BytesLen != 0 {
		meta.RespData["bytesLen"] = strconv.Itoa(resp.BytesLen)
	}
	if resp.Truncated {
		meta.RespData["truncated"] = "true"
	}
	if resp.ToolResponse != nil && resp.ToolResponse.CallID != "" {
		meta.RespData["callId"] = resp.ToolResponse.CallID
		meta.StoreResp["callId"] = resp.ToolResponse.CallID
	}
	if resp.Op == types.HostOpToolRun && resp.ToolResponse != nil && len(resp.ToolResponse.Output) != 0 {
		if p := toolRunOutputPreviewForEvent(resp.ToolResponse.ToolID.String(), resp.ToolResponse.ActionID, resp.ToolResponse.Output); strings.TrimSpace(p) != "" {
			meta.RespData["outputPreview"] = p
		}
		if fields := shellStoreFieldsFromResponse(resp); len(fields) != 0 {
			for k, v := range fields {
				meta.StoreResp[k] = v
			}
		}
	}
	if resp.Op == types.HostOpShellExec {
		meta.RespData["exitCode"] = strconv.Itoa(resp.ExitCode)
		if resp.Stdout != "" {
			s, tr := capBytes(resp.Stdout, 1000)
			meta.RespData["stdout"] = s
			if tr {
				meta.RespData["stdoutTruncated"] = "true"
			}
		}
		if resp.Stderr != "" {
			s, tr := capBytes(resp.Stderr, 1000)
			meta.RespData["stderr"] = s
			if tr {
				meta.RespData["stderrTruncated"] = "true"
			}
		}
		if fields := shellStoreFieldsFromResponse(resp); len(fields) != 0 {
			for k, v := range fields {
				meta.StoreResp[k] = v
			}
		}
	}
	if resp.Op == types.HostOpTrace {
		if resp.Text != "" {
			s, tr := capBytes(resp.Text, 1000)
			meta.RespData["output"] = s
			if tr {
				meta.RespData["outputTruncated"] = "true"
			}
		}
	}
	if resp.Op == types.HostOpHTTPFetch {
		meta.RespData["status"] = strconv.Itoa(resp.Status)
		if resp.FinalURL != "" {
			meta.RespData["finalUrl"] = resp.FinalURL
		}
		if resp.Body != "" {
			s, tr := capBytes(resp.Body, 1000)
			meta.RespData["body"] = s
			if tr {
				meta.RespData["bodyTruncated"] = "true"
			}
		}
	}

	m.emit(ctx, events.Event{
		Type:      "agent.op.response",
		Message:   "Host op completed",
		Data:      meta.RespData,
		StoreData: meta.StoreResp,
	})
	return resp
}

type diffMiddleware struct {
	fs      *vfs.FS
	metaKey opContextKey
}

func (m *diffMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next HostExecFunc) types.HostOpResponse {
	meta, _ := ctx.Value(m.metaKey).(*opContext)
	if meta != nil && m.fs != nil {
		if (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend || req.Op == types.HostOpFSEdit || req.Op == types.HostOpFSPatch) && strings.TrimSpace(req.Path) != "" {
			if b, err := m.fs.Read(req.Path); err == nil {
				meta.BeforeBytes = b
				meta.HadBefore = true
			}
		}
	}

	resp := next(ctx, req)

	if meta == nil || m.fs == nil {
		return resp
	}
	if !resp.Ok || strings.TrimSpace(req.Path) == "" {
		return resp
	}
	if req.Op != types.HostOpFSWrite && req.Op != types.HostOpFSAppend && req.Op != types.HostOpFSEdit {
		return resp
	}

	before := string(meta.BeforeBytes)
	after := ""
	switch req.Op {
	case types.HostOpFSWrite:
		after = req.Text
	case types.HostOpFSAppend:
		after = before + req.Text
	case types.HostOpFSEdit:
		if b, err := m.fs.Read(req.Path); err == nil {
			after = string(b)
		}
	}

	if after == "" && !meta.HadBefore {
		return resp
	}
	fromFile := "a" + strings.TrimSpace(req.Path)
	toFile := "b" + strings.TrimSpace(req.Path)
	if !meta.HadBefore {
		fromFile = "/dev/null"
	}
	ud := difflib.UnifiedDiff{
		A:        difflib.SplitLines(strings.ReplaceAll(before, "\r\n", "\n")),
		B:        difflib.SplitLines(strings.ReplaceAll(after, "\r\n", "\n")),
		FromFile: fromFile,
		ToFile:   toFile,
		Context:  3,
	}
	diffText, _ := difflib.GetUnifiedDiffString(ud)
	diffText = strings.TrimSpace(diffText)
	if diffText == "" {
		return resp
	}
	if looksSensitiveText(diffText) {
		meta.PatchPreview = "<omitted>"
		meta.PatchRedact = true
		return resp
	}
	diffText, tr := capBytes(diffText, 12_000)
	meta.PatchPreview = diffText
	meta.PatchTrunc = tr
	return resp
}

type observerMiddleware struct {
	observers       []HostOpObserver
	artifactObserve func(path string)
}

func (m *observerMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next HostExecFunc) types.HostOpResponse {
	resp := next(ctx, req)
	if resp.Ok && (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) {
		if m.artifactObserve != nil {
			m.artifactObserve(req.Path)
		}
	}
	for _, obs := range m.observers {
		if obs != nil {
			obs.ObserveHostOp(req, resp)
		}
	}
	return resp
}

type guardMiddleware struct {
	guard func(req types.HostOpRequest) *types.HostOpResponse
}

func (m *guardMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next HostExecFunc) types.HostOpResponse {
	if m == nil || m.guard == nil {
		return next(ctx, req)
	}
	if resp := m.guard(req); resp != nil {
		return *resp
	}
	return next(ctx, req)
}

func shellStoreFieldsFromInput(req types.HostOpRequest) map[string]string {
	switch req.Op {
	case types.HostOpShellExec:
		return shellArgsToFields(req.Argv, req.Cwd)
	case types.HostOpToolRun:
		if strings.TrimSpace(req.ToolID.String()) != "builtin.shell" || strings.TrimSpace(req.ActionID) != "exec" {
			return nil
		}
		var in struct {
			Argv []string `json:"argv"`
			Cwd  string   `json:"cwd"`
		}
		if err := json.Unmarshal(req.Input, &in); err != nil {
			return nil
		}
		return shellArgsToFields(in.Argv, in.Cwd)
	default:
		return nil
	}
}

func shellArgsToFields(argv []string, cwd string) map[string]string {
	if len(argv) == 0 && strings.TrimSpace(cwd) == "" {
		return nil
	}
	out := map[string]string{}
	if len(argv) != 0 {
		out["argv0"] = argv[0]
		preview := singleLine(strings.Join(argv, " "))
		if len(argv) >= 3 && (argv[0] == "bash" || argv[0] == "sh") && argv[1] == "-c" {
			preview = singleLine(argv[2])
		}
		if p, tr := capBytes(preview, 160); p != "" {
			out["argvPreview"] = p
			if tr {
				out["argvPreviewTruncated"] = "true"
			}
		}
	}
	if cwd := strings.TrimSpace(cwd); cwd != "" {
		out["cwd"] = cwd
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func shellStoreFieldsFromResponse(resp types.HostOpResponse) map[string]string {
	switch resp.Op {
	case types.HostOpShellExec:
		fields := map[string]string{
			"exitCode": strconv.Itoa(resp.ExitCode),
		}
		if strings.TrimSpace(resp.StdoutPath) != "" {
			fields["stdoutPath"] = strings.TrimSpace(resp.StdoutPath)
		}
		if strings.TrimSpace(resp.StderrPath) != "" {
			fields["stderrPath"] = strings.TrimSpace(resp.StderrPath)
		}
		return fields
	case types.HostOpToolRun:
		if resp.ToolResponse == nil {
			return nil
		}
		if strings.TrimSpace(resp.ToolResponse.ToolID.String()) != "builtin.shell" || strings.TrimSpace(resp.ToolResponse.ActionID) != "exec" {
			return nil
		}
		fields := map[string]string{}
		if resp.ToolResponse.Error != nil && strings.TrimSpace(resp.ToolResponse.Error.Code) != "" {
			fields["errorCode"] = strings.TrimSpace(resp.ToolResponse.Error.Code)
		}
		if len(resp.ToolResponse.Output) != 0 {
			var out struct {
				ExitCode   int    `json:"exitCode"`
				StdoutPath string `json:"stdoutPath"`
				StderrPath string `json:"stderrPath"`
			}
			if err := json.Unmarshal(resp.ToolResponse.Output, &out); err == nil {
				fields["exitCode"] = strconv.Itoa(out.ExitCode)
				if strings.TrimSpace(out.StdoutPath) != "" {
					fields["stdoutPath"] = strings.TrimSpace(out.StdoutPath)
				}
				if strings.TrimSpace(out.StderrPath) != "" {
					fields["stderrPath"] = strings.TrimSpace(out.StderrPath)
				}
			}
		}
		if len(fields) == 0 {
			return nil
		}
		return fields
	default:
		return nil
	}
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
