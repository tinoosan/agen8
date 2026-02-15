package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/tinoosan/workbench-core/pkg/agent"
	"github.com/tinoosan/workbench-core/pkg/debuglog"
	"github.com/tinoosan/workbench-core/pkg/events"
	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
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
	if req.Op == types.HostOpFSRead && strings.TrimSpace(req.Path) == "/workspace/context_constructor_manifest.json" {
		debuglog.Log("context", "H9", "runtime:eventMiddleware", "fs_read_context_constructor_manifest", map[string]any{
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
	if req.Tag != "" {
		reqData["tag"] = req.Tag
		storeReq["tag"] = req.Tag
	}

	if req.Op == types.HostOpFSSearch {
		if strings.TrimSpace(req.Query) != "" {
			reqData["query"] = strings.TrimSpace(req.Query)
			storeReq["query"] = strings.TrimSpace(req.Query)
		}
		if req.Limit != 0 {
			reqData["limit"] = strconv.Itoa(req.Limit)
			storeReq["limit"] = strconv.Itoa(req.Limit)
		}
	}
	if op := m.operations.Get(req.Op); op != nil {
		if fieldsProvider, ok := op.(HostOpRequestStoreFields); ok {
			if fields := fieldsProvider.RequestStoreFields(req); len(fields) != 0 {
				for k, v := range fields {
					storeReq[k] = v
					reqData[k] = v
				}
			}
		}
		if enricher, ok := op.(HostOpRequestEventEnricher); ok {
			enricher.EnrichRequestEvent(req, reqData, storeReq)
		}
	}
	if req.Op == types.HostOpTrace {
		reqData["traceAction"] = req.Action
		if len(req.Input) > 0 {
			reqData["traceInput"] = string(req.Input)
		}
	}
	if req.Op == types.HostOpBrowser && len(req.Input) > 0 {
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
		if err := json.Unmarshal(req.Input, &bReq); err == nil {
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
	}
	if req.Op == types.HostOpEmail && len(req.Input) > 0 {
		var emailReq struct {
			To      string `json:"to"`
			Subject string `json:"subject"`
		}
		if err := json.Unmarshal(req.Input, &emailReq); err == nil {
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
	if resp.Op == types.HostOpShellExec {
		meta.RespData["exitCode"] = strconv.Itoa(resp.ExitCode)
		meta.RespData["vfsPathTranslated"] = fmtBool(resp.VFSPathTranslated)
		meta.StoreResp["vfsPathTranslated"] = fmtBool(resp.VFSPathTranslated)
		if mounts := strings.TrimSpace(resp.VFSPathMounts); mounts != "" {
			meta.RespData["vfsPathMounts"] = mounts
			meta.StoreResp["vfsPathMounts"] = mounts
		}
		meta.RespData["scriptPathNormalized"] = fmtBool(resp.ScriptPathNormalized)
		anti := strings.TrimSpace(resp.ScriptAntiPattern)
		if anti == "" {
			anti = "none"
		}
		meta.RespData["scriptAntiPattern"] = anti
		meta.StoreResp["scriptPathNormalized"] = fmtBool(resp.ScriptPathNormalized)
		meta.StoreResp["scriptAntiPattern"] = anti
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
		if strings.TrimSpace(resp.Warning) != "" {
			if s, tr := capBytes(resp.Warning, 300); s != "" {
				meta.RespData["warning"] = s
				if tr {
					meta.RespData["warningTruncated"] = "true"
				}
			}
		}
	}
	if op := m.operations.Get(resp.Op); op != nil {
		if fieldsProvider, ok := op.(HostOpResponseStoreFields); ok {
			if fields := fieldsProvider.ResponseStoreFields(resp); len(fields) != 0 {
				for k, v := range fields {
					meta.StoreResp[k] = v
				}
			}
		}
		if enricher, ok := op.(HostOpResponseEventEnricher); ok {
			enricher.EnrichResponseEvent(req, resp, meta.RespData, meta.StoreResp)
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
	if resp.Op == types.HostOpFSSearch {
		meta.RespData["results"] = strconv.Itoa(len(resp.Results))
	}
	if strings.HasPrefix(resp.Op, "browser.") {
		meta.RespData["browserOp"] = resp.Op
		if strings.TrimSpace(resp.Text) != "" {
			var mOut map[string]any
			if err := json.Unmarshal([]byte(resp.Text), &mOut); err == nil {
				if v, ok := mOut["sessionId"].(string); ok && strings.TrimSpace(v) != "" {
					meta.RespData["sessionId"] = strings.TrimSpace(v)
					meta.StoreResp["sessionId"] = strings.TrimSpace(v)
				}
				if v, ok := mOut["pageId"].(string); ok && strings.TrimSpace(v) != "" {
					meta.RespData["pageId"] = strings.TrimSpace(v)
					meta.StoreResp["pageId"] = strings.TrimSpace(v)
				}
				if v, ok := mOut["title"].(string); ok && strings.TrimSpace(v) != "" {
					if p, tr := capBytes(singleLine(v), 200); p != "" {
						meta.RespData["title"] = p
						if tr {
							meta.RespData["titleTruncated"] = "true"
						}
					}
				}
				if v, ok := mOut["url"].(string); ok && strings.TrimSpace(v) != "" {
					meta.RespData["url"] = strings.TrimSpace(v)
					meta.StoreResp["url"] = strings.TrimSpace(v)
				}
				if v, ok := mOut["path"].(string); ok && strings.TrimSpace(v) != "" {
					meta.RespData["path"] = strings.TrimSpace(v)
					meta.StoreResp["path"] = strings.TrimSpace(v)
				}
				if v, ok := mOut["count"].(float64); ok {
					meta.RespData["count"] = strconv.Itoa(int(v))
					meta.StoreResp["count"] = strconv.Itoa(int(v))
				}
				if v, ok := mOut["dismissCount"].(float64); ok {
					meta.RespData["dismissCount"] = strconv.Itoa(int(v))
					meta.StoreResp["dismissCount"] = strconv.Itoa(int(v))
				}
				if v, ok := mOut["suggestedFilename"].(string); ok && strings.TrimSpace(v) != "" {
					meta.RespData["suggestedFilename"] = strings.TrimSpace(v)
					meta.StoreResp["suggestedFilename"] = strings.TrimSpace(v)
				}
			} else if resp.Op == "browser.extract" || resp.Op == "browser.extract_links" || resp.Op == "browser.tab_list" {
				meta.RespData["bytes"] = strconv.Itoa(len(resp.Text))
				meta.StoreResp["bytes"] = strconv.Itoa(len(resp.Text))
			}

			if resp.Op == "browser.extract" || resp.Op == "browser.extract_links" {
				var arr []any
				if err := json.Unmarshal([]byte(resp.Text), &arr); err == nil {
					meta.RespData["items"] = strconv.Itoa(len(arr))
					meta.StoreResp["items"] = strconv.Itoa(len(arr))
				}
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
