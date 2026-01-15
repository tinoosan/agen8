package main

import (
	"context"
	"errors"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/tinoosan/workbench-core/internal/agent"
	"github.com/tinoosan/workbench-core/internal/events"
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/repl"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
	"github.com/tinoosan/workbench-core/internal/trace"
	"github.com/tinoosan/workbench-core/internal/types"
	"github.com/tinoosan/workbench-core/internal/vfs"
)

func main() {
	log.Printf("== Workbench demo: agent loop simulation ==")

	run, err := store.CreateRun("demo: user request -> agent loop -> result", 100)
	if err != nil {
		log.Fatalf("error creating run: %v", err)
	}
	log.Printf("runId=%s", run.RunId)

	historyRes, err := resources.NewRunHistoryResource(run.RunId)
	if err != nil {
		log.Fatalf("error creating history: %v", err)
	}
	historySink := &events.HistorySink{BaseDir: historyRes.BaseDir}

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

	// Store-only run lifecycle events (kept identical to previous behavior).
	mustEmit(context.Background(), events.Event{
		Type:    "run.started",
		Message: "Run started",
		Data:    data,
		Console: boolp(false),
	})

	traceRes, err := resources.NewTraceResource(run.RunId)
	if err != nil {
		log.Fatalf("error creating trace: %v", err)
	}

	fs := vfs.NewFS()

	workspace, err := resources.NewRunWorkspace(run.RunId)
	if err != nil {
		log.Fatalf("error creating workspace: %v", err)
	}

	toolsResource, err := resources.NewToolsResource()
	if err != nil {
		log.Fatalf("error creating tools: %v", err)
	}
	if err := tools.RegisterBuiltinManifests(toolsResource); err != nil {
		log.Fatalf("error registering builtin tool manifests: %v", err)
	}

	// Disk tools under data/tools/<toolId>/manifest.json
	if err := os.MkdirAll(filepath.Join(toolsResource.BaseDir, "github.com.other.stock"), 0755); err != nil {
		log.Fatalf("error creating disk tool directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(toolsResource.BaseDir, "github.com.other.stock", "manifest.json"),
		[]byte(`{"id":"github.com.other.stock","version":"0.1.0","kind":"custom","displayName":"Other Stock","description":"Example disk tool","actions":[{"id":"quote.latest","displayName":"Latest Quote","description":"Fetch latest quote for a symbol","inputSchema":{"type":"object","properties":{"symbol":{"type":"string"}},"required":["symbol"]},"outputSchema":{"type":"object","properties":{"symbol":{"type":"string"},"price":{"type":"number"}},"required":["symbol","price"]}}]}`),
		0644,
	); err != nil {
		log.Fatalf("error writing disk tool manifest: %v", err)
	}
	// Collision on disk: should be ignored on read due to builtin override
	if err := os.MkdirAll(filepath.Join(toolsResource.BaseDir, "github.com.dupe.tool"), 0755); err != nil {
		log.Fatalf("error creating collision tool directory: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(toolsResource.BaseDir, "github.com.dupe.tool", "manifest.json"),
		[]byte(`{"id":"github.com.dupe.tool","version":"0.1.0","kind":"custom","displayName":"Dupe Tool (disk)","description":"Should not be returned","actions":[{"id":"dupe.noop","displayName":"No-op","description":"Returns an empty object","inputSchema":{"type":"object"},"outputSchema":{"type":"object"}}]}`),
		0644,
	); err != nil {
		log.Fatalf("error writing collision tool manifest: %v", err)
	}

	results, err := resources.NewRunResults(run.RunId)
	if err != nil {
		log.Fatalf("error creating results: %v", err)
	}

	memoryRes, err := resources.NewRunMemoryResource(run.RunId)
	if err != nil {
		log.Fatalf("error creating memory: %v", err)
	}

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted /workspace => %s", workspace.BaseDir)
	fs.Mount(vfs.MountResults, results)
	log.Printf("mounted /results => %s", results.BaseDir)
	fs.Mount(vfs.MountTrace, traceRes)
	log.Printf("mounted /trace => %s", traceRes.BaseDir)
	fs.Mount(vfs.MountTools, toolsResource)
	log.Printf("mounted /tools => %s", toolsResource.BaseDir)
	fs.Mount(vfs.MountMemory, memoryRes)
	log.Printf("mounted /memory => %s", memoryRes.BaseDir)
	fs.Mount(vfs.MountHistory, historyRes)
	log.Printf("mounted /history => %s", historyRes.BaseDir)

	absWorkspaceRoot, err := filepath.Abs(workspace.BaseDir)
	if err != nil {
		log.Fatalf("error resolving workspace root absolute path: %v", err)
	}

	traceStore := trace.DiskTraceStore{Dir: traceRes.BaseDir}
	builtinCfg := tools.BuiltinConfig{
		BashRootDir: absWorkspaceRoot,
		TraceStore:  traceStore,
	}

	runner := tools.Runner{
		FS:           fs,
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
		log.Fatalf("OPENROUTER_API_KEY and OPENROUTER_MODEL are required to run the non-scripted agent loop")
	}
	historySink.Model = model

	client, err := llm.NewOpenRouterClientFromEnv()
	if err != nil {
		log.Fatalf("error creating OpenRouter client: %v", err)
	}

	systemPromptBytes, err := os.ReadFile("internal/agent/INITIAL_PROMPT.md")
	if err != nil {
		log.Fatalf("error reading internal/agent/INITIAL_PROMPT.md: %v", err)
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
		// Preserve existing divergence: console includes extra fields (e.g. maxBytes),
		// while the store log keeps a minimal schema.
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

	// Interactive chat loop:
	//   - user types a message
	//   - agent runs the loop until {"op":"final"} and prints the result
	//   - host evaluates /memory/update.md and (optionally) commits it to /memory/memory.md
	//   - host appends an immutable audit line to /memory/commits.jsonl
	log.Printf("== Chat session started (type 'exit' to quit) ==")
	historyPath := filepath.Join("data", "runs", run.RunId, "repl_history.txt")
	rr, err := repl.NewReader(historyPath)
	if err != nil {
		log.Fatalf("error starting readline: %v", err)
	}
	// Route all log output (including event JSON lines) through readline so terminal
	// redraw stays correct while the user is typing.
	//
	// Without this, log output can interleave with the input prompt/line buffer and
	// make the UI look "broken" (shifted/indented lines).
	oldLogWriter := log.Writer()
	log.SetOutput(rr)
	defer rr.Close()
	defer log.SetOutput(oldLogWriter)
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

		turn++
		mustEmit(context.Background(), events.Event{
			Type:    "user.message",
			Message: "User message received",
			Data:    map[string]string{"text": userMsg},
			Console: boolp(false),
		})

		// Reset per-turn usage accumulator.
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
		final, updated, steps, err := a.RunConversation(context.Background(), conversation)
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

		// Print a turn-level usage summary if the provider reported usage.
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
					// Commit to run-scoped memory.md on disk (host policy).
					if err := appendRunMemory(memoryRes.BaseDir, strings.TrimSpace(cleaned)); err != nil {
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

				if err := agent.AppendCommitLog(memoryRes.BaseDir, agent.MemoryCommitLine{
					Model:    model,
					Turn:     turn,
					Accepted: accepted,
					Reason:   reason,
					Bytes:    len(trimmed),
					SHA256:   hash,
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

		// Print the final agent response last, after host-side housekeeping logs.
		// This keeps the terminal output easy to follow during interactive sessions.
		rr.Printf("agent> %s\n", final)
		mustEmit(context.Background(), events.Event{
			Type:    "agent.final",
			Message: "Agent produced final answer",
			Data:    map[string]string{"text": final},
		})
	}

	mustEmit(context.Background(), events.Event{
		Type:    "run.completed",
		Message: "Run finished",
		Data:    data,
		Console: boolp(false),
	})

	log.Printf("== Workbench demo complete ==")
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

func appendRunMemory(baseDir string, update string) error {
	update = strings.TrimSpace(update)
	if update == "" {
		return nil
	}
	p := filepath.Join(baseDir, "memory.md")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString("\n\n---\n" + time.Now().UTC().Format(time.RFC3339Nano) + "\n\n" + update + "\n")
	return err
}

const (
	primaryPrompt      = "you> "
	continuationPrompt = "...> "
)

type lineReader interface {
	ReadLine(prompt string) (string, error)
}

// readUserMessage reads one "user turn" from stdin.
//
// It supports three ways to compose a message:
//  1. Single-line input: type a line and press Enter.
//  2. Multi-line paste: paste text; the editor captures it into a single message.
//  3. Explicit compose modes (recommended for long input):
//     - :paste   (multi-line; end with a line containing only ".")
//     - :compose (opens $VISUAL/$EDITOR)
//
// This is intentionally "boring" (no readline deps). The goal is to avoid the UX bug where
// multi-line paste turns into multiple agent turns and looks like the agent keeps running
// after it already produced a final answer.
func readUserMessage(lr lineReader, out io.Writer) (msg string, exit bool, err error) {
	for {
		raw, readErr := lr.ReadLine(primaryPrompt)
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", false, readErr
		}
		if errors.Is(readErr, io.EOF) && raw == "" {
			return "", true, nil
		}

		raw = strings.ReplaceAll(raw, "\r", "")
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}

		switch line {
		case "exit", "quit":
			return "", true, nil
		case ":help", "help":
			printREPLHelp(out)
			continue
		case ":paste":
			msg, exit, err := readMultilinePaste(lr, out)
			if err != nil {
				return "", false, err
			}
			if exit {
				return "", true, nil
			}
			if strings.TrimSpace(msg) == "" {
				continue
			}
			edited, err := maybeEditMessage(lr, out, msg)
			if err != nil {
				return "", false, err
			}
			return strings.TrimSpace(edited), false, nil
		case ":compose", ":edit":
			edited, err := editMessageInEditor("")
			if err != nil {
				return "", false, err
			}
			if strings.TrimSpace(edited) == "" {
				continue
			}
			if ok, err := confirmSend(lr, out); err != nil {
				return "", false, err
			} else if !ok {
				continue
			}
			return strings.TrimSpace(edited), false, nil
		default:
			// If the user pasted multi-line content, open an editor so they can adjust it
			// before sending it to the agent (better UX than trying to edit inline).
			if strings.Contains(raw, "\n") {
				edited, err := maybeEditMessage(lr, out, raw)
				if err != nil {
					return "", false, err
				}
				return strings.TrimSpace(edited), false, nil
			}
			return line, false, nil
		}
	}
}

func printREPLHelp(w io.Writer) {
	_, _ = io.WriteString(w, strings.TrimSpace(`
Commands:
  :help           show this help
  :paste          enter multi-line mode (end with a line containing only ".")
  :compose/:edit  open $VISUAL/$EDITOR to write a message
  :reset          clear conversation history for this session
  exit/quit       leave the chat session

Tips:
  - Arrow keys + paste are supported in the REPL.
  - For long or multi-line messages, use :paste or :compose.
`)+"\n\n")
}

// readMultilinePaste reads multi-line input until the user terminates it.
//
// End markers:
//   - a line containing only "." sends the message
//   - ":abort" cancels the message
//   - "exit"/"quit" exits the chat session
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

// maybeEditMessage opens an editor (if configured) and asks for send confirmation.
//
// This is primarily used for multi-line pasted messages: the user can paste, edit, then
// press Enter to submit as a single agent turn.
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

// confirmSend prompts the user to press Enter to send the composed message.
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

// editMessageInEditor opens $VISUAL or $EDITOR to edit a message, returning the final text.
//
// If no editor is configured, it returns the original text unchanged.
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
