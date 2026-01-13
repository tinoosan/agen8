package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/tinoosan/workbench-core/internal/agent"
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

	fs.Mount(vfs.MountWorkspace, workspace)
	log.Printf("mounted /workspace => %s", workspace.BaseDir)
	fs.Mount(vfs.MountResults, results)
	log.Printf("mounted /results => %s", results.BaseDir)
	fs.Mount(vfs.MountTrace, trace)
	log.Printf("mounted /trace => %s", trace.BaseDir)
	fs.Mount(vfs.MountTools, toolsResource)
	log.Printf("mounted /tools => %s", toolsResource.BaseDir)

	absWorkspaceRoot, err := filepath.Abs(workspace.BaseDir)
	if err != nil {
		log.Fatalf("error resolving workspace root absolute path: %v", err)
	}

	builtinCfg := tools.BuiltinConfig{BashRootDir: absWorkspaceRoot}

	runner := tools.Runner{
		FS:           fs,
		ToolRegistry: tools.BuiltinInvokerRegistry(builtinCfg),
	}

	log.Printf("== Simulating: user request -> agent loop -> result ==")

	userRequest := "Show me what's in the workspace directory, then get the latest quote for AAPL. Save the raw quote as JSON and write a short markdown summary."
	log.Printf("user -> agent: %q", userRequest)
	emit("user.request", "User request received", map[string]string{
		"text": userRequest,
	})

	executor := &agent.HostOpExecutor{FS: fs, Runner: &runner, DefaultMaxBytes: 4096}
	exec := func(req types.HostOpRequest) types.HostOpResponse { return executor.Exec(context.Background(), req) }

	agentSay := func(req types.HostOpRequest) types.HostOpResponse {
		emit("agent.op.request", "Agent requested host op", map[string]string{
			"op":       req.Op,
			"path":     req.Path,
			"toolId":   req.ToolID.String(),
			"actionId": req.ActionID,
		})
		resp := agent.AgentSay(log.Printf, exec, req)
		data := map[string]string{
			"op":  resp.Op,
			"ok":  strconv.FormatBool(resp.Ok),
			"err": resp.Error,
		}
		if resp.BytesLen != 0 {
			data["bytesLen"] = strconv.Itoa(resp.BytesLen)
		}
		if resp.Truncated {
			data["truncated"] = "true"
		}
		if resp.ToolResponse != nil {
			data["callId"] = resp.ToolResponse.CallID
		}
		emit("agent.op.response", "Host op completed", data)
		return resp
	}

	emit("agent.loop.start", "Agent loop started", map[string]string{})

	// Observe environment and recent trace.
	agentSay(types.HostOpRequest{Op: "fs.list", Path: "/"})
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/trace/events.latest/10", MaxBytes: 2048})

	// Discover tools and read the chosen tool manifest.
	toolsList := agentSay(types.HostOpRequest{Op: "fs.list", Path: "/tools"})
	if !toolsList.Ok {
		log.Fatalf("agent loop failed: cannot list tools: %s", toolsList.Error)
	}
	emit("agent.plan", "First list workspace via builtin.bash, then fetch quote via github.com.acme.stock", map[string]string{})

	// 1) Use builtin.bash exec to list the workspace (agent discovers it via /tools).
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/tools/builtin.bash", MaxBytes: 2048})
	lsResp := agentSay(types.HostOpRequest{
		Op:       "tool.run",
		ToolID:   types.ToolID("builtin.bash"),
		ActionID: "exec",
		Input:    json.RawMessage(`{"argv":["ls","-la"],"cwd":"."}`),
	})
	if !lsResp.Ok || lsResp.ToolResponse == nil {
		log.Fatalf("agent loop failed: builtin.bash tool.run did not return toolResponse: %s", lsResp.Error)
	}
	lsCallID := lsResp.ToolResponse.CallID
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/results/" + lsCallID + "/response.json", MaxBytes: 4096})

	// 2) Use the stock quote tool (example builtin registered in memory).
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/tools/github.com.acme.stock", MaxBytes: 2048})

	// Execute tool via host primitive.
	runResp := agentSay(types.HostOpRequest{
		Op:        "tool.run",
		ToolID:    types.ToolID("github.com.acme.stock"),
		ActionID:  "quote.latest",
		Input:     json.RawMessage(`{"symbol":"AAPL"}`),
		TimeoutMs: 0,
	})
	if !runResp.Ok || runResp.ToolResponse == nil {
		log.Fatalf("agent loop failed: tool.run did not return toolResponse: %s", runResp.Error)
	}

	callID := runResp.ToolResponse.CallID

	// Read persisted results (what the agent would do next).
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/results/" + callID + "/response.json", MaxBytes: 4096})
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/results/" + callID + "/quote.json", MaxBytes: 2048})
	agentSay(types.HostOpRequest{Op: "fs.read", Path: "/results/" + callID + "/notes.md", MaxBytes: 2048})

	// Write a final answer to workspace (simulated "agent response").
	finalAnswer := "Latest quote for AAPL retrieved. See /results/" + callID + "/quote.json and /results/" + callID + "/notes.md."
	agentSay(types.HostOpRequest{Op: "fs.write", Path: "/workspace/final.md", Text: finalAnswer})
	emit("agent.final", "Agent produced final answer", map[string]string{
		"path":   "/workspace/final.md",
		"callId": callID,
	})

	log.Printf("agent -> user:\n%s", finalAnswer)
	emit("run.completed", "Run finished", data)

	log.Printf("== Workbench demo complete ==")
}
