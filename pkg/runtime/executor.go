package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/tinoosan/agen8/pkg/agent"
	"github.com/tinoosan/agen8/pkg/debuglog"
	"github.com/tinoosan/agen8/pkg/events"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
)

type HostOpObserver interface {
	ObserveHostOp(req types.HostOpRequest, resp types.HostOpResponse)
}

type HostOpMiddleware interface {
	Handle(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse
}

type ExecutorOptions struct {
	Emit            func(ctx context.Context, ev events.Event)
	Model           string
	RunID           string
	SessionID       string
	FS              *vfs.FS
	Guard           func(req types.HostOpRequest) *types.HostOpResponse
	Observers       []HostOpObserver
	ArtifactObserve func(path string)
	Operations      []HostOperation
}

// ChainExecutor composes middleware around a base executor.
func ChainExecutor(base agent.HostExecutor, middleware ...HostOpMiddleware) agent.HostExecutor {
	if base == nil {
		return types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing"}
		})
	}
	if len(middleware) == 0 {
		return base
	}
	next := types.HostExecFunc(base.Exec)
	for i := len(middleware) - 1; i >= 0; i-- {
		mw := middleware[i]
		inner := next
		next = types.HostExecFunc(func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
			return mw.Handle(ctx, req, inner)
		})
	}
	return next
}

// NewExecutor constructs an executor with the standard middleware chain.
func NewExecutor(base *agent.HostOpExecutor, opts ExecutorOptions) agent.HostExecutor {
	var seq uint64
	metaKey := opContextKey{}
	ops := newHostOperationRegistry(opts.Operations)
	dispatchMW := &dispatchMiddleware{operations: ops}
	eventMW := &eventMiddleware{
		emit:       opts.Emit,
		model:      opts.Model,
		runID:      opts.RunID,
		sessionID:  opts.SessionID,
		seq:        &seq,
		metaKey:    metaKey,
		operations: ops,
	}
	diffMW := &diffMiddleware{
		fs:         opts.FS,
		metaKey:    metaKey,
		operations: ops,
	}
	observerMW := &observerMiddleware{
		observers:       opts.Observers,
		artifactObserve: opts.ArtifactObserve,
	}
	guardMW := &guardMiddleware{guard: opts.Guard}
	return ChainExecutor(base, dispatchMW, eventMW, diffMW, observerMW, guardMW)
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
	emit       func(ctx context.Context, ev events.Event)
	model      string
	runID      string
	sessionID  string
	seq        *uint64
	metaKey    opContextKey
	operations *hostOperationRegistry
}

func (m *eventMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	if m == nil || m.emit == nil {
		return next(ctx, req)
	}

	// Debug logs for specific context constructor reads.
	if req.Op == types.HostOpFSList && strings.TrimSpace(req.Path) == "/workspace" {
		debuglog.Log("context", "H9", "runtime:eventMiddleware", "fs_list_workspace", map[string]any{
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
	storeReq["opId"] = opID
	storeReq["op"] = req.Op
	storeReq["path"] = req.Path
	if action := strings.TrimSpace(req.Action); action != "" {
		reqData["action"] = action
		storeReq["action"] = action
	}
	if req.Tag != "" {
		reqData["tag"] = req.Tag
		storeReq["tag"] = req.Tag
	}

	reqOp := m.operations.Get(req.Op)
	if reqOp != nil {
		if fieldsProvider, ok := reqOp.(HostOpRequestStoreFields); ok {
			if fields := fieldsProvider.RequestStoreFields(req); len(fields) != 0 {
				for k, v := range fields {
					storeReq[k] = v
					reqData[k] = v
				}
			}
		}
		if enricher, ok := reqOp.(HostOpRequestEventEnricher); ok {
			enricher.EnrichRequestEvent(req, reqData, storeReq)
		}
		if txt := strings.TrimSpace(reqOp.FormatRequestText(req, reqData)); txt != "" {
			reqData["requestText"] = txt
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
		"opId": meta.OpID,
		"op":   resp.Op,
		"ok":   fmtBool(resp.Ok),
		"err":  resp.Error,
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
		meta.StoreResp["path"] = strings.TrimSpace(req.Path)
	}
	if resp.BytesLen != 0 {
		meta.RespData["bytesLen"] = strconv.Itoa(resp.BytesLen)
	}
	if resp.Truncated {
		meta.RespData["truncated"] = "true"
	}
	respOp := resolveOperationForResponse(m.operations, req, resp)
	if respOp != nil {
		if fieldsProvider, ok := respOp.(HostOpResponseStoreFields); ok {
			if fields := fieldsProvider.ResponseStoreFields(resp); len(fields) != 0 {
				for k, v := range fields {
					meta.StoreResp[k] = v
				}
			}
		}
		if enricher, ok := respOp.(HostOpResponseEventEnricher); ok {
			enricher.EnrichResponseEvent(req, resp, meta.RespData, meta.StoreResp)
		}
		if txt := strings.TrimSpace(respOp.FormatResponseText(req, resp, reqData, meta.RespData)); txt != "" {
			meta.RespData["responseText"] = txt
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
	fs         *vfs.FS
	metaKey    opContextKey
	operations *hostOperationRegistry
}

func (m *diffMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
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
	op := m.operations.Get(req.Op)
	resolver, ok := op.(HostOpDiffAfterResolver)
	if !ok {
		return resp
	}

	before := string(meta.BeforeBytes)
	after, ok := resolver.ResolveAfter(req, before, m.fs)
	if !ok {
		return resp
	}

	if after == "" && !meta.HadBefore {
		return resp
	}
	fromFile := "a" + strings.TrimSpace(req.Path)
	toFile := "b" + strings.TrimSpace(req.Path)
	if !meta.HadBefore {
		fromFile = "/dev/null"
	}
	before = strings.ReplaceAll(before, "\r\n", "\n")
	after = strings.ReplaceAll(after, "\r\n", "\n")
	edits := myers.ComputeEdits(span.URIFromPath(strings.TrimSpace(req.Path)), before, after)
	ud := gotextdiff.ToUnified(fromFile, toFile, before, edits)
	diffText := fmt.Sprintf("%s", ud)
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

func (m *observerMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
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

type dispatchMiddleware struct {
	operations *hostOperationRegistry
}

func (m *dispatchMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	if m == nil || m.operations == nil {
		return next(ctx, req)
	}
	op := m.operations.Get(req.Op)
	if op == nil {
		return next(ctx, req)
	}
	return op.Execute(ctx, req, next)
}

func (m *guardMiddleware) Handle(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	if m == nil || m.guard == nil {
		return next(ctx, req)
	}
	if resp := m.guard(req); resp != nil {
		return *resp
	}
	return next(ctx, req)
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

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
