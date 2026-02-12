package runtime

import (
	"context"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/pkg/types"
	"github.com/tinoosan/workbench-core/pkg/vfs"
)

// HostOperation defines a strategy for a single host op type.
type HostOperation interface {
	Op() string
	Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse
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
		fsReadOperation{},
		fsWriteOperation{},
		fsAppendOperation{},
		fsEditOperation{},
		shellExecOperation{},
		httpFetchOperation{},
	}
}

type fsReadOperation struct{}

func (fsReadOperation) Op() string { return types.HostOpFSRead }
func (fsReadOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsReadOperation) EnrichRequestEvent(req types.HostOpRequest, reqData map[string]string, _ map[string]string) {
	if req.MaxBytes != 0 {
		reqData["maxBytes"] = strconv.Itoa(req.MaxBytes)
	}
}

type fsWriteOperation struct{}

func (fsWriteOperation) Op() string { return types.HostOpFSWrite }
func (fsWriteOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsWriteOperation) ResolveAfter(req types.HostOpRequest, _ string, _ *vfs.FS) (string, bool) {
	return req.Text, true
}

type fsAppendOperation struct{}

func (fsAppendOperation) Op() string { return types.HostOpFSAppend }
func (fsAppendOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (fsAppendOperation) ResolveAfter(req types.HostOpRequest, before string, _ *vfs.FS) (string, bool) {
	return before + req.Text, true
}

type fsEditOperation struct{}

func (fsEditOperation) Op() string { return types.HostOpFSEdit }
func (fsEditOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
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

type shellExecOperation struct{}

func (shellExecOperation) Op() string { return types.HostOpShellExec }
func (shellExecOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
}
func (shellExecOperation) RequestStoreFields(req types.HostOpRequest) map[string]string {
	return shellArgsToFields(req.Argv, req.Cwd)
}
func (shellExecOperation) ResponseStoreFields(resp types.HostOpResponse) map[string]string {
	return map[string]string{
		"exitCode": strconv.Itoa(resp.ExitCode),
	}
}

type httpFetchOperation struct{}

func (httpFetchOperation) Op() string { return types.HostOpHTTPFetch }
func (httpFetchOperation) Execute(ctx context.Context, req types.HostOpRequest, next types.HostExecFunc) types.HostOpResponse {
	return next(ctx, req)
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
