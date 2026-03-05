package agent

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/tinoosan/agen8/pkg/checksumutil"
	"github.com/tinoosan/agen8/pkg/debuglog"
	"github.com/tinoosan/agen8/pkg/store"
	pkgtools "github.com/tinoosan/agen8/pkg/tools"
	"github.com/tinoosan/agen8/pkg/tools/builtins"
	"github.com/tinoosan/agen8/pkg/types"
	"github.com/tinoosan/agen8/pkg/vfs"
)

// BrowserManager abstracts interactive browser sessions for HostOpBrowser.
//
// It is an interface so tests can stub it and so the agent package does not
// depend on a specific browser implementation.
type BrowserManager interface {
	Start(ctx context.Context, headless bool, userAgent string, viewportWidth, viewportHeight int, extraHeaders map[string]string) (sessionID string, err error)
	Navigate(ctx context.Context, sessionID, url, waitFor string, timeoutMs int) (title string, finalURL string, err error)
	Wait(ctx context.Context, sessionID, waitType, url, selector, state string, timeoutMs int, sleepMs int) error
	// Dismiss attempts to dismiss cookie consent banners and popups.
	// kind: cookies|popups|all; mode: accept|reject|close; maxClicks caps total clicks across strategies.
	Dismiss(ctx context.Context, sessionID, kind, mode string, maxClicks int) (json.RawMessage, error)
	Click(ctx context.Context, sessionID, selector, waitFor string, expectPopup bool, timeoutMs int) (popupPageID, popupTitle, popupURL string, err error)
	Fill(ctx context.Context, sessionID, selector, text, waitFor string, timeoutMs int) error
	Hover(ctx context.Context, sessionID, selector string, timeoutMs int) error
	Press(ctx context.Context, sessionID, selector, key string, timeoutMs int) error
	Scroll(ctx context.Context, sessionID string, dx, dy int) error
	Select(ctx context.Context, sessionID, selector string, values []string, timeoutMs int) (json.RawMessage, error)
	SetChecked(ctx context.Context, sessionID, selector string, checked bool, timeoutMs int) error
	Upload(ctx context.Context, sessionID, selector, absPath string, timeoutMs int) error
	Download(ctx context.Context, sessionID, selector, absPath string, timeoutMs int) (json.RawMessage, error)
	GoBack(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error)
	GoForward(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error)
	Reload(ctx context.Context, sessionID string, timeoutMs int) (title string, finalURL string, err error)
	TabNew(ctx context.Context, sessionID, url string, setActive bool, timeoutMs int) (pageID, title, finalURL string, err error)
	TabList(ctx context.Context, sessionID string) (json.RawMessage, error)
	TabSwitch(ctx context.Context, sessionID, pageID string) error
	TabClose(ctx context.Context, sessionID, pageID string) error
	StorageSave(ctx context.Context, sessionID, absPath string) error
	StorageLoad(ctx context.Context, sessionID, absPath string) error
	SetExtraHeaders(ctx context.Context, sessionID string, headers map[string]string) error
	SetViewport(ctx context.Context, sessionID string, width, height int) error
	Extract(ctx context.Context, sessionID, selector, attribute string) (json.RawMessage, error)
	ExtractLinks(ctx context.Context, sessionID, selector string) (json.RawMessage, error)
	Screenshot(ctx context.Context, sessionID, absPath string, fullPage bool) error
	PDF(ctx context.Context, sessionID, absPath string) error
	Close(ctx context.Context, sessionID string) error
	CleanupStale()
	Shutdown() error
}

// HostOpExecutor is a tiny "host primitive" dispatcher for demos/tests.
//
// This is not the final host API; it is a concrete reference for the agent-facing
// request/response flow:
//   - fs_list/fs_stat/fs_read/fs_search/fs_write/fs_append are always available
type HostOpExecutor struct {
	FS *vfs.FS

	// Core invokers for direct host operations.
	ShellInvoker    pkgtools.ToolInvoker
	HTTPInvoker     pkgtools.ToolInvoker
	CodeExecInvoker pkgtools.ToolInvoker
	TraceInvoker    pkgtools.ToolInvoker // For all trace actions via BuiltinTraceInvoker
	Browser         BrowserManager
	EmailClient     builtins.EmailSender

	// WorkspaceDir is the host filesystem path backing the /workspace VFS mount.
	// It is used for browser screenshots and PDFs.
	WorkspaceDir string
	// ProjectDir is the host filesystem path backing the /project VFS mount.
	// It is used for browser uploads (e.g., resumes) when the agent references /project paths.
	ProjectDir string

	DefaultMaxBytes int

	// MaxReadBytes caps fs_read payload size returned to the model.
	//
	// This protects the model context window and cost from accidental "read the whole file"
	// requests (e.g. reading large HTML pages, logs, or binary blobs).
	//
	// If zero, no explicit cap is applied beyond DefaultMaxBytes / req.MaxBytes behavior.
	MaxReadBytes int
}

func debugAppendNDJSON(payload map[string]any) {
	// #region agent log
	// Debug-mode log sink (NDJSON).
	//
	// IMPORTANT: Do not log secrets; keep payloads small.
	f, err := debuglog.OpenLogFile()
	if err == nil {
		if b, jerr := json.Marshal(payload); jerr == nil {
			_, _ = f.Write(append(b, '\n'))
		}
		_ = f.Close()
	}
	// #endregion
}

func (x *HostOpExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
	if x == nil || x.FS == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "host executor missing FS"}
	}
	if err := req.Validate(); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	op, ok := mockHostOperationFor(req.Op)
	if !ok {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: fmt.Sprintf("unknown op %q", req.Op)}
	}
	return op.Exec(ctx, req, x)
}

type MockHostOperation interface {
	Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse
}

var (
	mockNoopOrToolResultOp = noopOrToolResultMockOp{}
	mockHostOperations     = map[string]MockHostOperation{
		types.HostOpNoop:       mockNoopOrToolResultOp,
		types.HostOpToolResult: mockNoopOrToolResultOp,

		types.HostOpFSList:   fsListMockOp{},
		types.HostOpFSStat:   fsStatMockOp{},
		types.HostOpFSRead:   fsReadMockOp{},
		types.HostOpFSSearch: fsSearchMockOp{},
		types.HostOpFSWrite:  fsWriteMockOp{},
		types.HostOpFSAppend: fsAppendMockOp{},
		types.HostOpFSEdit:   fsEditMockOp{},
		types.HostOpFSPatch:  fsPatchMockOp{},
		types.HostOpFSTxn:    fsTxnMockOp{},

		types.HostOpShellExec: shellExecMockOp{},
		types.HostOpHTTPFetch: httpFetchMockOp{},
		types.HostOpCodeExec:  codeExecMockOp{},
		types.HostOpTrace:     traceMockOp{},
		types.HostOpEmail:     emailMockOp{},
		types.HostOpBrowser:   browserMockOp{},
	}
)

