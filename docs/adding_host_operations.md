# Adding Host Operations to Agen8

This document explains the architecture for adding new host operations (like `browser`, `shell_exec`, `http_fetch`) to the Agen8 agent runtime.

## Architecture Overview

Host operations follow a **four-layer pattern**:

```
┌─────────────────────────────────────────────────────────────┐
│ 1. Tool Definition (pkg/agent/hosttools/*.go)              │
│    - Defines LLM-facing schema                             │
│    - Converts tool call args → HostOpRequest               │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 2. Protocol (pkg/types/host_op_protocol.go)                │
│    - Defines operation constants (HostOpBrowser, etc.)     │
│    - Validates requests                                     │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 3. Executor (pkg/agent/host_ops_mock.go)                   │
│    - Big switch statement dispatches operations            │
│    - Returns HostOpResponse                                 │
└─────────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────────┐
│ 4. Invoker/State (pkg/tools/builtins/*.go)                 │
│    - Actual implementation (e.g., session managers)        │
│    - Stateful components live here                         │
└─────────────────────────────────────────────────────────────┘
```

---

## Step-by-Step Guide: Adding a New Host Operation

### Example: Adding the `browser` operation

#### Step 1: Define the Tool (`pkg/agent/hosttools/browser.go`)

Create a tool struct that implements the tool interface:

```go
package hosttools

type BrowserTool struct{}

func (t *BrowserTool) Definition() llmtypes.Tool {
    return llmtypes.Tool{
        Type: "function",
        Function: llmtypes.ToolFunction{
            Name:        "browser",
            Description: "Interactive web browser...",
            Strict:      true,
            Parameters: map[string]any{
                "type": "object",
                "properties": map[string]any{
                    "action": map[string]any{
                        "type": "string",
                        "enum": []string{"start", "navigate", "click", ...},
                        "description": "Browser action to perform",
                    },
                    // ... more params
                },
                "required": []any{"action"},
                "additionalProperties": false,
            },
        },
    }
}

func (t *BrowserTool) Execute(_ context.Context, args json.RawMessage) (types.HostOpRequest, error) {
    // Parse args, validate, return HostOpRequest
    return types.HostOpRequest{
        Op:    types.HostOpBrowser,
        Input: args, // Pass raw JSON for executor to parse
    }, nil
}
```

**Pattern**: Tool definitions are **stateless converters** from LLM tool calls to `HostOpRequest`.

---

#### Step 2: Add Protocol Constant (`pkg/types/host_op_protocol.go`)

Define the operation constant:

```go
const (
    // ... existing ops
    HostOpBrowser = "browser"
)
```

Add validation in `HostOpRequest.Validate()`:

```go
case HostOpBrowser:
    if r.Input == nil {
        return fmt.Errorf("browser.input is required")
    }
    return nil
```

---

#### Step 3: Implement Execution (`pkg/agent/host_ops_mock.go`)

Add a case to the `HostOpExecutor.Exec()` switch statement:

```go
func (x *HostOpExecutor) Exec(ctx context.Context, req types.HostOpRequest) types.HostOpResponse {
    // ... existing validation
    
    switch req.Op {
    // ... existing cases (fs_list, fs_read, shell_exec, http_fetch, trace)
    
    case types.HostOpBrowser:
        if x.BrowserManager == nil {
            return types.HostOpResponse{Op: req.Op, Ok: false, Error: "browser not configured"}
        }
        
        var params struct {
            Action    string `json:"action"`
            SessionID string `json:"sessionId"`
            URL       string `json:"url"`
            // ... more fields
        }
        
        if err := json.Unmarshal(req.Input, &params); err != nil {
            return types.HostOpResponse{Op: req.Op, Ok: false, Error: err.Error()}
        }
        
        switch params.Action {
        case "start":
            session, err := x.BrowserManager.Start(ctx, true)
            if err != nil {
                return types.HostOpResponse{Op: "browser.start", Ok: false, Error: err.Error()}
            }
            return types.HostOpResponse{
                Op:   "browser.start",
                Ok:   true,
                Text: fmt.Sprintf(`{"sessionId": "%s"}`, session.ID),
            }
            
        case "navigate":
            // ... implementation
            
        default:
            return types.HostOpResponse{Op: req.Op, Ok: false, Error: "unknown action"}
        }
    
    default:
        return types.HostOpResponse{Op: req.Op, Ok: false, Error: "unknown op"}
    }
}
```

