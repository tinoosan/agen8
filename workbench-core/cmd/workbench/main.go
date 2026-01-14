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

	memoryRes, err := resources.NewMemoryResource()
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

	// Keep the default goal intentionally vague so the agent has to discover
	// the environment (/tools, /trace, /results, /workspace) and choose actions.
	userRequest := "Test the builtin.bash tool first: fetch https://example.com and write the response body to /workspace/example.html. Then summarize what you did and what you observed about the environment."
	if len(os.Args) > 1 {
		userRequest = strings.Join(os.Args[1:], " ")
	}
	log.Printf("user -> agent: %q", userRequest)
	emit("user.request", "User request received", map[string]string{
		"text": userRequest,
	})

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

	memoryPath := agent.DefaultPersistentMemoryPath()
	memoryText, err := agent.LoadPersistentMemory(memoryPath)
	if err != nil {
		log.Fatalf("error reading persistent memory: %v", err)
	}
	systemPrompt := agent.BuildSystemPromptWithMemory(baseSystemPrompt, memoryText)

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

	a := &agent.Agent{
		LLM:          client,
		Exec:         execWithEvents,
		Model:        model,
		SystemPrompt: systemPrompt,
		MaxSteps:     20,
		Logf:         nil,
	}

	// Interactive chat loop:
	//   - user types a message
	//   - agent runs the loop until {"op":"final"} and prints the result
	//   - host ingests /workspace/memory_update.md and persists it across sessions
	log.Printf("== Chat session started (type 'exit' to quit) ==")
	in := bufio.NewScanner(os.Stdin)
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

		emit("user.message", "User message received", map[string]string{"text": line})
		final, err := a.Run(context.Background(), line)
		if err != nil {
			logEventLine("agent.error", "Agent loop error", map[string]string{"err": err.Error()})
			log.Printf("agent error: %v", err)
			continue
		}
		log.Printf("agent> %s", final)
		logEventLine("agent.final", "Agent produced final answer", map[string]string{"text": final})
		emit("agent.final", "Agent produced final answer", map[string]string{"text": final})

		// Ingest memory update if the agent wrote one.
		if b, err := fs.Read("/memory/update.md"); err == nil {
			update := strings.TrimSpace(string(b))
			if update != "" {
				if err := agent.AppendPersistentMemory(memoryPath, update); err != nil {
					logEventLine("agent.memory.error", "Failed to persist memory update", map[string]string{"err": err.Error()})
				} else {
					logEventLine("agent.memory.append", "Persisted memory update", map[string]string{"bytes": strconv.Itoa(len(update))})
					// Refresh in-memory system prompt for subsequent turns.
					memoryText = memoryText + "\n\n" + update
					systemPrompt = agent.BuildSystemPromptWithMemory(baseSystemPrompt, memoryText)
					a.SystemPrompt = systemPrompt
				}
				_ = fs.Write("/memory/update.md", []byte{})
			}
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
