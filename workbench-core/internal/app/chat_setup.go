package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"

	"github.com/pmezard/go-difflib/difflib"
)

type tuiChatSetup struct {
	FS *vfs.FS

	Agent            *agent.Agent
	BaseSystemPrompt string
	Constructor      *agent.ContextConstructor

	Artifacts *ArtifactIndex

	WorkdirBase string

	MemStore     store.MemoryCommitter
	ProfileStore store.ProfileCommitter

	// BuiltinInvokers is the in-memory registry used by tool.run for builtins.
	// It is a map (reference type), so updating entries updates runner behavior.
	BuiltinInvokers tools.MapRegistry
}

// setupTUIChatRuntime performs the core runtime setup for a TUI-driven run:
// mounting VFS resources, wiring tools/executor, creating context constructor/updater,
// and instantiating the agent.
//
// Event emission and model selection happen at the call site; this function assumes
// model is already resolved/validated and will persist it into run/session state.
func setupTUIChatRuntime(
	cfg config.Config,
	run types.Run,
	opts RunChatOptions,
	model string,
	historyRes *resources.HistoryResource,
	mustEmit func(ctx context.Context, ev events.Event),
) (*tuiChatSetup, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if historyRes == nil {
		return nil, fmt.Errorf("history resource is required")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, fmt.Errorf("model is required")
	}

	fs := vfs.NewFS()

	workdirAbs, err := resolveWorkDir(opts.WorkDir)
	if err != nil {
		return nil, err
	}
	workdirRes, err := resources.NewWorkdirResource(workdirAbs)
	if err != nil {
		return nil, fmt.Errorf("create workdir: %w", err)
	}

	f := &resources.Factory{
		DataDir:   cfg.DataDir,
		SessionID: run.SessionID,
		RunID:     run.RunId,
	}
	// Reuse the existing disk history store instance (used by history sinks) when possible.
	if hs, ok := historyRes.Store.(store.HistoryStore); ok {
		f.HistoryStore = hs
	} else if hs, ok := historyRes.Appender.(store.HistoryStore); ok {
		f.HistoryStore = hs
	}
	if err := f.MountAll(fs); err != nil {
		return nil, err
	}
	// /workdir depends on a user-provided OS directory, so it is mounted outside the factory.
	fs.Mount(vfs.MountWorkdir, workdirRes)

	// Pull resource handles back out for wiring and debug data.
	_, wsr, _, _ := fs.Resolve("/" + vfs.MountWorkspace)
	workspace := wsr.(*resources.DirResource)
	_, trr, _, _ := fs.Resolve("/" + vfs.MountTrace)
	traceRes := trr.(*resources.TraceResource)
	_, mr, _, _ := fs.Resolve("/" + vfs.MountMemory)
	memoryRes := mr.(*resources.MemoryResource)
	_, hr, _, _ := fs.Resolve("/" + vfs.MountHistory)
	historyRes = hr.(*resources.HistoryResource)

	resultsStore := f.ResultsStore
	memStore := f.MemoryStore
	profileStore := f.ProfileStore

	mustEmit(context.Background(), events.Event{
		Type:    "host.mounted",
		Message: "Mounted VFS resources",
		Data: map[string]string{
			"/workspace": workspace.BaseDir,
			"/workdir":   workdirRes.BaseDir,
			"/results":   "(virtual)",
			"/trace":     traceRes.BaseDir,
			"/tools":     "(virtual)",
			"/memory":    memoryRes.BaseDir,
			"/profile":   "(global)",
			"/history":   historyRes.BaseDir,
		},
		Console: boolPtr(false),
	})

	absWorkdirRoot, err := filepath.Abs(workdirRes.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir root: %w", err)
	}

	traceStore := f.TraceStore
	builtinCfg := tools.BuiltinConfig{
		BashRootDir:    absWorkdirRoot,
		RipgrepRootDir: absWorkdirRoot,
		TraceStore:     traceStore,
	}

	builtinInvokers := tools.BuiltinInvokerRegistry(builtinCfg)
	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: builtinInvokers,
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    16 * 1024,
	}

	run.Runtime = &types.RunRuntimeConfig{
		DataDir:               cfg.DataDir,
		Model:                 model,
		ReasoningEffort:       strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary:      strings.TrimSpace(opts.ReasoningSummary),
		MaxSteps:              opts.MaxSteps,
		MaxTraceBytes:         opts.MaxTraceBytes,
		MaxMemoryBytes:        opts.MaxMemoryBytes,
		MaxProfileBytes:       opts.MaxProfileBytes,
		RecentHistoryPairs:    opts.RecentHistoryPairs,
		IncludeHistoryOps:     derefBool(opts.IncludeHistoryOps, true),
		PriceInPerMTokensUSD:  opts.PriceInPerMTokensUSD,
		PriceOutPerMTokensUSD: opts.PriceOutPerMTokensUSD,
	}
	_ = store.SaveRun(cfg, run)

	// Persist the active model at the session level so resume is deterministic.
	if sess, err := store.LoadSession(cfg, run.SessionID); err == nil {
		changed := false
		if strings.TrimSpace(sess.ActiveModel) != model {
			sess.ActiveModel = model
			changed = true
		}
		if strings.TrimSpace(sess.ReasoningEffort) != strings.TrimSpace(opts.ReasoningEffort) {
			sess.ReasoningEffort = strings.TrimSpace(opts.ReasoningEffort)
			changed = true
		}
		if strings.TrimSpace(sess.ReasoningSummary) != strings.TrimSpace(opts.ReasoningSummary) {
			sess.ReasoningSummary = strings.TrimSpace(opts.ReasoningSummary)
			changed = true
		}
		if changed {
			_ = store.SaveSession(cfg, sess)
		}
	}

	client, err := llm.NewClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create LLM client: %w", err)
	}

	systemPromptBytes, err := os.ReadFile("internal/agent/INITIAL_PROMPT.md")
	if err != nil {
		return nil, fmt.Errorf("read internal/agent/INITIAL_PROMPT.md: %w", err)
	}
	baseSystemPrompt := string(systemPromptBytes)

	constructor := &agent.ContextConstructor{
		FS:                fs,
		Cfg:               cfg,
		RunID:             run.RunId,
		SessionID:         run.SessionID,
		TraceStore:        traceStore,
		HistoryStore:      historyRes.Store,
		IncludeHistoryOps: derefBool(opts.IncludeHistoryOps, true),
		MaxProfileBytes:   opts.MaxProfileBytes,
		MaxMemoryBytes:    opts.MaxMemoryBytes,
		MaxTraceBytes:     opts.MaxTraceBytes,
		MaxHistoryBytes:   8 * 1024,
		StatePath:         "/workspace/context_constructor_state.json",
		ManifestPath:      "/workspace/context_constructor_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}
	artifactIndex := newArtifactIndex()

	var updater *agent.ContextUpdater
	var opSeq uint64
	execWithEvents := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		opID := fmt.Sprintf("op-%d", atomic.AddUint64(&opSeq, 1))
		// For file ops, capture "before" deterministically on the host side so the UI
		// can render diffs without racing on client-side reads.
		//
		// NOTE: this reads the whole file; this is acceptable for now because the preview
		// is hard-capped, and workbench's file ops are typically small.
		beforeBytes := []byte(nil)
		hadBefore := false
		if (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend || req.Op == types.HostOpFSEdit || req.Op == types.HostOpFSPatch) && strings.TrimSpace(req.Path) != "" {
			if b, err := executor.FS.Read(req.Path); err == nil {
				beforeBytes = b
				hadBefore = true
			}
		}

		reqData := map[string]string{
			"opId":     opID,
			"op":       req.Op,
			"path":     req.Path,
			"toolId":   req.ToolID.String(),
			"actionId": req.ActionID,
		}
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
			// Patch preview is used by the UI to render a compact diff block in the transcript
			// without needing to re-read large patch payloads.
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

		mustEmit(ctx, events.Event{
			Type:      "agent.op.request",
			Message:   "Agent requested host op",
			Data:      reqData,
			StoreData: map[string]string{"op": req.Op, "path": req.Path, "toolId": req.ToolID.String(), "actionId": req.ActionID},
		})

		resp := executor.Exec(ctx, req)
		if resp.Ok && (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) {
			artifactIndex.ObserveWrite(req.Path)
		}
		if updater != nil {
			updater.ObserveHostOp(req, resp)
		}
		constructor.ObserveHostOp(req, resp)

		respData := map[string]string{
			"opId": opID,
			"op":  resp.Op,
			"ok":  fmtBool(resp.Ok),
			"err": resp.Error,
		}

		// For non-patch file ops, emit a host-generated diff so the UI can display a
		// reliable preview even when client-side "before" reads race with execution.
		if resp.Ok && (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend || req.Op == types.HostOpFSEdit) && strings.TrimSpace(req.Path) != "" {
			before := string(beforeBytes)
			after := ""
			switch req.Op {
			case types.HostOpFSWrite:
				after = req.Text
			case types.HostOpFSAppend:
				after = before + req.Text
			case types.HostOpFSEdit:
				if b, err := executor.FS.Read(req.Path); err == nil {
					after = string(b)
				}
			}
			// Only emit if we have something to diff.
			if after != "" || hadBefore {
				fromFile := "a" + strings.TrimSpace(req.Path)
				toFile := "b" + strings.TrimSpace(req.Path)
				if !hadBefore {
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
				if diffText != "" && !looksSensitiveText(diffText) {
					// Hard cap to keep event stream small; the UI also caps lines.
					diffText, tr := capBytes(diffText, 12_000)
					respData["patchPreview"] = diffText
					if tr {
						respData["patchTruncated"] = "true"
					}
				} else if diffText != "" {
					respData["patchPreview"] = "<omitted>"
					respData["patchRedacted"] = "true"
				}
			}
		}
		// Include request context so the UI can associate responses with requests
		// (and render diffs/patch previews for file ops).
		if strings.TrimSpace(req.Path) != "" {
			respData["path"] = strings.TrimSpace(req.Path)
		}
		if strings.TrimSpace(req.ToolID.String()) != "" {
			respData["toolId"] = strings.TrimSpace(req.ToolID.String())
		}
		if strings.TrimSpace(req.ActionID) != "" {
			respData["actionId"] = strings.TrimSpace(req.ActionID)
		}
		if resp.BytesLen != 0 {
			respData["bytesLen"] = strconv.Itoa(resp.BytesLen)
		}
		if resp.Truncated {
			respData["truncated"] = "true"
		}
		if resp.ToolResponse != nil && resp.ToolResponse.CallID != "" {
			respData["callId"] = resp.ToolResponse.CallID
		}
		if resp.Op == types.HostOpToolRun && resp.ToolResponse != nil && len(resp.ToolResponse.Output) != 0 {
			if p := toolRunOutputPreviewForEvent(resp.ToolResponse.ToolID.String(), resp.ToolResponse.ActionID, resp.ToolResponse.Output); strings.TrimSpace(p) != "" {
				respData["outputPreview"] = p
			}
		}
		mustEmit(ctx, events.Event{
			Type:      "agent.op.response",
			Message:   "Host op completed",
			Data:      respData,
			StoreData: map[string]string{"op": resp.Op, "ok": fmtBool(resp.Ok), "err": resp.Error},
		})
		return resp
	}

	mustEmit(context.Background(), events.Event{
		Type:    "agent.loop.start",
		Message: "Agent loop started",
		Data:    map[string]string{"model": model},
	})

	updater = &agent.ContextUpdater{
		FS:              fs,
		TraceStore:      traceStore,
		MaxProfileBytes: opts.MaxProfileBytes,
		MaxMemoryBytes:  opts.MaxMemoryBytes,
		MaxTraceBytes:   opts.MaxTraceBytes,
		ManifestPath:    "/workspace/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}

	a, err := agent.New(agent.Config{
		LLM:              client,
		Exec:             agent.HostExecFunc(execWithEvents),
		Model:            model,
		ReasoningEffort:  strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(opts.ReasoningSummary),
		SystemPrompt:     baseSystemPrompt,
		Context:          constructor,
		MaxSteps:         opts.MaxSteps,
	})
	if err != nil {
		return nil, err
	}

	return &tuiChatSetup{
		FS:               fs,
		Agent:            a,
		BaseSystemPrompt: baseSystemPrompt,
		Constructor:      constructor,
		Artifacts:        artifactIndex,
		WorkdirBase:      workdirRes.BaseDir,
		MemStore:         memStore,
		ProfileStore:     profileStore,
		BuiltinInvokers:  builtinInvokers,
	}, nil
}