**Pattern**: Executor is a **big switch statement** that delegates to stateful components.

---

#### Step 4: Create Stateful Components (`pkg/tools/builtins/*.go`)

For operations that need state (sessions, invokers, etc.), create them in `pkg/tools/builtins/`:

```go
// pkg/tools/builtins/browser_session.go
package builtins

type BrowserSessionManager struct {
    mu       sync.RWMutex
    sessions map[string]*BrowserSession
    // ... state
}

func NewBrowserSessionManager(timeout time.Duration) (*BrowserSessionManager, error) {
    // Initialize
}

func (m *BrowserSessionManager) Start(ctx context.Context, headless bool) (*BrowserSession, error) {
    // Create session
}

func (m *BrowserSessionManager) Get(sessionID string) (*BrowserSession, error) {
    // Retrieve session
}

// ... more methods
```

**Pattern**: Stateful components (invokers, session managers, caches) live in `pkg/tools/builtins/`.

---

#### Step 5: Wire into Runtime (`pkg/runtime/runtime.go`)

Initialize your component in `Build()`:

```go
func Build(cfg BuildConfig) (*Runtime, error) {
    // ... existing setup
    
    // Initialize browser manager
    browserManager, err := builtins.NewBrowserSessionManager(30 * time.Minute)
    if err != nil {
        return nil, fmt.Errorf("create browser manager: %w", err)
    }
    
    // Wire into executor
    executor := &agent.HostOpExecutor{
        FS:              r.FS,
        ShellInvoker:    builtinInvokers.Invoke,
        HTTPInvoker:     builtinInvokers.Invoke,
        TraceInvoker:    builtinInvokers.Invoke,
        BrowserManager:  browserManager,  // NEW
        WorkspaceDir:    workspaceDir,
        DefaultMaxBytes: defaultMaxBytes,
        MaxReadBytes:    maxReadBytes,
    }
    
    // Start cleanup goroutine (if needed)
    go func() {
        ticker := time.NewTicker(5 * time.Minute)
        defer ticker.Stop()
        for range ticker.C {
            browserManager.CleanupStale()
        }
    }()
    
    // Store in runtime for shutdown
    return &Runtime{
        // ... existing fields
        BrowserManager: browserManager,
    }, nil
}
```

Add cleanup in daemon shutdown:

```go
// In RunDaemon or similar
if rt.BrowserManager != nil {
    rt.BrowserManager.Shutdown()
}
```

**Pattern**: Runtime wires everything together and manages lifecycle.

---

#### Step 6: Register Tool (`pkg/agent/hosttools/`)

Ensure the tool is registered (typically in daemon or runtime setup):

```go
browserTool := &hosttools.BrowserTool{}
if err := hostToolRegistry.Register(browserTool); err != nil {
    return err
}
```

---

#### Step 7: Update System Prompt (`pkg/agent/loop.go`)

Add the operation to the agent's capabilities:

```xml
<op name="browser">Interactive web browser for JavaScript-rendered content, form automation, and visual verification. Use browser.start to create a session, then navigate/click/type/extract/screenshot/pdf. Always close sessions when done.</op>
```

Add usage guidance:

```xml
<browser_usage>
Use browser for:
- Sites requiring JavaScript (SPAs, dynamic content)
- Multi-step interactions (login, forms, navigation)
- Visual verification (screenshots, PDFs)

Use http_fetch for:
- Simple API calls
- Static content
- Performance-critical requests
</browser_usage>
```

---

## Key Patterns to Follow

### 1. **Separation of Concerns**

- **Tool Definition**: LLM schema, validation, arg parsing
- **Protocol**: Operation constants, request structure
- **Executor**: Dispatch logic (switch statement)
- **Invoker**: Actual implementation, state management

### 2. **Naming Conventions**

- Tool structs: `*Tool` (e.g., `BrowserTool`, `ShellExecTool`)
- Invokers: `Builtin*Invoker` (e.g., `BuiltinShellInvoker`)
- Session managers: `*SessionManager` or `*Manager`
- Operations: `HostOp*` constants (e.g., `HostOpBrowser`)

