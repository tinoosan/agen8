package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/chzyer/readline"
	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/config"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/fsutil"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/repl"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

// RunChat starts the interactive REPL-driven agent loop for a run.
//
// This is the main "Workbench" experience:
//   - mounts run-scoped resources (/workspace, /trace, /memory)
//   - mounts virtual resources (/results, /tools)
//   - mounts session-scoped history (/history)
//   - starts a readline-based chat session
//
// The CLI (cmd/workbench) decides how runs/sessions are created or resumed.
func RunChat(ctx context.Context, run types.Run) (retErr error) {
	runCtx, stopSignals := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	// Ensure the run is transitioned to a terminal state and persisted to run.json.
	//
	// This fixes the current WIP behavior where a run can remain "running" forever if the
	// process exits without calling store.StopRun (e.g. Ctrl-C).
	defer func() {
		status := types.StatusDone
		errMsg := ""
		if runCtx.Err() != nil {
			status = types.StatusCanceled
			errMsg = "interrupted"
		}
		if retErr != nil {
			status = types.StatusFailed
			errMsg = retErr.Error()
		}
		_, _ = store.StopRun(run.RunId, status, errMsg)
	}()

	log.Printf("== Workbench chat ==")
	log.Printf("sessionId=%s", run.SessionID)
	log.Printf("runId=%s", run.RunId)

	historyRes, err := resources.NewSessionHistoryResource(run.SessionID)
	if err != nil {
		return fmt.Errorf("create history: %w", err)
	}
	historySink := &events.HistorySink{Store: historyRes.Store}

	emitter := &events.Emitter{
		RunID: run.RunId,
		Sink: events.MultiSink{
			events.ConsoleSink{},
			events.StoreSink{},
			historySink,
		},
	}
	mustEmit := func(ctx context.Context, ev events.Event) {
		if err := emitter.Emit(ctx, ev); err != nil {
			log.Fatalf("error emitting event: %v", err)
		}
	}
	boolp := func(b bool) *bool { return &b }

	data := map[string]string{
		"name":  "Alice",
		"email": "alice@example.com",
	}

	mustEmit(context.Background(), events.Event{
		Type:    "run.started",
		Message: "Run started",
		Data:    data,
		Console: boolp(false),
	})

	traceRes, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		return fmt.Errorf("create trace: %w", err)
	}

	fs := vfs.NewFS()

	workspace, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}

	// /tools is virtual and does not require a disk directory.
	// If data/tools exists, it is used as an optional provider.
	toolsDir := fsutil.GetToolsDir(config.DataDir)
	_ = os.MkdirAll(toolsDir, 0755)

	builtinProvider, err := tools.NewBuiltinManifestProvider()
	if err != nil {
		return fmt.Errorf("load builtin tool manifests: %w", err)
	}
	diskProvider := tools.NewDiskManifestProvider(toolsDir)
	diskProvider.Logf = log.Printf

	toolManifests := tools.NewCompositeToolManifestRegistry(builtinProvider, diskProvider)
	toolManifests.Logf = log.Printf

	toolsResource, err := resources.NewVirtualToolsResource(toolManifests)
	if err != nil {
		return fmt.Errorf("create tools resource: %w", err)
	}

	resultsStore := store.NewInMemoryResultsStore()
	resultsRes, err := resources.NewVirtualResultsResource(resultsStore)
	if err != nil {
		return fmt.Errorf("create results: %w", err)
	}

	memStore, err := store.NewDiskMemoryStore(run.RunId)
	if err != nil {
		return fmt.Errorf("create memory store: %w", err)
	}
	memoryRes, err := resources.NewVirtualMemoryResource(memStore)
	if err != nil {
		return fmt.Errorf("create memory resource: %w", err)
	}

	profileStore, err := store.NewDiskProfileStore()
	if err != nil {
		return fmt.Errorf("create profile store: %w", err)
	}
	profileRes, err := resources.NewVirtualProfileResource(profileStore)
	if err != nil {
		return fmt.Errorf("create profile resource: %w", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted /workspace => %s", workspace.BaseDir)
	fs.Mount(vfs.MountResults, resultsRes)
	log.Printf("mounted /results => (virtual)")
	fs.Mount(vfs.MountTrace, traceRes)
	log.Printf("mounted /trace => %s", traceRes.BaseDir)
	fs.Mount(vfs.MountTools, toolsResource)
	log.Printf("mounted /tools => (virtual; disk provider: %s)", toolsDir)
	fs.Mount(vfs.MountMemory, memoryRes)
	log.Printf("mounted /memory => %s", memoryRes.BaseDir)
	fs.Mount(vfs.MountProfile, profileRes)
	log.Printf("mounted /profile => (global; disk store)")
	fs.Mount(vfs.MountHistory, historyRes)
	log.Printf("mounted /history => %s", historyRes.BaseDir)

	absWorkspaceRoot, err := filepath.Abs(workspace.BaseDir)
	if err != nil {
		return fmt.Errorf("resolve workspace root: %w", err)
	}

	traceStore := store.DiskTraceStore{Dir: traceRes.BaseDir}
	builtinCfg := tools.BuiltinConfig{
		BashRootDir: absWorkspaceRoot,
		TraceStore:  traceStore,
	}

	runner := tools.Runner{
		Results:      resultsStore,
		ToolRegistry: tools.BuiltinInvokerRegistry(builtinCfg),
	}

	executor := &agent.HostOpExecutor{
		FS:              fs,
		Runner:          &runner,
		DefaultMaxBytes: 4096,
		MaxReadBytes:    16 * 1024,
	}

	model := strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" || model == "" {
		return fmt.Errorf("OPENROUTER_API_KEY and OPENROUTER_MODEL are required to run the non-scripted agent loop")
	}
	historySink.Model = model

	client, err := llm.NewOpenRouterClientFromEnv()
	if err != nil {
		return fmt.Errorf("create OpenRouter client: %w", err)
	}

	systemPromptBytes, err := os.ReadFile("internal/agent/INITIAL_PROMPT.md")
	if err != nil {
		return fmt.Errorf("read internal/agent/INITIAL_PROMPT.md: %w", err)
	}
	baseSystemPrompt := string(systemPromptBytes)

	var updater *agent.ContextUpdater
	execWithEvents := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		reqData := map[string]string{
			"op":       req.Op,
			"path":     req.Path,
			"toolId":   req.ToolID.String(),
			"actionId": req.ActionID,
		}
		if req.Op == "fs.read" && req.MaxBytes != 0 {
			reqData["maxBytes"] = strconv.Itoa(req.MaxBytes)
		}
		if req.Op == "tool.run" && req.TimeoutMs != 0 {
			reqData["timeoutMs"] = strconv.Itoa(req.TimeoutMs)
		}

		mustEmit(ctx, events.Event{
			Type:      "agent.op.request",
			Message:   "Agent requested host op",
			Data:      reqData,
			StoreData: map[string]string{"op": req.Op, "path": req.Path, "toolId": req.ToolID.String(), "actionId": req.ActionID},
		})
		resp := executor.Exec(ctx, req)
		if updater != nil {
			updater.ObserveHostOp(req, resp)
		}

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
		FS:             fs,
		TraceStore:     traceStore,
		MaxMemoryBytes: 8 * 1024,
		MaxTraceBytes:  8 * 1024,
		ManifestPath:   "/workspace/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			mustEmit(context.Background(), events.Event{Type: eventType, Message: message, Data: data})
		},
	}

	a := &agent.Agent{
		LLM:            client,
		Exec:           execWithEvents,
		Model:          model,
		SystemPrompt:   baseSystemPrompt,
		ContextUpdater: updater,
		MaxSteps:       200,
		Logf:           nil,
		OnLLMUsage:     nil,
	}

	log.Printf("== Chat session started (type 'exit' to quit) ==")
	historyPath := filepath.Join(config.DataDir, "runs", run.RunId, "repl_history.txt")
	rr, err := repl.NewReader(historyPath)
	if err != nil {
		return fmt.Errorf("start readline: %w", err)
	}
	oldLogWriter := log.Writer()
	log.SetOutput(rr)
	defer rr.Close()
	defer log.SetOutput(oldLogWriter)

	// Print REPL help once at session start (not on every user turn).
	_, _ = io.WriteString(rr, userInputHelp())

	memEval := agent.DefaultMemoryEvaluator()
	turn := 0
	var conversation []types.LLMMessage
	for {
		userMsg, exit, err := readUserMessage(rr, rr)
		if err != nil {
			log.Printf("read stdin: %v", err)
			break
		}
		if exit {
			break
		}
		if strings.TrimSpace(userMsg) == "" {
			continue
		}
		if strings.TrimSpace(userMsg) == ":reset" {
			conversation = nil
			mustEmit(context.Background(), events.Event{
				Type:    "chat.reset",
				Message: "Cleared conversation history",
				Store:   boolp(false),
			})
			continue
		}

		// Refresh session state and inject it so the agent stays coherent across runs.
		if sess, err := store.LoadSession(run.SessionID); err == nil {
			if blk := agent.SessionContextBlock(sess); strings.TrimSpace(blk) != "" {
				a.SystemPrompt = strings.TrimSpace(baseSystemPrompt) + "\n\n" + blk + "\n"
			} else {
				a.SystemPrompt = baseSystemPrompt
			}
		} else {
			a.SystemPrompt = baseSystemPrompt
		}

		turn++
		mustEmit(context.Background(), events.Event{
			Type:    "user.message",
			Message: "User message received",
			Data:    map[string]string{"text": userMsg},
			Console: boolp(false),
		})

		var turnUsage types.LLMUsage
		a.OnLLMUsage = func(step int, usage types.LLMUsage) {
			turnUsage.InputTokens += usage.InputTokens
			turnUsage.OutputTokens += usage.OutputTokens
			turnUsage.TotalTokens += usage.TotalTokens
			mustEmit(context.Background(), events.Event{
				Type:    "llm.usage",
				Message: "Model usage",
				Data: map[string]string{
					"step":      strconv.Itoa(step),
					"input":     strconv.Itoa(usage.InputTokens),
					"output":    strconv.Itoa(usage.OutputTokens),
					"total":     strconv.Itoa(usage.TotalTokens),
					"turnTotal": strconv.Itoa(turnUsage.TotalTokens),
				},
				Store: boolp(false),
			})
		}

		conversation = append(conversation, types.LLMMessage{Role: "user", Content: userMsg})
		start := time.Now()
		final, updated, steps, err := a.RunConversation(runCtx, conversation)
		dur := time.Since(start)
		conversation = updated
		if err != nil {
			mustEmit(context.Background(), events.Event{
				Type:    "agent.error",
				Message: "Agent loop error",
				Data:    map[string]string{"err": err.Error()},
				Store:   boolp(false),
			})
			log.Printf("agent error: %v", err)
			continue
		}
		mustEmit(context.Background(), events.Event{
			Type:    "agent.turn.complete",
			Message: "Agent completed user request",
			Data: map[string]string{
				"turn":       strconv.Itoa(turn),
				"steps":      strconv.Itoa(steps),
				"durationMs": strconv.FormatInt(dur.Milliseconds(), 10),
				"duration":   dur.Truncate(time.Millisecond).String(),
			},
			Store: boolp(false),
		})
		if turnUsage.TotalTokens != 0 {
			mustEmit(context.Background(), events.Event{
				Type:    "llm.usage.total",
				Message: "Turn usage total",
				Data: map[string]string{
					"input":  strconv.Itoa(turnUsage.InputTokens),
					"output": strconv.Itoa(turnUsage.OutputTokens),
					"total":  strconv.Itoa(turnUsage.TotalTokens),
				},
				Store: boolp(false),
			})
		}

		// Ingest memory update if the agent wrote one.
		if b, err := fs.Read("/memory/update.md"); err == nil {
			updateRaw := string(b)
			if strings.TrimSpace(updateRaw) == "" {
				mustEmit(context.Background(), events.Event{
					Type:    "memory.evaluate",
					Message: "No memory update written",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": "false",
						"reason":   "no_update",
						"bytes":    "0",
					},
				})
			} else {
				trimmed := strings.TrimSpace(updateRaw)
				hash := agent.SHA256Hex(trimmed)

				accepted, reason, cleaned := memEval.Evaluate(updateRaw)

				mustEmit(context.Background(), events.Event{
					Type:    "memory.evaluate",
					Message: "Evaluated memory update",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": fmtBool(accepted),
						"reason":   reason,
						"bytes":    strconv.Itoa(len(trimmed)),
						"sha256":   hash[:12],
					},
				})

				if accepted {
					if err := memStore.AppendMemory(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
						mustEmit(context.Background(), events.Event{
							Type:    "memory.commit.error",
							Message: "Failed to commit memory update",
							Data:    map[string]string{"err": err.Error()},
							Store:   boolp(false),
						})
					} else {
						mustEmit(context.Background(), events.Event{
							Type:    "memory.commit",
							Message: "Committed memory update",
							Data: map[string]string{
								"turn":   strconv.Itoa(turn),
								"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
								"sha256": hash[:12],
							},
						})
					}
				}

				if err := memStore.AppendCommitLog(context.Background(), types.MemoryCommitLine{
					Scope:     "memory",
					SessionID: run.SessionID,
					RunID:     run.RunId,
					Model:     model,
					Turn:      turn,
					Accepted:  accepted,
					Reason:    reason,
					Bytes:     len(trimmed),
					SHA256:    hash,
				}); err != nil {
					mustEmit(context.Background(), events.Event{
						Type:    "memory.audit.error",
						Message: "Failed to append memory audit log",
						Data: map[string]string{
							"turn": strconv.Itoa(turn),
							"err":  err.Error(),
						},
						Store: boolp(false),
					})
				} else {
					mustEmit(context.Background(), events.Event{
						Type:    "memory.audit.append",
						Message: "Appended memory audit log",
						Data: map[string]string{
							"turn":     strconv.Itoa(turn),
							"accepted": fmtBool(accepted),
							"reason":   reason,
							"sha256":   hash[:12],
						},
						Store: boolp(false),
					})
				}
			}

			_ = fs.Write("/memory/update.md", []byte{})
		}

		// Ingest profile update if the agent wrote one.
		if b, err := fs.Read("/profile/update.md"); err == nil {
			updateRaw := string(b)
			if strings.TrimSpace(updateRaw) == "" {
				mustEmit(context.Background(), events.Event{
					Type:    "profile.evaluate",
					Message: "No profile update written",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": "false",
						"reason":   "no_update",
						"bytes":    "0",
					},
				})
			} else {
				trimmed := strings.TrimSpace(updateRaw)
				hash := agent.SHA256Hex(trimmed)

				accepted, reason, cleaned := memEval.Evaluate(updateRaw)

				mustEmit(context.Background(), events.Event{
					Type:    "profile.evaluate",
					Message: "Evaluated profile update",
					Data: map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": fmtBool(accepted),
						"reason":   reason,
						"bytes":    strconv.Itoa(len(trimmed)),
						"sha256":   hash[:12],
					},
				})

				if accepted {
					if err := profileStore.AppendProfile(context.Background(), formatRunMemoryAppend(strings.TrimSpace(cleaned))); err != nil {
						mustEmit(context.Background(), events.Event{
							Type:    "profile.commit.error",
							Message: "Failed to commit profile update",
							Data:    map[string]string{"err": err.Error()},
							Store:   boolp(false),
						})
					} else {
						mustEmit(context.Background(), events.Event{
							Type:    "profile.commit",
							Message: "Committed profile update",
							Data: map[string]string{
								"turn":   strconv.Itoa(turn),
								"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
								"sha256": hash[:12],
							},
						})
					}
				}

				if err := profileStore.AppendCommitLog(context.Background(), types.MemoryCommitLine{
					Scope:     "profile",
					SessionID: run.SessionID,
					RunID:     run.RunId,
					Model:     model,
					Turn:      turn,
					Accepted:  accepted,
					Reason:    reason,
					Bytes:     len(trimmed),
					SHA256:    hash,
				}); err != nil {
					mustEmit(context.Background(), events.Event{
						Type:    "profile.audit.error",
						Message: "Failed to append profile audit log",
						Data: map[string]string{
							"turn": strconv.Itoa(turn),
							"err":  err.Error(),
						},
						Store: boolp(false),
					})
				} else {
					mustEmit(context.Background(), events.Event{
						Type:    "profile.audit.append",
						Message: "Appended profile audit log",
						Data: map[string]string{
							"turn":     strconv.Itoa(turn),
							"accepted": fmtBool(accepted),
							"reason":   reason,
							"sha256":   hash[:12],
						},
						Store: boolp(false),
					})
				}
			}

			_ = fs.Write("/profile/update.md", []byte{})
		}

		if _, err := store.RecordTurnInSession(run.SessionID, run.RunId, userMsg, final); err != nil {
			mustEmit(context.Background(), events.Event{
				Type:    "session.update.error",
				Message: "Failed to update session state",
				Data:    map[string]string{"err": err.Error()},
				Store:   boolp(false),
			})
		} else {
			mustEmit(context.Background(), events.Event{
				Type:    "session.update",
				Message: "Updated session state",
				Data:    map[string]string{"sessionId": run.SessionID, "runId": run.RunId},
				Store:   boolp(false),
			})
		}

		rr.Printf("agent> %s\n", final)
		mustEmit(context.Background(), events.Event{
			Type:    "agent.final",
			Message: "Agent produced final answer",
			Data:    map[string]string{"text": final},
			Console: boolp(false),
		})
	}

	mustEmit(context.Background(), events.Event{
		Type:    "run.completed",
		Message: "Run finished",
		Data:    data,
		Console: boolp(false),
	})

	log.Printf("== Workbench complete ==")
	return nil
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// formatRunMemoryAppend produces the exact block appended to memory.md when a memory update
// is accepted by the host.
func formatRunMemoryAppend(update string) string {
	update = strings.TrimSpace(update)
	if update == "" {
		return ""
	}
	return "\n\n—\n" + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + update + "\n"
}

