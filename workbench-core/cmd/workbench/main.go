package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"os"
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
	in := bufio.NewScanner(os.Stdin)
	memEval := agent.DefaultMemoryEvaluator()
	turn := 0
	for {
		log.Printf("you> ")
		if !in.Scan() {
			break
		}
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			break
		}

		turn++
		emit("user.message", "User message received", map[string]string{"text": line})

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

		final, err := a.Run(context.Background(), line)
		if err != nil {
			logEventLine("agent.error", "Agent loop error", map[string]string{"err": err.Error()})
			log.Printf("agent error: %v", err)
			continue
		}
		log.Printf("agent> %s", final)
		logEventLine("agent.final", "Agent produced final answer", map[string]string{"text": final})
		emit("agent.final", "Agent produced final answer", map[string]string{"text": final})

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
				continue
			}

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

			_ = fs.Write("/memory/update.md", []byte{})
		}
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