### 3. **Error Handling**

Always return structured errors via `HostOpResponse`:

```go
return types.HostOpResponse{
    Op:    "browser.navigate",
    Ok:    false,
    Error: err.Error(),
}
```

### 4. **Session/State Management**

For stateful operations:
- Use sync.RWMutex for concurrent access
- Implement cleanup goroutines for stale state
- Store session/state managers in `HostOpExecutor` struct
- Provide `Shutdown()` methods for graceful cleanup

### 5. **Testing**

Add tests in `pkg/agent/host_ops_mock_test.go`:

```go
func TestBrowserOperation(t *testing.T) {
    manager, err := builtins.NewBrowserSessionManager(5 * time.Minute)
    require.NoError(t, err)
    defer manager.Shutdown()
    
    executor := &agent.HostOpExecutor{
        BrowserManager: manager,
        WorkspaceDir:   t.TempDir(),
    }
    
    // Test start
    startReq := types.HostOpRequest{
        Op:    types.HostOpBrowser,
        Input: json.RawMessage(`{"action": "start"}`),
    }
    resp := executor.Exec(context.Background(), startReq)
    require.True(t, resp.Ok)
}
```

---

## Existing Examples

Study these for reference:

### Simple Operation (No State)
- **fs_list, fs_read, fs_write** (`host_ops_mock.go` lines 68-160)
- Direct VFS operations, no external state

### Invoker Pattern (Stateful)
- **shell_exec** (`builtin_shell.go`)
  - `BuiltinShellInvoker` manages shell execution
  - Handles path translation, environment filtering
- **http_fetch** (`builtin_http.go`)
  - `BuiltinHTTPInvoker` manages HTTP requests
  - Handles redirects, chunked responses
- **trace** (`builtin_trace.go`)
  - `BuiltinTraceInvoker` manages event logging
  - Routes trace actions (events.latest, events.since)

### Multi-Action Operations
- **trace** has sub-actions: `events.latest`, `events.since`, `events.summary`
- **browser** has sub-actions: `start`, `navigate`, `click`, `type`, etc.

Pattern: Parse `action` field in params, switch on it.

---

## File Checklist

When adding a new host operation, you'll touch:

- [ ] `pkg/types/host_op_protocol.go` - Add constant + validation
- [ ] `pkg/agent/hosttools/<operation>.go` - Tool definition [NEW]
- [ ] `pkg/agent/host_ops_mock.go` - Add case to switch statement
- [ ] `pkg/tools/builtins/<operation>.go` - Invoker/state [NEW, if needed]
- [ ] `pkg/runtime/runtime.go` - Initialize & wire
- [ ] `pkg/agent/loop.go` - Update system prompt
- [ ] `pkg/agent/host_ops_mock_test.go` - Add tests [NEW]

---

## Common Pitfalls

### ❌ Don't: Create a separate executor file
**Wrong**: `pkg/runtime/browser_executor.go` with its own `Execute()` method.
**Right**: Add to `HostOpExecutor.Exec()` switch statement.

### ❌ Don't: Put invokers in pkg/runtime
**Wrong**: `pkg/runtime/browser_session.go`
**Right**: `pkg/tools/builtins/browser_session.go`

### ❌ Don't: Mix tool definition with execution logic
**Wrong**: `BrowserTool.Execute()` does actual browser operations.
**Right**: `BrowserTool.Execute()` just parses args, returns `HostOpRequest`. Execution happens in `HostOpExecutor.Exec()`.

### ❌ Don't: Forget cleanup
**Wrong**: Sessions/resources never cleaned up, causing memory leaks.
**Right**: Implement `Shutdown()`, cleanup goroutines, timeouts.

---

## Summary

The Agen8 host operation pattern is:

1. **Tool** (`pkg/agent/hosttools/`) defines the LLM interface
2. **Protocol** (`pkg/types/`) defines the operation contract
3. **Executor** (`pkg/agent/host_ops_mock.go`) dispatches via switch
4. **Invoker** (`pkg/tools/builtins/`) provides the implementation

This pattern keeps code organized, testable, and easy to extend. When adding new capabilities, follow this structure and study existing operations for reference.