type lineReader interface {
	ReadLine(prompt string) (string, error)
}

const (
	userPrompt         = "you> "
	continuationPrompt = "...> "
)

func readUserMessage(lr lineReader, out io.Writer) (msg string, exit bool, err error) {
	line, err := lr.ReadLine(userPrompt)
	if err != nil && !errors.Is(err, io.EOF) {
		if errors.Is(err, readline.ErrInterrupt) {
			return "", true, nil
		}
		return "", false, err
	}
	if errors.Is(err, io.EOF) {
		return "", true, nil
	}

	line = strings.ReplaceAll(line, "\r", "")
	line = strings.TrimRight(line, "\n")

	trim := strings.TrimSpace(line)
	switch trim {
	case "exit", "quit":
		return "", true, nil
	case ":paste":
		msg, exit, err := readMultilinePaste(lr, out)
		if err != nil || exit {
			return "", exit, err
		}
		edited, err := maybeEditMessage(lr, out, msg)
		return edited, false, err
	case ":compose":
		edited, err := maybeEditMessage(lr, out, "")
		return edited, false, err
	default:
		// If the terminal paste bracket leaked into the input, strip it.
		line = strings.TrimPrefix(line, "\x1b[200~")
		line = strings.TrimPrefix(line, "\x1b[201~")
		return line, false, nil
	}
}

