package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/debuglog"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/skills"
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
	// /project depends on a user-provided OS directory, so it is mounted outside the factory.
	fs.Mount(vfs.MountProject, workdirRes)

	skillDir := fsutil.GetSkillsDir(cfg.DataDir)
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare skills dir: %w", err)
	}
	workdirSkillDir := filepath.Join(workdirAbs, "skills")
	if err := os.MkdirAll(workdirSkillDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare workdir skills dir: %w", err)
	}
	skillRoots := []string{
		skillDir,
		workdirSkillDir,
	}
	skillMgr := skills.NewManager(skillRoots)
	skillMgr.WritableRoot = skillDir
	if err := skillMgr.Scan(); err != nil {
		return nil, fmt.Errorf("scan skills: %w", err)
	}
	// Debug: log discovered skills
	if entries := skillMgr.Entries(); len(entries) > 0 {
		debuglog.Log("skills", "H13", "chat_setup.go:setupTUIChatRuntime", "skills_discovered", map[string]any{
			"count": len(entries),
			"names": func() []string {
				names := make([]string, len(entries))
				for i, e := range entries {
					names[i] = e.Dir
				}
				return names
			}(),
		})
	} else {
		debuglog.Log("skills", "H13", "chat_setup.go:setupTUIChatRuntime", "no_skills_discovered", map[string]any{
			"roots": skillRoots,
		})
	}
	fs.Mount(vfs.MountSkills, skills.NewResource(skillMgr))

	// Pull resource handles back out for wiring and debug data.
	_, wsr, _, _ := fs.Resolve("/" + vfs.MountScratch)
	workspace := wsr.(*resources.DirResource)
	_, trr, _, _ := fs.Resolve("/" + vfs.MountLog)
	traceRes := trr.(*resources.TraceResource)
	_, hr, _, _ := fs.Resolve("/" + vfs.MountHistory)
	historyRes = hr.(*resources.HistoryResource)

	planDir := filepath.Join(workspace.BaseDir, "plan")
	if err := os.MkdirAll(planDir, 0755); err != nil {
		return nil, fmt.Errorf("prepare plan dir: %w", err)
	}
	planRes, err := resources.NewDirResource(planDir, "plan")
	if err != nil {
		return nil, fmt.Errorf("create plan resource: %w", err)
	}
	fs.Mount("plan", planRes)

	resultsStore := f.ResultsStore
	memStore := f.MemoryStore
	profileStore := f.ProfileStore

	mustEmit(context.Background(), events.Event{
		Type:    "host.mounted",
		Message: "Mounted VFS resources",
		Data: map[string]string{
			"/scratch":            workspace.BaseDir,
			"/project":            workdirRes.BaseDir,
			"/results":            "(virtual)",
			"/log":                traceRes.BaseDir,
			"/tools":              "(virtual)",
			"/plan":               "(virtual)",
			"/memory":             "(virtual)",
			"/profile":            "(global)",
			"/history":            historyRes.BaseDir,
			"/" + vfs.MountSkills: "(virtual)",
		},
		Console: boolPtr(false),
	})

	absWorkdirRoot, err := filepath.Abs(workdirRes.BaseDir)
	if err != nil {
		return nil, fmt.Errorf("resolve workdir root: %w", err)
	}

	traceStore := f.TraceStore
	builtinCfg := tools.BuiltinConfig{
		ShellRootDir:  absWorkdirRoot,
		ShellVFSMount: vfs.MountProject,
		ShellConfirm:  nil,
		TraceStore:    traceStore,
	}
	shellInvoker := tools.NewBuiltinShellInvoker(absWorkdirRoot, nil, vfs.MountProject)
	httpInvoker := tools.NewBuiltinHTTPInvoker()
	traceInvoker := tools.BuiltinTraceInvoker{Store: traceStore}

	builtinInvokers := tools.BuiltinInvokerRegistry(builtinCfg)
	if builtinInvokers == nil {
		builtinInvokers = make(tools.MapRegistry)
	}
	builtinInvokers[types.ToolID("builtin.shell")] = shellInvoker
	builtinInvokers[types.ToolID("builtin.http")] = httpInvoker
	builtinInvokers[types.ToolID("builtin.trace")] = traceInvoker

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: builtinInvokers,
	}

	builtinManifestProvider, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		return nil, fmt.Errorf("load builtin manifests: %w", err)
	}
	toolManifests := []types.ToolManifest{}
	if ids, err := builtinManifestProvider.ListToolIDs(context.Background()); err != nil {
		return nil, fmt.Errorf("list builtin manifests: %w", err)
	} else {
		for _, id := range ids {
			b, ok, err := builtinManifestProvider.GetManifest(context.Background(), id)
			if err != nil {
				return nil, fmt.Errorf("read builtin manifest %s: %w", id.String(), err)
			}
			if !ok {
				continue
			}
			m, err := types.ParseBuiltinToolManifest(b)
			if err != nil {
				return nil, fmt.Errorf("parse builtin manifest %s: %w", id.String(), err)
			}
			toolManifests = append(toolManifests, m)
		}
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		ShellInvoker:    shellInvoker,
		HTTPInvoker:     httpInvoker,
		TraceInvoker:    traceInvoker,
		DefaultMaxBytes: 4096,
		// Allow tool manifest reads (/tools/<toolId>) to be non-truncated by default.
		// Other fs.read operations remain bounded by DefaultMaxBytes unless the agent requests more.
		MaxReadBytes: 256 * 1024,
	}

	run.Runtime = &types.RunRuntimeConfig{
		DataDir:          cfg.DataDir,
		Model:            model,
		ReasoningEffort:  strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(opts.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(opts.ApprovalsMode),
		PlanMode:         opts.PlanMode,

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
	// Add resilience against transient provider/network failures.
	llmClient := types.LLMClient(llm.NewRetryClient(client, llm.RetryConfig{
		MaxRetries:   3,
		InitialDelay: 250 * time.Millisecond,
		MaxDelay:     4 * time.Second,
		Multiplier:   2.0,
	}))

	// Use the default system prompt embedded in the agent package.
	baseSystemPrompt := ""

	constructor := &agent.ContextConstructor{
		FS:                fs,
		Cfg:               cfg,
		RunID:             run.RunId,
		SessionID:         run.SessionID,
		TraceStore:        traceStore,
		HistoryStore:      historyRes.Store,
		SkillsManager:     skillMgr,
		IncludeHistoryOps: derefBool(opts.IncludeHistoryOps, true),
		MaxProfileBytes:   opts.MaxProfileBytes,
		MaxMemoryBytes:    opts.MaxMemoryBytes,
		MaxTraceBytes:     opts.MaxTraceBytes,
		MaxHistoryBytes:   8 * 1024,
		// Store constructor bookkeeping on disk under the run root (NOT in VFS) so the model can't discover it.
		StatePath:    filepath.Join(fsutil.GetRunDir(cfg.DataDir, run.RunId), "context_constructor_state.json"),
		ManifestPath: filepath.Join(fsutil.GetRunDir(cfg.DataDir, run.RunId), "context_constructor_manifest.json"),
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}
	artifactIndex := newArtifactIndex()

	var updater *agent.ContextUpdater
	var opSeq uint64
	execWithEvents := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		// #region agent log
		// Capture the specific "lost → read context constructor manifest" failure mode.
		if req.Op == types.HostOpFSList && strings.TrimSpace(req.Path) == "/scratch" {
			debuglog.Log("context", "H9", "chat_setup.go:execWithEvents", "fs_list_workspace", map[string]any{
				"model":     strings.TrimSpace(model),
				"runId":     strings.TrimSpace(run.RunId),
				"sessionId": strings.TrimSpace(run.SessionID),
			})
		}
		if req.Op == types.HostOpFSRead && strings.TrimSpace(req.Path) == "/scratch/context_constructor_manifest.json" {
			debuglog.Log("context", "H9", "chat_setup.go:execWithEvents", "fs_read_context_constructor_manifest", map[string]any{
				"model":     strings.TrimSpace(model),
				"runId":     strings.TrimSpace(run.RunId),
				"sessionId": strings.TrimSpace(run.SessionID),
			})
		}
		// Also detect reads if we change the manifest location (keep the old log above intact).
		// #region agent log
		if req.Op == types.HostOpFSRead && strings.TrimSpace(req.Path) == "/results/context_constructor_manifest.json" {
			debuglog.Log("context", "H10", "chat_setup.go:execWithEvents", "fs_read_context_constructor_manifest_results", map[string]any{
				"model":     strings.TrimSpace(model),
				"runId":     strings.TrimSpace(run.RunId),
				"sessionId": strings.TrimSpace(run.SessionID),
			})
		}
		// #endregion
		// #endregion

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
		storeReq := map[string]string{"op": req.Op, "path": req.Path, "toolId": req.ToolID.String(), "actionId": req.ActionID}
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
				reqData["traceInput"] = string(req.Input) // Input is usually small JSON
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
			StoreData: storeReq,
		})

		resp := types.HostOpResponse{}
		if guard := enforcePlanChecklist(executor.FS, req); guard != nil {
			resp = *guard
		} else {
			resp = executor.Exec(ctx, req)
		}
		if resp.Ok && (req.Op == types.HostOpFSWrite || req.Op == types.HostOpFSAppend) {
			artifactIndex.ObserveWrite(req.Path)
		}
		if updater != nil {
			updater.ObserveHostOp(req, resp)
		}
		constructor.ObserveHostOp(req, resp)

		respData := map[string]string{
			"opId": opID,
			"op":   resp.Op,
			"ok":   fmtBool(resp.Ok),
			"err":  resp.Error,
		}
		storeResp := map[string]string{"op": resp.Op, "ok": fmtBool(resp.Ok), "err": resp.Error}

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
			storeResp["callId"] = resp.ToolResponse.CallID
		}
		if resp.Op == types.HostOpToolRun && resp.ToolResponse != nil && len(resp.ToolResponse.Output) != 0 {
			if p := toolRunOutputPreviewForEvent(resp.ToolResponse.ToolID.String(), resp.ToolResponse.ActionID, resp.ToolResponse.Output); strings.TrimSpace(p) != "" {
				respData["outputPreview"] = p
			}
			if fields := shellStoreFieldsFromResponse(resp); len(fields) != 0 {
				for k, v := range fields {
					storeResp[k] = v
				}
			}
		}
		if resp.Op == types.HostOpShellExec {
			respData["exitCode"] = strconv.Itoa(resp.ExitCode)
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
			// Use existing helper to populate store fields (stderr proper path etc)
			if fields := shellStoreFieldsFromResponse(resp); len(fields) != 0 {
				for k, v := range fields {
					storeResp[k] = v
				}
			}
		}
		if resp.Op == types.HostOpTrace {
			if resp.Text != "" {
				// Trace output (from Text field in HostOpResponse)
				s, tr := capBytes(resp.Text, 1000)
				respData["output"] = s
				if tr {
					respData["outputTruncated"] = "true"
				}
			}
		}
		if resp.Op == types.HostOpHTTPFetch {
			respData["status"] = strconv.Itoa(resp.Status)
			if resp.FinalURL != "" {
				respData["finalUrl"] = resp.FinalURL
			}
			if resp.Body != "" {
				s, tr := capBytes(resp.Body, 1000)
				respData["body"] = s
				if tr {
					respData["bodyTruncated"] = "true"
				}
			}
		}
		mustEmit(ctx, events.Event{
			Type:      "agent.op.response",
			Message:   "Host op completed",
			Data:      respData,
			StoreData: storeResp,
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
		ManifestPath:    "/scratch/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}

	a, err := agent.New(agent.Config{
		LLM:              llmClient,
		Exec:             agent.HostExecFunc(execWithEvents),
		Model:            model,
		ReasoningEffort:  strings.TrimSpace(opts.ReasoningEffort),
		ReasoningSummary: strings.TrimSpace(opts.ReasoningSummary),
		ApprovalsMode:    strings.TrimSpace(opts.ApprovalsMode),
		EnableWebSearch:  opts.WebSearchEnabled,
		PlanMode:         opts.PlanMode,
		SystemPrompt:     baseSystemPrompt,
		Context:          constructor,

		ToolManifests: toolManifests,
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
		// Better UX for shell: strip the wrapper "bash -c" if present.
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
