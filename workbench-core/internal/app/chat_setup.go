package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
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

	traceRes, err := resources.NewTraceResource(cfg, run.RunId)
	if err != nil {
		return nil, fmt.Errorf("create trace: %w", err)
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

	workspace, err := resources.NewRunWorkspace(cfg, run.RunId)
	if err != nil {
		return nil, fmt.Errorf("create workspace: %w", err)
	}

	toolsDir := fsutil.GetToolsDir(cfg.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)

	builtinProvider, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		return nil, fmt.Errorf("load builtin tool manifests: %w", err)
	}
	diskProvider := tools.NewDiskManifestProvider(toolsDir)
	toolManifests := tools.NewCompositeToolManifestRegistry(builtinProvider, diskProvider)

	toolsResource, err := resources.NewVirtualToolsResource(toolManifests)
	if err != nil {
		return nil, fmt.Errorf("create tools resource: %w", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		return nil, fmt.Errorf("create results: %w", err)
	}

	memStore, err := store.NewDiskMemoryStore(cfg, run.RunId)
	if err != nil {
		return nil, fmt.Errorf("create memory store: %w", err)
	}
	memoryRes, err := resources.NewVirtualMemoryResource(memStore)
	if err != nil {
		return nil, fmt.Errorf("create memory resource: %w", err)
	}

	profileStore, err := store.NewDiskProfileStore(cfg)
	if err != nil {
		return nil, fmt.Errorf("create profile store: %w", err)
	}
	profileRes, err := resources.NewVirtualProfileResource(profileStore)
	if err != nil {
		return nil, fmt.Errorf("create profile resource: %w", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	fs.Mount(vfs.MountWorkdir, workdirRes)
	fs.Mount(vfs.MountResults, resultsRes)
	fs.Mount(vfs.MountTrace, traceRes)
	fs.Mount(vfs.MountTools, toolsResource)
	fs.Mount(vfs.MountMemory, memoryRes)
	fs.Mount(vfs.MountProfile, profileRes)
	fs.Mount(vfs.MountHistory, historyRes)

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

	traceStore := store.DiskTraceStore{Dir: traceRes.BaseDir}
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
		if strings.TrimSpace(sess.ActiveModel) != model {
			sess.ActiveModel = model
			_ = store.SaveSession(cfg, sess)
		}
	}

	client, err := llm.NewOpenRouterClientFromEnv()
	if err != nil {
		return nil, fmt.Errorf("create OpenRouter client: %w", err)
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
	execWithEvents := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		reqData := map[string]string{
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
			"op":  resp.Op,
			"ok":  fmtBool(resp.Ok),
			"err": resp.Error,
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
		LLM:          client,
		Exec:         agent.HostExecFunc(execWithEvents),
		Model:        model,
		SystemPrompt: baseSystemPrompt,
		Context:      constructor,
		MaxSteps:     opts.MaxSteps,
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
