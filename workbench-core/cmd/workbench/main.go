package main

import (
	"bufio"
	"context"
	"encoding/json"
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
	"github.com/tinoosan/workbench-core/internal/llm"
	"github.com/tinoosan/workbench-core/internal/resources"
	"github.com/tinoosan/workbench-core/internal/store"
	"github.com/tinoosan/workbench-core/internal/tools"
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

	emit := func(eventType, message string, data map[string]string) {
		if err := store.AppendEvent(run.RunId, eventType, message, data); err != nil {
			log.Fatalf("error appending event: %v", err)
		}
	}

	data := map[string]string{
		"name":  "Alice",
		"email": "alice@example.com",
	}

	emit("run.started", "Run started", data)

	trace, err := resources.NewTraceResource(run.RunId)
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
	fs.Mount(vfs.MountTrace, trace)
	log.Printf("mounted /trace => %s", trace.BaseDir)
	fs.Mount(vfs.MountTools, toolsResource)
	log.Printf("mounted /tools => %s", toolsResource.BaseDir)
	fs.Mount(vfs.MountMemory, memoryRes)
	log.Printf("mounted /memory => %s", memoryRes.BaseDir)

	absWorkspaceRoot, err := filepath.Abs(workspace.BaseDir)
	if err != nil {
		log.Fatalf("error resolving workspace root absolute path: %v", err)
	}

	builtinCfg := tools.BuiltinConfig{BashRootDir: absWorkspaceRoot}

	runner := tools.Runner{
		FS:           fs,
		ToolRegistry: tools.BuiltinInvokerRegistry(builtinCfg),
	}

	executor := &agent.HostOpExecutor{FS: fs, Runner: &runner, DefaultMaxBytes: 4096}
	model := strings.TrimSpace(os.Getenv("OPENROUTER_MODEL"))
	if strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")) == "" || model == "" {
		log.Fatalf("OPENROUTER_API_KEY and OPENROUTER_MODEL are required to run the non-scripted agent loop")
	}

	client, err := llm.NewOpenRouterClientFromEnv()
	if err != nil {
		log.Fatalf("error creating OpenRouter client: %v", err)
	}

	systemPromptBytes, err := os.ReadFile("internal/agent/INITIAL_PROMPT.md")
	if err != nil {
		log.Fatalf("error reading internal/agent/INITIAL_PROMPT.md: %v", err)
	}
	baseSystemPrompt := string(systemPromptBytes)

	execWithEvents := func(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
		logEventLine("agent.op.request", "Agent requested host op", map[string]string{
			"op":       req.Op,
			"path":     req.Path,
			"toolId":   req.ToolID.String(),
			"actionId": req.ActionID,
		})
		emit("agent.op.request", "Agent requested host op", map[string]string{
			"op":       req.Op,
			"path":     req.Path,
			"toolId":   req.ToolID.String(),
			"actionId": req.ActionID,
		})
		resp := executor.Exec(ctx, req)

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
		logEventLine("agent.op.response", "Host op completed", respData)

		emit("agent.op.response", "Host op completed", map[string]string{
			"op":  resp.Op,
			"ok":  fmtBool(resp.Ok),
			"err": resp.Error,
		})
		return resp
	}

	emit("agent.loop.start", "Agent loop started", map[string]string{
		"model": model,
	})
	logEventLine("agent.loop.start", "Agent loop started", map[string]string{"model": model})

	updater := &agent.ContextUpdater{
		FS:             fs,
		MaxMemoryBytes: 8 * 1024,
		MaxTraceBytes:  8 * 1024,
		ManifestPath:   "/workspace/context_manifest.json",
		Emit: func(eventType, message string, data map[string]string) {
			logEventLine(eventType, message, data)
			emit(eventType, message, data)
		},
	}

	a := &agent.Agent{
		LLM:            client,
		Exec:           execWithEvents,
		Model:          model,
		SystemPrompt:   baseSystemPrompt,
		ContextUpdater: updater,
		MaxSteps:       20,
		Logf:           nil,
		OnLLMUsage:     nil,
	}

	// Interactive chat loop:
	//   - user types a message
	//   - agent runs the loop until {"op":"final"} and prints the result
	//   - host evaluates /memory/update.md and (optionally) commits it to /memory/memory.md
	//   - host appends an immutable audit line to /memory/commits.jsonl
	log.Printf("== Chat session started (type 'exit' to quit) ==")
	in := bufio.NewReader(os.Stdin)
	enableBracketedPaste(os.Stdout)
	defer disableBracketedPaste(os.Stdout)
	memEval := agent.DefaultMemoryEvaluator()
	turn := 0
	for {
		userMsg, exit, err := readUserMessage(in, os.Stdout)
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

		turn++
		emit("user.message", "User message received", map[string]string{"text": userMsg})

		// Reset per-turn usage accumulator.
		var turnUsage types.LLMUsage
		a.OnLLMUsage = func(step int, usage types.LLMUsage) {
			turnUsage.InputTokens += usage.InputTokens
			turnUsage.OutputTokens += usage.OutputTokens
			turnUsage.TotalTokens += usage.TotalTokens
			logEventLine("llm.usage", "Model usage", map[string]string{
				"step":      strconv.Itoa(step),
				"input":     strconv.Itoa(usage.InputTokens),
				"output":    strconv.Itoa(usage.OutputTokens),
				"total":     strconv.Itoa(usage.TotalTokens),
				"turnTotal": strconv.Itoa(turnUsage.TotalTokens),
			})
		}

		final, err := a.Run(context.Background(), userMsg)
		if err != nil {
			logEventLine("agent.error", "Agent loop error", map[string]string{"err": err.Error()})
			log.Printf("agent error: %v", err)
			continue
		}

		// Print a turn-level usage summary if the provider reported usage.
		if turnUsage.TotalTokens != 0 {
			logEventLine("llm.usage.total", "Turn usage total", map[string]string{
				"input":  strconv.Itoa(turnUsage.InputTokens),
				"output": strconv.Itoa(turnUsage.OutputTokens),
				"total":  strconv.Itoa(turnUsage.TotalTokens),
			})
		}

		// Ingest memory update if the agent wrote one.
		if b, err := fs.Read("/memory/update.md"); err == nil {
			updateRaw := string(b)
			if strings.TrimSpace(updateRaw) == "" {
				logEventLine("memory.evaluate", "No memory update written", map[string]string{
					"turn":     strconv.Itoa(turn),
					"accepted": "false",
					"reason":   "no_update",
					"bytes":    "0",
				})
				emit("memory.evaluate", "No memory update written", map[string]string{
					"turn":     strconv.Itoa(turn),
					"accepted": "false",
					"reason":   "no_update",
					"bytes":    "0",
				})
			} else {

				trimmed := strings.TrimSpace(updateRaw)
				hash := agent.SHA256Hex(trimmed)

				accepted, reason, cleaned := memEval.Evaluate(updateRaw)

				logEventLine("memory.evaluate", "Evaluated memory update", map[string]string{
					"turn":     strconv.Itoa(turn),
					"accepted": fmtBool(accepted),
					"reason":   reason,
					"bytes":    strconv.Itoa(len(trimmed)),
					"sha256":   hash[:12],
				})
				emit("memory.evaluate", "Evaluated memory update", map[string]string{
					"turn":     strconv.Itoa(turn),
					"accepted": fmtBool(accepted),
					"reason":   reason,
					"bytes":    strconv.Itoa(len(trimmed)),
					"sha256":   hash[:12],
				})

				if accepted {
					// Commit to run-scoped memory.md on disk (host policy).
					if err := appendRunMemory(memoryRes.BaseDir, strings.TrimSpace(cleaned)); err != nil {
						logEventLine("memory.commit.error", "Failed to commit memory update", map[string]string{"err": err.Error()})
					} else {
						logEventLine("memory.commit", "Committed memory update", map[string]string{
							"turn":   strconv.Itoa(turn),
							"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
							"sha256": hash[:12],
						})
						emit("memory.commit", "Committed memory update", map[string]string{
							"turn":   strconv.Itoa(turn),
							"bytes":  strconv.Itoa(len(strings.TrimSpace(cleaned))),
							"sha256": hash[:12],
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
					logEventLine("memory.audit.error", "Failed to append memory audit log", map[string]string{
						"turn": strconv.Itoa(turn),
						"err":  err.Error(),
					})
				} else {
					logEventLine("memory.audit.append", "Appended memory audit log", map[string]string{
						"turn":     strconv.Itoa(turn),
						"accepted": fmtBool(accepted),
						"reason":   reason,
						"sha256":   hash[:12],
					})
				}
			}

			_ = fs.Write("/memory/update.md", []byte{})
		}

		// Print the final agent response last, after host-side housekeeping logs.
		// This keeps the terminal output easy to follow during interactive sessions.
		log.Printf("agent> %s", final)
		logEventLine("agent.final", "Agent produced final answer", map[string]string{"text": final})
		emit("agent.final", "Agent produced final answer", map[string]string{"text": final})
	}

	emit("run.completed", "Run finished", data)

	log.Printf("== Workbench demo complete ==")
}

func fmtBool(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// logEventLine prints a single-line JSON log similar to events.jsonl.
//
// It is intentionally compact so you can follow the agent loop in the terminal
// without opening the on-disk event log.
func logEventLine(eventType, message string, data map[string]string) {
	line := struct {
		Type    string            `json:"type"`
		Message string            `json:"message"`
		Data    map[string]string `json:"data,omitempty"`
	}{
		Type:    eventType,
		Message: message,
		Data:    data,
	}
	b, err := json.Marshal(line)
	if err != nil {
		log.Printf("%s %s", eventType, message)
		return
	}
	log.Printf("%s", string(b))
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
	bracketedPasteEnable  = "\x1b[?2004h"
	bracketedPasteDisable = "\x1b[?2004l"
	bracketedPasteStart   = "\x1b[200~"
	bracketedPasteEnd     = "\x1b[201~"
)

// enableBracketedPaste asks the terminal to wrap pasted text in sentinel escape sequences.
//
// This makes multi-line paste usable in a line-oriented REPL:
//   - the terminal sends "\x1b[200~" before pasted content and "\x1b[201~" after
//   - the REPL can buffer until it sees the end sentinel and treat the entire paste as one message
//
// If the terminal does not support bracketed paste, it is a harmless no-op.
func enableBracketedPaste(w io.Writer) {
	_, _ = io.WriteString(w, bracketedPasteEnable)
}

// disableBracketedPaste disables terminal bracketed paste mode.
func disableBracketedPaste(w io.Writer) {
	_, _ = io.WriteString(w, bracketedPasteDisable)
}

// readUserMessage reads one "user turn" from stdin.
//
// It supports three ways to compose a message:
//  1. Single-line input: type a line and press Enter.
//  2. Multi-line paste: paste text; the REPL buffers until the terminal signals paste end.
//  3. Explicit compose modes:
//     - :paste   (multi-line; end with a line containing only ".")
//     - :compose (opens $VISUAL/$EDITOR)
//
// This is intentionally "boring" (no readline deps). The goal is to avoid the UX bug where
// multi-line paste turns into multiple agent turns and looks like the agent keeps running
// after it already produced a final answer.
func readUserMessage(in *bufio.Reader, out io.Writer) (msg string, exit bool, err error) {
	for {
		_, _ = io.WriteString(out, "you> ")
		raw, readErr := in.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return "", false, readErr
		}
		if errors.Is(readErr, io.EOF) && raw == "" {
			return "", true, nil
		}

		// If the terminal wrapped a paste, buffer until we see the end sentinel.
		if strings.Contains(raw, bracketedPasteStart) && !strings.Contains(raw, bracketedPasteEnd) && !errors.Is(readErr, io.EOF) {
			var b strings.Builder
			b.WriteString(raw)
			for {
				next, nextErr := in.ReadString('\n')
				b.WriteString(next)
				if strings.Contains(next, bracketedPasteEnd) || strings.Contains(b.String(), bracketedPasteEnd) || errors.Is(nextErr, io.EOF) {
					raw = b.String()
					readErr = nextErr
					break
				}
				if nextErr != nil && !errors.Is(nextErr, io.EOF) {
					return "", false, nextErr
				}
			}
		}

		raw = strings.ReplaceAll(raw, "\r", "")
		raw = strings.ReplaceAll(raw, bracketedPasteStart, "")
		raw = strings.ReplaceAll(raw, bracketedPasteEnd, "")
		raw = strings.TrimRight(raw, "\n")

		line := strings.TrimSpace(raw)
		if line == "" {
			if errors.Is(readErr, io.EOF) {
				return "", true, nil
			}
			continue
		}

		switch line {
		case "exit", "quit":
			return "", true, nil
		case ":help", "help":
			printREPLHelp(out)
			continue
		case ":paste":
			msg, exit, err := readMultilinePaste(in, out)
			if err != nil {
				return "", false, err
			}
			if exit {
				return "", true, nil
			}
			if strings.TrimSpace(msg) == "" {
				continue
			}
			edited, err := maybeEditMessage(in, out, msg)
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
			if ok, err := confirmSend(in, out); err != nil {
				return "", false, err
			} else if !ok {
				continue
			}
			return strings.TrimSpace(edited), false, nil
		default:
			// If the user pasted multi-line content, open an editor so they can adjust it
			// before sending it to the agent (better UX than trying to edit inline).
			if strings.Contains(raw, "\n") {
				edited, err := maybeEditMessage(in, out, raw)
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
  exit/quit       leave the chat session

Tips:
  - Multi-line paste is supported when your terminal supports "bracketed paste".
    If paste still submits line-by-line, use :paste or :compose.
`)+"\n\n")
}

// readMultilinePaste reads multi-line input until the user terminates it.
//
// End markers:
//   - a line containing only "." sends the message
//   - ":abort" cancels the message
//   - "exit"/"quit" exits the chat session
func readMultilinePaste(in *bufio.Reader, out io.Writer) (msg string, exit bool, err error) {
	_, _ = io.WriteString(out, "paste mode (end with a line containing only \".\"; :abort cancels)\n")
	var b strings.Builder
	for {
		_, _ = io.WriteString(out, "...> ")
		line, readErr := in.ReadString('\n')
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
func maybeEditMessage(in *bufio.Reader, out io.Writer, initial string) (string, error) {
	edited, err := editMessageInEditor(initial)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(edited) == "" {
		return "", nil
	}
	ok, err := confirmSend(in, out)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	return edited, nil
}

// confirmSend prompts the user to press Enter to send the composed message.
func confirmSend(in *bufio.Reader, out io.Writer) (bool, error) {
	_, _ = io.WriteString(out, "press Enter to send (or type 'abort' to cancel)\n")
	_, _ = io.WriteString(out, "send> ")
	line, err := in.ReadString('\n')
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