func mockHostOperationFor(op string) (MockHostOperation, bool) {
	h, ok := mockHostOperations[op]
	return h, ok
}

type noopOrToolResultMockOp struct{}

func (noopOrToolResultMockOp) Exec(_ context.Context, req types.HostOpRequest, _ *HostOpExecutor) types.HostOpResponse {
	return types.HostOpResponse{Op: req.Op, Ok: true, Text: req.Text}
}

type fsListMockOp struct{}

func (fsListMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	entries, err := x.FS.List(req.Path)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, e.Path)
	}
	return types.HostOpResponse{Op: req.Op, Ok: true, Entries: out}
}

type fsStatMockOp struct{}

func (fsStatMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	entry, err := x.FS.Stat(req.Path)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			exists := false
			return types.HostOpResponse{
				Op:     req.Op,
				Ok:     true,
				Exists: &exists,
			}
		}
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	exists := true
	isDir := entry.IsDir
	resp := types.HostOpResponse{
		Op:     req.Op,
		Ok:     true,
		Exists: &exists,
		IsDir:  &isDir,
	}
	if !entry.IsDir && entry.HasSize {
		size := entry.Size
		resp.SizeBytes = &size
	}
	if entry.HasModTime {
		mtime := entry.ModTime.Unix()
		resp.Mtime = &mtime
	}
	return resp
}

type fsReadMockOp struct{}

func (fsReadMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	b, err := x.FS.Read(req.Path)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	maxBytes := req.MaxBytes
	if maxBytes == 0 {
		maxBytes = x.DefaultMaxBytes
	}
	if maxBytes <= 0 {
		maxBytes = 4096
	}
	if x.MaxReadBytes > 0 && maxBytes > x.MaxReadBytes {
		maxBytes = x.MaxReadBytes
	}
	text, b64, truncated := encodeReadPayload(b, maxBytes)
	readChecksums := readChecksumsForResponse(req.Checksum, req.Checksums, b)
	return types.HostOpResponse{
		Op:            req.Op,
		Ok:            true,
		BytesLen:      len(b),
		Text:          text,
		BytesB64:      b64,
		Truncated:     truncated,
		ReadChecksums: readChecksums,
	}
}

type fsSearchMockOp struct{}

func (fsSearchMockOp) Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	searchResp, err := x.FS.Search(ctx, req.Path, types.SearchRequest{
		Query:           req.Query,
		Pattern:         req.Pattern,
		Limit:           req.Limit,
		Glob:            req.Glob,
		Exclude:         req.Exclude,
		PreviewLines:    req.PreviewLines,
		IncludeMetadata: req.IncludeMetadata,
		MaxSizeBytes:    req.MaxSizeBytes,
	})
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{
		Op:               req.Op,
		Ok:               true,
		Results:          searchResp.Results,
		ResultsTotal:     searchResp.Total,
		ResultsReturned:  searchResp.Returned,
		ResultsTruncated: searchResp.Truncated,
	}
}

type fsWriteMockOp struct{}

func (fsWriteMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	writeBytes := []byte(req.Text)
	requestMode := normalizeWriteModeInput(req.Mode)
	writeMode := "overwritten"
	if requestMode == "a" {
		writeMode = "appended"
		if err := x.FS.Append(req.Path, writeBytes); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
	} else {
		if _, err := x.FS.Stat(req.Path); err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				writeMode = "created"
			} else {
				return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
			}
		}
		if err := x.FS.Write(req.Path, writeBytes); err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
	}

	written := int64(len(writeBytes))
	resp := types.HostOpResponse{
		Op:                   req.Op,
		Ok:                   true,
		WriteRequestMode:     requestMode,
		WriteMode:            writeMode,
		WriteBytes:           &written,
		WriteAtomicRequested: req.Atomic,
		WriteSyncRequested:   req.Sync,
	}
	if entry, err := x.FS.Stat(req.Path); err == nil && entry.HasSize {
		size := entry.Size
		resp.WriteFinalSize = &size
	}
	if algo := strings.ToLower(strings.TrimSpace(req.Checksum)); algo != "" {
		sum, err := writeChecksumHex(algo, writeBytes)
		if err != nil {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
		resp.WriteChecksumAlgo = algo
		resp.WriteChecksum = sum
		if expected := strings.TrimSpace(req.ChecksumExpected); expected != "" {
			resp.WriteChecksumExpected = expected
			matched := strings.EqualFold(sum, expected)
			resp.WriteChecksumMatch = &matched
			if !matched {
				return types.HostOpResponse{
					Op:                    req.Op,
					Ok:                    false,
					Error:                 fmt.Sprintf("checksum mismatch (%s): expected %s, got %s", algo, expected, sum),
					WriteChecksumAlgo:     resp.WriteChecksumAlgo,
					WriteChecksum:         resp.WriteChecksum,
					WriteChecksumExpected: resp.WriteChecksumExpected,
					WriteChecksumMatch:    resp.WriteChecksumMatch,
					WriteAtomicRequested:  resp.WriteAtomicRequested,
					WriteSyncRequested:    resp.WriteSyncRequested,
				}
			}
		}
	}

	if req.Verify {
		readBack, err := x.FS.Read(req.Path)
		if err != nil {
			verified := false
			resp.WriteVerified = &verified
			return types.HostOpResponse{
				Op:                   req.Op,
				Ok:                   false,
				Error:                fmt.Sprintf("verify failed: read-back error: %v", err),
				WriteVerified:        resp.WriteVerified,
				WriteChecksumAlgo:    resp.WriteChecksumAlgo,
				WriteChecksum:        resp.WriteChecksum,
				WriteAtomicRequested: resp.WriteAtomicRequested,
				WriteSyncRequested:   resp.WriteSyncRequested,
			}
		}
		expected := writeBytes
		actual := readBack
		if requestMode == "a" {
			if len(readBack) >= len(writeBytes) {
				actual = readBack[len(readBack)-len(writeBytes):]
			}
		}
		if !bytes.Equal(expected, actual) {
			verified := false
			mismatchAt := int64(firstMismatchOffset(expected, actual))
			expectedBytes := int64(len(expected))
			actualBytes := int64(len(actual))
			resp.WriteVerified = &verified
			resp.WriteMismatchAt = &mismatchAt
			resp.WriteExpectedBytes = &expectedBytes
			resp.WriteActualBytes = &actualBytes
			return types.HostOpResponse{
				Op:                   req.Op,
				Ok:                   false,
				Error:                fmt.Sprintf("verify failed: read-back mismatch at byte %d (expected %d bytes, got %d bytes)", mismatchAt, expectedBytes, actualBytes),
				WriteVerified:        resp.WriteVerified,
				WriteRequestMode:     resp.WriteRequestMode,
				WriteChecksumAlgo:    resp.WriteChecksumAlgo,
				WriteChecksum:        resp.WriteChecksum,
				WriteAtomicRequested: resp.WriteAtomicRequested,
				WriteSyncRequested:   resp.WriteSyncRequested,
				WriteMismatchAt:      resp.WriteMismatchAt,
				WriteExpectedBytes:   resp.WriteExpectedBytes,
				WriteActualBytes:     resp.WriteActualBytes,
			}
		}
		verified := true
		resp.WriteVerified = &verified
	}

	return resp
}