func userInputHelp() string {
	return strings.TrimSpace(`
Commands:
  - exit / quit: exit the chat session
  - :reset: clear in-process conversation (does not delete /history)
  - :paste: multi-line paste mode (end with a line containing only ".")
  - :compose: open $EDITOR/$VISUAL to compose a message

Notes:
  - Workbench uses a REPL; each submitted message becomes one agent turn.
  - For long or multi-line messages, use :paste or :compose.
`) + "\n\n"
}

func readMultilinePaste(lr lineReader, out io.Writer) (msg string, exit bool, err error) {
	_, _ = io.WriteString(out, "paste mode (end with a line containing only \".\"; :abort cancels)\n")
	var b strings.Builder
	for {
		line, readErr := lr.ReadLine(continuationPrompt)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", false, readErr
		}
		if errors.Is(readErr, io.EOF) && line == "" {
			return "", true, nil
		}
		line = strings.ReplaceAll(line, "\r", "")
		line = strings.TrimRight(line, "\n")
		trim := strings.TrimSpace(line)
		switch trim {
		case ".":
			return b.String(), false, nil
		case ":abort":
			return "", false, nil
		case "exit", "quit":
			return "", true, nil
		default:
			b.WriteString(line)
			b.WriteString("\n")
		}
	}
}

func maybeEditMessage(lr lineReader, out io.Writer, initial string) (string, error) {
	edited, err := editMessageInEditor(initial)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(edited) == "" {
		return "", nil
	}
	ok, err := confirmSend(lr, out)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return edited, nil
}

func confirmSend(lr lineReader, out io.Writer) (bool, error) {
	_, _ = io.WriteString(out, "press Enter to send (or type 'abort' to cancel)\n")
	line, err := lr.ReadLine("send> ")
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	if strings.TrimSpace(line) == "abort" {
		return false, nil
	}
	return true, nil
}

func editMessageInEditor(initial string) (string, error) {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		return initial, nil
	}

	tmp, err := os.CreateTemp("", "workbench-message-*.md")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	_ = os.Chmod(name, 0600)
	if _, err := tmp.WriteString(initial); err != nil {
		_ = tmp.Close()
		_ = os.Remove(name)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(name)
		return "", err
	}
	defer os.Remove(name)

	fields := strings.Fields(editor)
	if len(fields) == 0 {
		return initial, nil
	}
	cmd := exec.Command(fields[0], append(fields[1:], name)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", err
	}

	b, err := os.ReadFile(name)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