type fsAppendMockOp struct{}

func (fsAppendMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if err := x.FS.Append(req.Path, []byte(req.Text)); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: req.Op, Ok: true}
}

type fsEditMockOp struct{}

func (fsEditMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	beforeBytes, err := x.FS.Read(req.Path)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			beforeBytes = nil
		} else {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
	}

	before := string(beforeBytes)
	after, err := ApplyStructuredEdits(before, req.Input)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	if err := x.FS.Write(req.Path, []byte(after)); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: req.Op, Ok: true}
}

type fsPatchMockOp struct{}

func (fsPatchMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	beforeBytes, err := x.FS.Read(req.Path)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
			beforeBytes = nil
		} else {
			return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
		}
	}
	after, diag, err := ApplyUnifiedDiffWithDiagnostics(string(beforeBytes), req.Text, req.DryRun, req.Verbose)
	if err != nil {
		return types.HostOpResponse{
			Op:               req.Op,
			Ok:               false,
			Error:            err.Error(),
			PatchDryRun:      req.DryRun,
			PatchDiagnostics: &diag,
		}
	}
	if req.DryRun {
		return types.HostOpResponse{
			Op:               req.Op,
			Ok:               true,
			PatchDryRun:      true,
			PatchDiagnostics: &diag,
		}
	}
	if err := x.FS.Write(req.Path, []byte(after)); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{
		Op:               req.Op,
		Ok:               true,
		PatchDiagnostics: &diag,
	}
}

type fsTxnMockOp struct{}

func (fsTxnMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	dryRun := true
	apply := false
	rollbackOnError := true
	if req.TxnOptions != nil {
		if req.TxnOptions.DryRun {
			dryRun = true
		}
		if req.TxnOptions.Apply {
			apply = true
			dryRun = false
		}
		if !req.TxnOptions.RollbackOnError {
			rollbackOnError = false
		}
	}

	if !dryRun && !apply {
		dryRun = true
	}

	stepResults := make([]types.FSTxnStepResult, 0, len(req.TxnSteps))
	diag := types.FSTxnDiagnostics{
		StepsTotal: len(req.TxnSteps),
		ApplyMode:  "dry_run",
	}
	if apply {
		diag.ApplyMode = "apply"
	}

	type snapshot struct {
		existed bool
		content []byte
	}

	snapshots := map[string]snapshot{}
	touchedPaths := make([]string, 0, len(req.TxnSteps))

	type shadowFile struct {
		exists  bool
		content []byte
	}
	shadow := map[string]shadowFile{}

	loadShadow := func(path string) (shadowFile, error) {
		if current, ok := shadow[path]; ok {
			return current, nil
		}
		b, err := x.FS.Read(path)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				current := shadowFile{exists: false}
				shadow[path] = current
				return current, nil
			}
			return shadowFile{}, err
		}
		current := shadowFile{exists: true, content: append([]byte(nil), b...)}
		shadow[path] = current
		return current, nil
	}

	saveShadow := func(path string, content []byte) {
		shadow[path] = shadowFile{exists: true, content: append([]byte(nil), content...)}
	}

	captureSnapshot := func(path string) error {
		if _, ok := snapshots[path]; ok {
			return nil
		}
		b, err := x.FS.Read(path)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) || errors.Is(err, fs.ErrNotExist) {
				snapshots[path] = snapshot{existed: false}
				touchedPaths = append(touchedPaths, path)
				return nil
			}
			return err
		}
		snapshots[path] = snapshot{existed: true, content: append([]byte(nil), b...)}
		touchedPaths = append(touchedPaths, path)
		return nil
	}

	for i, step := range req.TxnSteps {
		result := types.FSTxnStepResult{
			Index: i + 1,
			Op:    step.Op,
			Path:  step.Path,
		}

		if apply {
			if err := captureSnapshot(step.Path); err != nil {
				result.Ok = false
				result.Error = err.Error()
				stepResults = append(stepResults, result)
				diag.FailedStep = i + 1
				break
			}
		}

		if dryRun {
			current, err := loadShadow(step.Path)
			if err != nil {
				result.Ok = false
				result.Error = err.Error()
				stepResults = append(stepResults, result)
				diag.FailedStep = i + 1
				break
			}
			nextText, stepSummary, err := executeTxnStep(step, string(current.content), current.exists, true)
			result.WriteMode = stepSummary.writeMode
			result.WriteBytes = stepSummary.writeBytes
			result.PatchDiagnostics = stepSummary.patchDiagnostics
			if err != nil {
				result.Ok = false
				result.Error = err.Error()
				stepResults = append(stepResults, result)
				diag.FailedStep = i + 1
				break
			}
			result.Ok = true
			stepResults = append(stepResults, result)
			diag.StepsApplied++
			saveShadow(step.Path, []byte(nextText))
			continue
		}

		stepReq := hostRequestFromTxnStep(step)
		handler, ok := mockHostOperationFor(stepReq.Op)
		if !ok {
			result.Ok = false
			result.Error = fmt.Sprintf("unsupported txn step op %q", stepReq.Op)
			stepResults = append(stepResults, result)
			diag.FailedStep = i + 1
			break
		}
		stepResp := handler.Exec(context.Background(), stepReq, x)
		result.WriteMode = strings.TrimSpace(stepResp.WriteMode)
		result.WriteBytes = stepResp.WriteBytes
		result.PatchDiagnostics = stepResp.PatchDiagnostics
		if !stepResp.Ok {
			result.Ok = false
			result.Error = strings.TrimSpace(stepResp.Error)
			stepResults = append(stepResults, result)
			diag.FailedStep = i + 1
			break
		}
		result.Ok = true
		stepResults = append(stepResults, result)
		diag.StepsApplied++
	}

	if diag.FailedStep != 0 && apply && rollbackOnError {
		diag.RollbackPerformed = true
		for i := len(touchedPaths) - 1; i >= 0; i-- {
			path := touchedPaths[i]
			snap := snapshots[path]
			if snap.existed {
				if err := x.FS.Write(path, snap.content); err != nil {
					diag.RollbackFailed = true
					diag.RollbackErrors = append(diag.RollbackErrors, fmt.Sprintf("%s: %v", path, err))
				}
				continue
			}
			if err := x.FS.Delete(path); err != nil {
				diag.RollbackFailed = true
				diag.RollbackErrors = append(diag.RollbackErrors, fmt.Sprintf("%s: %v", path, err))
			}
		}
	}

	ok := diag.FailedStep == 0 && !diag.RollbackFailed
	errMsg := ""
	if !ok {
		if diag.FailedStep != 0 && diag.FailedStep <= len(stepResults) {
			errMsg = strings.TrimSpace(stepResults[diag.FailedStep-1].Error)
		}
		if errMsg == "" {
			errMsg = "transaction failed"
		}
		if diag.RollbackFailed {
			errMsg += " (rollback failed)"
		}
	}

	return types.HostOpResponse{
		Op:             req.Op,
		Ok:             ok,
		Error:          errMsg,
		TxnStepResults: stepResults,
		TxnDiagnostics: &diag,
	}
}

type txnStepSummary struct {
	writeMode        string
	writeBytes       *int64
	patchDiagnostics *types.PatchDiagnostics
}

func executeTxnStep(step types.FSTxnStep, before string, beforeExists bool, dryRun bool) (string, txnStepSummary, error) {
	summary := txnStepSummary{}
	op := strings.ToLower(strings.TrimSpace(step.Op))
	switch op {
	case types.HostOpFSWrite:
		mode := normalizeWriteModeInput(step.Mode)
		switch mode {
		case "a":
			after := before + step.Text
			bytes := int64(len(step.Text))
			summary.writeBytes = &bytes
			summary.writeMode = "appended"
			return after, summary, nil
		default:
			after := step.Text
			bytes := int64(len(step.Text))
			summary.writeBytes = &bytes
			if beforeExists {
				summary.writeMode = "overwritten"
			} else {
				summary.writeMode = "created"
			}
			return after, summary, nil
		}
	case types.HostOpFSAppend:
		after := before + step.Text
		bytes := int64(len(step.Text))
		summary.writeBytes = &bytes
		summary.writeMode = "appended"
		return after, summary, nil
	case types.HostOpFSEdit:
		after, err := ApplyStructuredEdits(before, step.Input)
		if err != nil {
			return "", summary, err
		}
		bytes := int64(len(after))
		summary.writeBytes = &bytes
		if beforeExists {
			summary.writeMode = "overwritten"
		} else {
			summary.writeMode = "created"
		}
		return after, summary, nil
	case types.HostOpFSPatch:
		after, diag, err := ApplyUnifiedDiffWithDiagnostics(before, step.Text, dryRun, step.Verbose)
		summary.patchDiagnostics = &diag
		if err != nil {
			return "", summary, err
		}
		bytes := int64(len(after))
		summary.writeBytes = &bytes
		if beforeExists {
			summary.writeMode = "overwritten"
		} else {
			summary.writeMode = "created"
		}
		return after, summary, nil
	default:
		return "", summary, fmt.Errorf("unsupported txn step op %q", step.Op)
	}
}

func hostRequestFromTxnStep(step types.FSTxnStep) types.HostOpRequest {
	return types.HostOpRequest{
		Op:               strings.ToLower(strings.TrimSpace(step.Op)),
		Path:             step.Path,
		Text:             step.Text,
		Input:            step.Input,
		Mode:             step.Mode,
		Verify:           step.Verify,
		Checksum:         step.Checksum,
		ChecksumExpected: step.ChecksumExpected,
		Atomic:           step.Atomic,
		Sync:             step.Sync,
		Verbose:          step.Verbose,
	}
}

type shellExecMockOp struct{}

func (shellExecMockOp) Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if x.ShellInvoker == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "shell invoker not configured"}
	}
	payload := map[string]any{"argv": req.Argv}
	if strings.TrimSpace(req.Cwd) != "" {
		payload["cwd"] = req.Cwd
	}
	if req.Stdin != "" {
		payload["stdin"] = req.Stdin
	}
	inp, err := json.Marshal(payload)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	toolReq := pkgtools.ToolRequest{
		Version:  "v1",
		CallID:   "shell_exec",
		ToolID:   pkgtools.ToolID("builtin.shell"),
		ActionID: "exec",
		Input:    inp,
	}
	result, err := x.ShellInvoker.Invoke(ctx, toolReq)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	var out struct {
		ExitCode             int    `json:"exitCode"`
		Stdout               string `json:"stdout"`
		Stderr               string `json:"stderr"`
		Warning              string `json:"warning"`
		VFSPathTranslated    bool   `json:"vfsPathTranslated"`
		VFSPathMounts        string `json:"vfsPathMounts"`
		ScriptPathNormalized bool   `json:"scriptPathNormalized"`
		ScriptAntiPattern    string `json:"scriptAntiPattern"`
	}
	if err := json.Unmarshal(result.Output, &out); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	ok := out.ExitCode == 0
	errMsg := ""
	if !ok {
		errMsg = strings.TrimSpace(out.Stderr)
		if errMsg == "" {
			errMsg = fmt.Sprintf("shell_exec exited with code %d", out.ExitCode)
		}
	}
	return types.HostOpResponse{
		Op:                   req.Op,
		Ok:                   ok,
		Error:                errMsg,
		ExitCode:             out.ExitCode,
		Stdout:               out.Stdout,
		Stderr:               out.Stderr,
		Warning:              out.Warning,
		VFSPathTranslated:    out.VFSPathTranslated,
		VFSPathMounts:        strings.TrimSpace(out.VFSPathMounts),
		ScriptPathNormalized: out.ScriptPathNormalized,
		ScriptAntiPattern:    strings.TrimSpace(out.ScriptAntiPattern),
	}
}

type httpFetchMockOp struct{}

func (httpFetchMockOp) Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if x.HTTPInvoker == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "http invoker not configured"}
	}
	payload := map[string]any{"url": req.URL}
	if strings.TrimSpace(req.Method) != "" {
		payload["method"] = strings.TrimSpace(req.Method)
	}
	if req.Headers != nil {
		payload["headers"] = req.Headers
	}
	if req.Body != "" {
		payload["body"] = req.Body
	}
	if req.MaxBytes != 0 {
		payload["maxBytes"] = req.MaxBytes
	}
	if req.FollowRedirects != nil {
		payload["followRedirects"] = *req.FollowRedirects
	}
	inp, err := json.Marshal(payload)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	toolReq := pkgtools.ToolRequest{
		Version:  "v1",
		CallID:   "http_fetch",
		ToolID:   pkgtools.ToolID("builtin.http"),
		ActionID: "fetch",
		Input:    inp,
	}
	result, err := x.HTTPInvoker.Invoke(ctx, toolReq)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	var out struct {
		FinalURL      string              `json:"finalUrl"`
		Status        int                 `json:"status"`
		Headers       map[string][]string `json:"headers"`
		ContentType   string              `json:"contentType"`
		BytesRead     int                 `json:"bytesRead"`
		Truncated     bool                `json:"truncated"`
		Body          string              `json:"body"`
		BodyTruncated bool                `json:"bodyTruncated"`
		Warning       string              `json:"warning"`
	}
	if err := json.Unmarshal(result.Output, &out); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{
		Op:            req.Op,
		Ok:            true,
		FinalURL:      out.FinalURL,
		Status:        out.Status,
		Headers:       out.Headers,
		ContentType:   out.ContentType,
		BytesRead:     out.BytesRead,
		Truncated:     out.Truncated,
		Body:          out.Body,
		BodyTruncated: out.BodyTruncated,
		Warning:       out.Warning,
	}
}

type codeExecMockOp struct{}

func (codeExecMockOp) Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if x.CodeExecInvoker == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "code_exec invoker not configured"}
	}

	payload := map[string]any{
		"language": req.Language,
		"code":     req.Code,
	}
	if strings.TrimSpace(req.Cwd) != "" {
		payload["cwd"] = strings.TrimSpace(req.Cwd)
	}
	if req.TimeoutMs != 0 {
		payload["timeoutMs"] = req.TimeoutMs
	}
	if req.MaxBytes != 0 {
		payload["maxOutputBytes"] = req.MaxBytes
	}
	if len(req.Input) > 0 {
		var ext struct {
			MaxToolCalls *int `json:"maxToolCalls"`
		}
		if err := json.Unmarshal(req.Input, &ext); err == nil && ext.MaxToolCalls != nil {
			payload["maxToolCalls"] = *ext.MaxToolCalls
		}
	}

	inp, err := json.Marshal(payload)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	toolReq := pkgtools.ToolRequest{
		Version:   "v1",
		CallID:    "code_exec",
		ToolID:    pkgtools.ToolID("builtin.code_exec"),
		ActionID:  "run",
		Input:     inp,
		TimeoutMs: req.TimeoutMs,
	}
	result, err := x.CodeExecInvoker.Invoke(ctx, toolReq)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	var out struct {
		OK              bool   `json:"ok"`
		Error           string `json:"error"`
		Stdout          string `json:"stdout"`
		Stderr          string `json:"stderr"`
		StdoutTruncated bool   `json:"stdoutTruncated"`
		StderrTruncated bool   `json:"stderrTruncated"`
		ResultTruncated bool   `json:"resultTruncated"`
		ExitCode        int    `json:"exitCode"`
	}
	if err := json.Unmarshal(result.Output, &out); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}

	resp := types.HostOpResponse{
		Op:        req.Op,
		Ok:        out.OK,
		Error:     strings.TrimSpace(out.Error),
		Text:      string(result.Output),
		Stdout:    out.Stdout,
		Stderr:    out.Stderr,
		ExitCode:  out.ExitCode,
		Truncated: out.StdoutTruncated || out.StderrTruncated || out.ResultTruncated,
	}
	if !resp.Ok && resp.Error == "" {
		resp.Error = "code_exec failed"
	}
	return resp
}

type traceMockOp struct{}

func (traceMockOp) Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if x.TraceInvoker == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "trace invoker not configured"}
	}
	toolReq := pkgtools.ToolRequest{
		Version:  "v1",
		CallID:   "trace." + req.Action,
		ToolID:   pkgtools.ToolID("builtin.trace"),
		ActionID: req.Action,
		Input:    req.Input,
	}
	result, err := x.TraceInvoker.Invoke(ctx, toolReq)
	if err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: req.Op, Ok: true, Text: string(result.Output)}
}

type emailMockOp struct{}

func (emailMockOp) Exec(_ context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if x.EmailClient == nil {
		return types.HostOpResponse{
			Op:    req.Op,
			Ok:    false,
			Error: "email not configured (set GOOGLE_OAUTH_CLIENT_ID, GOOGLE_OAUTH_CLIENT_SECRET, GOOGLE_OAUTH_REFRESH_TOKEN, and GMAIL_USER)",
		}
	}
	var params struct {
		To      string `json:"to"`
		Subject string `json:"subject"`
		Body    string `json:"body"`
	}
	if err := json.Unmarshal(req.Input, &params); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	if err := x.EmailClient.Send(params.To, params.Subject, params.Body); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: req.Op, Ok: true}
}

type browserMockOp struct{}

type browserMockParams struct {
	Action      string   `json:"action"`
	SessionID   string   `json:"sessionId"`
	URL         string   `json:"url"`
	WaitType    string   `json:"waitType"`
	State       string   `json:"state"`
	SleepMs     *int     `json:"sleepMs"`
	Selector    string   `json:"selector"`
	Text        string   `json:"text"`
	WaitFor     string   `json:"waitFor"`
	Attribute   string   `json:"attribute"`
	Kind        string   `json:"kind"`
	Mode        string   `json:"mode"`
	MaxClicks   *int     `json:"maxClicks"`
	AutoDismiss *bool    `json:"autoDismiss"`
	TimeoutMs   *int     `json:"timeoutMs"`
	Headless    *bool    `json:"headless"`
	UserAgent   string   `json:"userAgent"`
	ViewportW   *int     `json:"viewportWidth"`
	ViewportH   *int     `json:"viewportHeight"`
	HeadersJSON string   `json:"headersJson"`
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
	FullPage    *bool    `json:"fullPage"`
}

type browserMockAction func(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse

var browserMockActions = map[string]browserMockAction{
	"start":         browserStartAction,
	"navigate":      browserNavigateAction,
	"wait":          browserWaitAction,
	"dismiss":       browserDismissAction,
	"click":         browserClickAction,
	"type":          browserTypeAction,
	"hover":         browserHoverAction,
	"press":         browserPressAction,
	"scroll":        browserScrollAction,
	"select":        browserSelectAction,
	"check":         browserSetCheckedAction("check", true),
	"uncheck":       browserSetCheckedAction("uncheck", false),
	"upload":        browserUploadAction,
	"download":      browserDownloadAction,
	"back":          browserBackAction,
	"forward":       browserForwardAction,
	"reload":        browserReloadAction,
	"tab_new":       browserTabNewAction,
	"tab_list":      browserTabListAction,
	"tab_switch":    browserTabSwitchAction,
	"tab_close":     browserTabCloseAction,
	"storage_save":  browserStorageSaveAction,
	"storage_load":  browserStorageLoadAction,
	"set_headers":   browserSetHeadersAction,
	"set_viewport":  browserSetViewportAction,
	"extract":       browserExtractAction,
	"extract_links": browserExtractLinksAction,
	"screenshot":    browserScreenshotAction,
	"pdf":           browserPDFAction,
	"close":         browserCloseAction,
}

func (browserMockOp) Exec(ctx context.Context, req types.HostOpRequest, x *HostOpExecutor) types.HostOpResponse {
	if x.Browser == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "browser not configured"}
	}
	if req.Input == nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "browser.input is required"}
	}

	var params browserMockParams
	if err := json.Unmarshal(req.Input, &params); err != nil {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
	}
	action := strings.ToLower(strings.TrimSpace(params.Action))
	if action == "" {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "action is required"}
	}

	handler, ok := browserMockActions[action]
	if !ok {
		return types.HostOpResponse{Op: req.Op, Ok: false, Error: "unknown browser action: " + action}
	}
	return handler(ctx, params, x)
}

func browserStartAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	headless := true
	if params.Headless != nil {
		headless = *params.Headless
	}
	viewportW, viewportH := 0, 0
	if params.ViewportW != nil {
		viewportW = *params.ViewportW
	}
	if params.ViewportH != nil {
		viewportH = *params.ViewportH
	}
	var extraHeaders map[string]string
	if strings.TrimSpace(params.HeadersJSON) != "" {
		if err := json.Unmarshal([]byte(params.HeadersJSON), &extraHeaders); err != nil {
			return types.HostOpResponse{Op: "browser.start", Ok: false, Error: "headersJson must be a JSON object of string values: " + err.Error()}
		}
	}
	sessionID, err := x.Browser.Start(ctx, headless, strings.TrimSpace(params.UserAgent), viewportW, viewportH, extraHeaders)
	if err != nil {
		return types.HostOpResponse{Op: "browser.start", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"sessionId": sessionID})
	return types.HostOpResponse{Op: "browser.start", Ok: true, Text: string(b)}
}

func browserNavigateAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	title, finalURL, err := x.Browser.Navigate(ctx, params.SessionID, params.URL, params.WaitFor, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.navigate", Ok: false, Error: err.Error()}
	}
	autoDismiss := true
	if params.AutoDismiss != nil {
		autoDismiss = *params.AutoDismiss
	}
	if autoDismiss {
		_, _ = x.Browser.Dismiss(ctx, params.SessionID, "cookies", "accept", 3)
	}
	b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
	return types.HostOpResponse{Op: "browser.navigate", Ok: true, Text: string(b)}
}

func browserWaitAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	sleepMs := positiveInt(params.SleepMs)
	if err := x.Browser.Wait(ctx, params.SessionID, strings.TrimSpace(params.WaitType), params.URL, params.Selector, strings.TrimSpace(params.State), timeoutMs, sleepMs); err != nil {
		return types.HostOpResponse{Op: "browser.wait", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.wait", Ok: true}
}

func browserDismissAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	maxClicks := 3
	if params.MaxClicks != nil && *params.MaxClicks > 0 {
		maxClicks = *params.MaxClicks
	}
	kind := strings.TrimSpace(params.Kind)
	if kind == "" {
		kind = "cookies"
	}
	mode := strings.TrimSpace(params.Mode)
	if mode == "" {
		if kind == "popups" {
			mode = "close"
		} else {
			mode = "accept"
		}
	}
	out, err := x.Browser.Dismiss(ctx, params.SessionID, kind, mode, maxClicks)
	if err != nil {
		return types.HostOpResponse{Op: "browser.dismiss", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.dismiss", Ok: true, Text: strings.TrimSpace(string(out))}
}

func browserClickAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	expectPopup := false
	if params.ExpectPopup != nil {
		expectPopup = *params.ExpectPopup
	}
	pageID, title, finalURL, err := x.Browser.Click(ctx, params.SessionID, params.Selector, params.WaitFor, expectPopup, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.click", Ok: false, Error: err.Error()}
	}
	if strings.TrimSpace(pageID) != "" {
		b, _ := json.Marshal(map[string]string{"pageId": strings.TrimSpace(pageID), "title": strings.TrimSpace(title), "url": strings.TrimSpace(finalURL)})
		return types.HostOpResponse{Op: "browser.click", Ok: true, Text: string(b)}
	}
	return types.HostOpResponse{Op: "browser.click", Ok: true}
}

func browserTypeAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	if err := x.Browser.Fill(ctx, params.SessionID, params.Selector, params.Text, params.WaitFor, timeoutMs); err != nil {
		return types.HostOpResponse{Op: "browser.type", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.type", Ok: true}
}

func browserHoverAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	if err := x.Browser.Hover(ctx, params.SessionID, params.Selector, timeoutMs); err != nil {
		return types.HostOpResponse{Op: "browser.hover", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.hover", Ok: true}
}

func browserPressAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	if err := x.Browser.Press(ctx, params.SessionID, params.Selector, params.Key, timeoutMs); err != nil {
		return types.HostOpResponse{Op: "browser.press", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.press", Ok: true}
}

func browserScrollAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	dx := 0
	if params.DX != nil {
		dx = *params.DX
	}
	dy := 500
	if params.DY != nil {
		dy = *params.DY
	}
	if err := x.Browser.Scroll(ctx, params.SessionID, dx, dy); err != nil {
		return types.HostOpResponse{Op: "browser.scroll", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.scroll", Ok: true}
}

func browserSelectAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	vals := append([]string(nil), params.Values...)
	if strings.TrimSpace(params.Value) != "" {
		vals = append([]string{strings.TrimSpace(params.Value)}, vals...)
	}
	out, err := x.Browser.Select(ctx, params.SessionID, params.Selector, vals, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.select", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.select", Ok: true, Text: strings.TrimSpace(string(out))}
}

func browserSetCheckedAction(action string, checked bool) browserMockAction {
	return func(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
		timeoutMs := positiveInt(params.TimeoutMs)
		if err := x.Browser.SetChecked(ctx, params.SessionID, params.Selector, checked, timeoutMs); err != nil {
			return types.HostOpResponse{Op: "browser." + action, Ok: false, Error: err.Error()}
		}
		return types.HostOpResponse{Op: "browser." + action, Ok: true}
	}
}

func browserUploadAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	abs, err := x.resolveBrowserFilePath(params.FilePath)
	if err != nil {
		return types.HostOpResponse{Op: "browser.upload", Ok: false, Error: err.Error()}
	}
	if err := x.Browser.Upload(ctx, params.SessionID, params.Selector, abs, timeoutMs); err != nil {
		return types.HostOpResponse{Op: "browser.upload", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.upload", Ok: true}
}

func browserDownloadAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if strings.TrimSpace(x.WorkspaceDir) == "" {
		return types.HostOpResponse{Op: "browser.download", Ok: false, Error: "workspace dir not configured"}
	}
	timeoutMs := positiveInt(params.TimeoutMs)
	filename := strings.TrimSpace(params.Filename)
	if filename == "" {
		filename = fmt.Sprintf("download-%s", uuid.NewString()[:8])
	}
	rel, err := safeRelPath(filename)
	if err != nil {
		return types.HostOpResponse{Op: "browser.download", Ok: false, Error: err.Error()}
	}
	absPath := filepath.Join(x.WorkspaceDir, rel)
	out, err := x.Browser.Download(ctx, params.SessionID, params.Selector, absPath, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.download", Ok: false, Error: err.Error()}
	}
	resp := map[string]any{"path": filepath.ToSlash(filepath.Join("/workspace", rel))}
	if len(out) != 0 {
		var extra map[string]any
		if jerr := json.Unmarshal(out, &extra); jerr == nil {
			for k, v := range extra {
				if k == "path" {
					continue
				}
				resp[k] = v
			}
		}
	}
	b, _ := json.Marshal(resp)
	return types.HostOpResponse{Op: "browser.download", Ok: true, Text: string(b)}
}

func browserBackAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	title, finalURL, err := x.Browser.GoBack(ctx, params.SessionID, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.back", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
	return types.HostOpResponse{Op: "browser.back", Ok: true, Text: string(b)}
}

func browserForwardAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	title, finalURL, err := x.Browser.GoForward(ctx, params.SessionID, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.forward", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
	return types.HostOpResponse{Op: "browser.forward", Ok: true, Text: string(b)}
}

func browserReloadAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	title, finalURL, err := x.Browser.Reload(ctx, params.SessionID, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.reload", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"title": title, "url": finalURL})
	return types.HostOpResponse{Op: "browser.reload", Ok: true, Text: string(b)}
}

func browserTabNewAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	timeoutMs := positiveInt(params.TimeoutMs)
	setActive := true
	if params.SetActive != nil {
		setActive = *params.SetActive
	}
	pageID, title, finalURL, err := x.Browser.TabNew(ctx, params.SessionID, params.URL, setActive, timeoutMs)
	if err != nil {
		return types.HostOpResponse{Op: "browser.tab_new", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"pageId": pageID, "title": title, "url": finalURL})
	return types.HostOpResponse{Op: "browser.tab_new", Ok: true, Text: string(b)}
}

func browserTabListAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	out, err := x.Browser.TabList(ctx, params.SessionID)
	if err != nil {
		return types.HostOpResponse{Op: "browser.tab_list", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.tab_list", Ok: true, Text: strings.TrimSpace(string(out))}
}

func browserTabSwitchAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if err := x.Browser.TabSwitch(ctx, params.SessionID, params.PageID); err != nil {
		return types.HostOpResponse{Op: "browser.tab_switch", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.tab_switch", Ok: true}
}

func browserTabCloseAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if err := x.Browser.TabClose(ctx, params.SessionID, params.PageID); err != nil {
		return types.HostOpResponse{Op: "browser.tab_close", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.tab_close", Ok: true}
}

func browserStorageSaveAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if strings.TrimSpace(x.WorkspaceDir) == "" {
		return types.HostOpResponse{Op: "browser.storage_save", Ok: false, Error: "workspace dir not configured"}
	}
	filename := strings.TrimSpace(params.Filename)
	if filename == "" {
		filename = fmt.Sprintf("storage-%s.json", uuid.NewString()[:8])
	}
	rel, err := safeRelPath(filename)
	if err != nil {
		return types.HostOpResponse{Op: "browser.storage_save", Ok: false, Error: err.Error()}
	}
	absPath := filepath.Join(x.WorkspaceDir, rel)
	if err := x.Browser.StorageSave(ctx, params.SessionID, absPath); err != nil {
		return types.HostOpResponse{Op: "browser.storage_save", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", rel))})
	return types.HostOpResponse{Op: "browser.storage_save", Ok: true, Text: string(b)}
}

func browserStorageLoadAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if strings.TrimSpace(x.WorkspaceDir) == "" {
		return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: "workspace dir not configured"}
	}
	filename := strings.TrimSpace(params.Filename)
	if filename == "" {
		return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: "filename is required"}
	}
	rel, err := safeRelPath(filename)
	if err != nil {
		return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: err.Error()}
	}
	absPath := filepath.Join(x.WorkspaceDir, rel)
	if err := x.Browser.StorageLoad(ctx, params.SessionID, absPath); err != nil {
		return types.HostOpResponse{Op: "browser.storage_load", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", rel))})
	return types.HostOpResponse{Op: "browser.storage_load", Ok: true, Text: string(b)}
}

func browserSetHeadersAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	var hdr map[string]string
	if strings.TrimSpace(params.HeadersJSON) != "" {
		if err := json.Unmarshal([]byte(params.HeadersJSON), &hdr); err != nil {
			return types.HostOpResponse{Op: "browser.set_headers", Ok: false, Error: "headersJson must be a JSON object of string values: " + err.Error()}
		}
	}
	if err := x.Browser.SetExtraHeaders(ctx, params.SessionID, hdr); err != nil {
		return types.HostOpResponse{Op: "browser.set_headers", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.set_headers", Ok: true}
}

func browserSetViewportAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	w, h := 0, 0
	if params.ViewportW != nil {
		w = *params.ViewportW
	}
	if params.ViewportH != nil {
		h = *params.ViewportH
	}
	if w <= 0 || h <= 0 {
		return types.HostOpResponse{Op: "browser.set_viewport", Ok: false, Error: "viewportWidth and viewportHeight are required"}
	}
	if err := x.Browser.SetViewport(ctx, params.SessionID, w, h); err != nil {
		return types.HostOpResponse{Op: "browser.set_viewport", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.set_viewport", Ok: true}
}

func browserExtractAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	data, err := x.Browser.Extract(ctx, params.SessionID, params.Selector, params.Attribute)
	if err != nil {
		return types.HostOpResponse{Op: "browser.extract", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.extract", Ok: true, Text: strings.TrimSpace(string(data))}
}

func browserExtractLinksAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	selector := strings.TrimSpace(params.Selector)
	if selector == "" {
		selector = "a"
	}
	data, err := x.Browser.ExtractLinks(ctx, params.SessionID, selector)
	if err != nil {
		return types.HostOpResponse{Op: "browser.extract_links", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.extract_links", Ok: true, Text: strings.TrimSpace(string(data))}
}

func browserScreenshotAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if strings.TrimSpace(x.WorkspaceDir) == "" {
		return types.HostOpResponse{Op: "browser.screenshot", Ok: false, Error: "workspace dir not configured"}
	}
	fullPage := true
	if params.FullPage != nil {
		fullPage = *params.FullPage
	}
	filename := fmt.Sprintf("screenshot-%s.png", uuid.NewString()[:8])
	absPath := filepath.Join(x.WorkspaceDir, filename)
	if err := x.Browser.Screenshot(ctx, params.SessionID, absPath, fullPage); err != nil {
		return types.HostOpResponse{Op: "browser.screenshot", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", filename))})
	return types.HostOpResponse{Op: "browser.screenshot", Ok: true, Text: string(b)}
}

func browserPDFAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if strings.TrimSpace(x.WorkspaceDir) == "" {
		return types.HostOpResponse{Op: "browser.pdf", Ok: false, Error: "workspace dir not configured"}
	}
	filename := fmt.Sprintf("document-%s.pdf", uuid.NewString()[:8])
	absPath := filepath.Join(x.WorkspaceDir, filename)
	if err := x.Browser.PDF(ctx, params.SessionID, absPath); err != nil {
		return types.HostOpResponse{Op: "browser.pdf", Ok: false, Error: err.Error()}
	}
	b, _ := json.Marshal(map[string]string{"path": filepath.ToSlash(filepath.Join("/workspace", filename))})
	return types.HostOpResponse{Op: "browser.pdf", Ok: true, Text: string(b)}
}

func browserCloseAction(ctx context.Context, params browserMockParams, x *HostOpExecutor) types.HostOpResponse {
	if err := x.Browser.Close(ctx, params.SessionID); err != nil {
		return types.HostOpResponse{Op: "browser.close", Ok: false, Error: err.Error()}
	}
	return types.HostOpResponse{Op: "browser.close", Ok: true}
}

func positiveInt(v *int) int {
	if v != nil && *v > 0 {
		return *v
	}
	return 0
}

// PrettyJSON is a small helper for demos/logging.
func PrettyJSON(v any) string {
	b, err := types.MarshalPretty(v)
	if err != nil {
		return "<json marshal error: " + err.Error() + ">"
	}
	return string(b)
}

func safeRelPath(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.Contains(spec, "\\") {
		spec = strings.ReplaceAll(spec, "\\", "/")
	}
	if strings.HasPrefix(spec, "/") {
		return "", fmt.Errorf("filename must be a relative path")
	}
	clean := filepath.Clean(spec)
	if clean == "." || clean == "" {
		return "", fmt.Errorf("filename is required")
	}
	if strings.HasPrefix(clean, ".."+string(filepath.Separator)) || clean == ".." {
		return "", fmt.Errorf("filename must not contain '..'")
	}
	return clean, nil
}

func (x *HostOpExecutor) resolveBrowserFilePath(vpath string) (string, error) {
	vpath = strings.TrimSpace(vpath)
	if vpath == "" {
		return "", fmt.Errorf("filePath is required")
	}
	if strings.HasPrefix(vpath, "/workspace/") || vpath == "/workspace" {
		if strings.TrimSpace(x.WorkspaceDir) == "" {
			return "", fmt.Errorf("workspace dir not configured")
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(vpath, "/workspace"), "/")
		if rel == "" {
			return "", fmt.Errorf("filePath must point to a file under /workspace (not the directory)")
		}
		r, err := safeRelPath(rel)
		if err != nil {
			return "", err
		}
		return filepath.Join(x.WorkspaceDir, r), nil
	}
	if strings.HasPrefix(vpath, "/project/") || vpath == "/project" {
		if strings.TrimSpace(x.ProjectDir) == "" {
			return "", fmt.Errorf("project dir not configured")
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(vpath, "/project"), "/")
		if rel == "" {
			return "", fmt.Errorf("filePath must point to a file under /project (not the directory)")
		}
		r, err := safeRelPath(rel)
		if err != nil {
			return "", err
		}
		return filepath.Join(x.ProjectDir, r), nil
	}
	return "", fmt.Errorf("filePath must be a VFS path under /project or /workspace")
}

func writeChecksumHex(algo string, b []byte) (string, error) { return checksumutil.ComputeHex(algo, b) }

func normalizeWriteModeInput(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "", "w", "overwrite":
		return "w"
	case "a", "append":
		return "a"
	default:
		return "w"
	}
}

func readChecksumsForResponse(single string, many []string, b []byte) map[string]string {
	algos := make([]string, 0, len(many)+1)
	if s := checksumutil.NormalizeAlgorithm(single); s != "" {
		algos = append(algos, s)
	}
	for _, raw := range many {
		if s := checksumutil.NormalizeAlgorithm(raw); s != "" {
			algos = append(algos, s)
		}
	}
	if len(algos) == 0 {
		return nil
	}
	out := make(map[string]string)
	for _, algo := range algos {
		if !checksumutil.IsSupportedAlgorithm(algo) {
			continue
		}
		if _, exists := out[algo]; exists {
			continue
		}
		sum, err := checksumutil.ComputeHex(algo, b)
		if err != nil {
			continue
		}
		out[algo] = sum
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstMismatchOffset(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

func encodeReadPayload(b []byte, maxBytes int) (text string, bytesB64 string, truncated bool) {
	if maxBytes <= 0 {
		maxBytes = len(b)
	}
	n := len(b)
	if n > maxBytes {
		n = maxBytes
		truncated = true
	}
	head := b[:n]

	// Prefer returning text when valid UTF-8.
	// If we truncated, try trimming bytes until the prefix is valid UTF-8.
	for len(head) > 0 && !utf8.Valid(head) {
		head = head[:len(head)-1]
	}
	if len(head) > 0 && utf8.Valid(head) {
		return string(head), "", truncated
	}

	// Binary or non-UTF8: return base64 so the contract is lossless.
	return "", base64.StdEncoding.EncodeToString(b[:n]), truncated
}

func AgentSay(logf func(string, ...any), exec func(types.HostOpRequest) types.HostOpResponse, req types.HostOpRequest) types.HostOpResponse {
	logf("agent -> host:\n%s", PrettyJSON(req))
	resp := exec(req)
	// Avoid dumping huge raw bytes; HostOpResponse may contain truncated text or base64.
	logf("host -> agent:\n%s", strings.TrimSpace(PrettyJSON(resp)))
	return resp
}
