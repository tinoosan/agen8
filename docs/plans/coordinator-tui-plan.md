# Coordinator TUI – Plan & UX Definition

## 1. Current Coordinator UI

The coordinator TUI is a **standalone, full-screen Bubble Tea chat-style interface** for interacting with a session coordinator. It is launched via `agen8 coordinator` or `agen8 attach <session-id>`.

### Layout (ASCII sketch)

```
┌─────────────────────────────────────────────────────────────────────────────┐
│ agen8 coordinator    session: abc123  ● connected  mode: standalone  role: X │  ← Header
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│   You                                              2m ago                   │
│   Please implement the authentication module                                │
│   ✓ queued                                                                  │
│                                                                             │
│   ● architect operations                                      30s ago       │
│     Read  src/auth/handler.go                                            │
│     └ Done                                                                  │
│     Bash  go test ./...                                               │
│     └ running ⠹                                                             │
│                                                                             │
│   ◆ system                                                    1m ago        │
│   Session paused                                                            │
│                                                                             │
├─────────────────────────────────────────────────────────────────────────────┤
│ ❯ type a goal or /command...                                    [feedback]  │  ← Input bar
├─────────────────────────────────────────────────────────────────────────────┤
│ end                    enter send  /pause /resume /stop /help  pgup/pgdn …   │  ← Footer
└─────────────────────────────────────────────────────────────────────────────┘
```

### Components

| Component     | Description                                                                                                                                         |
| ------------- | --------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Header**    | Title "agen8 coordinator", session ID (truncated), connection status pill, mode tag, role tag (wide only), last error                               |
| **Feed**      | Scrollable conversation-style feed with turns: **You** (user goals), **agent** (operations grouped by role), **system** (pause/resume/stop notices) |
| **Input bar** | Single-line text input with `❯` prompt, placeholder "type a goal or /command...", optional feedback (queued, error, etc.)                           |
| **Footer**    | Scroll position (end / N%), key hints (enter, /pause, /resume, /stop, /help, pgup/pgdn, home/end, ctrl+c)                                           |

### Data & behavior

- **Data source**: `activity.list` RPC (polled every 2s) + `task.create` for goals, `session.pause/resume/stop` for actions
- **Scope**: Single session, resolved via `rpcscope` (sessionID, teamID, runID, coordinatorRole)
- **Live follow**: Feed auto-scrolls to bottom; scroll up disables follow; pgup/pgdn, home/end, mouse wheel for navigation
- **Markdown**: User and agent text blocks rendered with Glamour

### Commands (TUI)

| Command   | Action                                                      |
| --------- | ----------------------------------------------------------- |
| `/pause`  | Pause session                                               |
| `/resume` | Resume session                                              |
| `/stop`   | Stop session                                                |
| `/help`   | Show feedback: "commands: /pause /resume /stop /help /quit" |
| `/quit`   | Exit TUI                                                    |

### Coordinator shell (readline) – not TUI

The `coordinator_shell.go` readline loop has **additional** commands not in the TUI:

| Command                | TUI | Shell                          |
| ---------------------- | --- | ------------------------------ |
| `/new`                 | ❌  | ✅ Start new session flow      |
| `/attach <session-id>` | ❌  | ✅ Attach to different session |
| `/reviewer`            | ❌  | ✅ Show coordinator role       |
| `/reconnect`           | ❌  | ✅ Reconnect context           |

---

## 2. Coordinator Features (What It Does)

| Feature           | Description                               | Backing RPC                    |
| ----------------- | ----------------------------------------- | ------------------------------ |
| Submit goals      | User types a goal; queued as task         | `task.create`                  |
| Pause/Resume/Stop | Session lifecycle control                 | `session.pause/resume/stop`    |
| Activity feed     | View coordinator activity (ops, messages) | `activity.list`                |
| Session scope     | sessionID, teamID, runID, coordinatorRole | `rpcscope`, `team.getManifest` |
| Connection status | Connected / disconnected                  | RPC health                     |

---

## 3. Monitor Features (For Comparison)

The monitor is a **multi-panel dashboard** with compact and dashboard layouts.

### Monitor panels

| Panel                          | Purpose                                                        |
| ------------------------------ | -------------------------------------------------------------- |
| **Agent Output**               | Stream of user, thought, tool_call, tool_result, error, system |
| **Activity**                   | Activity feed list + detail view                               |
| **Plan**                       | Checklist + plan details                                       |
| **Current Task / Role Status** | Active task or team role status                                |
| **Inbox**                      | Pending tasks                                                  |
| **Outbox**                     | Completed/sent items                                           |
| **Thoughts**                   | Reasoning/thinking stream                                      |
| **Subagents**                  | Child runs (team mode)                                         |
| **Memory**                     | Memory search                                                  |
| **Composer**                   | Input + command palette                                        |

### Monitor commands (subset)

| Command                        | Coordinator TUI | Monitor   |
| ------------------------------ | --------------- | --------- |
| `/new`                         | ❌              | ✅        |
| `/reconnect`                   | ❌              | ✅        |
| `/sessions`                    | ❌              | ✅        |
| `/agents`                      | ❌              | ✅        |
| `/team`                        | ❌              | ✅        |
| `/pause` / `/resume` / `/stop` | ✅              | ✅        |
| `/model`                       | ❌              | ✅        |
| `/copy`                        | ❌              | ✅        |
| `/editor`                      | ❌              | ✅        |
| `/help`                        | ✅ (minimal)    | ✅ (full) |
| `/quit`                        | ✅              | ✅        |

### Monitor layout modes

- **Compact**: Tab bar (Output | Activity | Plan | Outbox) + single content area + composer
- **Dashboard**: Two columns – left: Agent Output; right: tabbed side (Activity | Plan | Tasks | Thoughts | Subagents)

---

## 4. Feature Parity: Coordinator vs Monitor

| Capability             | Coordinator TUI           | Monitor                              |
| ---------------------- | ------------------------- | ------------------------------------ |
| Submit goals/tasks     | ✅ (single input)         | ✅ (composer)                        |
| View activity feed     | ✅ (conversation-style)   | ✅ (list + detail)                   |
| Pause/Resume/Stop      | ✅                        | ✅                                   |
| Session/run switching  | ❌                        | ✅ (`/sessions`, `/agents`, `/team`) |
| New session            | ❌                        | ✅ (`/new`)                          |
| Reconnect              | ❌                        | ✅                                   |
| Plan view              | ❌                        | ✅                                   |
| Inbox/Outbox           | ❌                        | ✅                                   |
| Thoughts/reasoning     | ❌                        | ✅                                   |
| Subagents              | ❌                        | ✅                                   |
| Model/reasoning config | ❌                        | ✅                                   |
| Copy transcript        | ❌                        | ✅                                   |
| Artifact viewer        | ❌                        | ✅                                   |
| Help modal             | ❌ (inline feedback only) | ✅                                   |
| Command palette        | ❌                        | ✅                                   |

---

## 5. UX/UI Definition – Questions & Visualizations

To define the exact UX/UI for the coordinator TUI, we need your input on the following.

### A. Scope & role

1. **Primary user**: Is the coordinator TUI for users who _only_ want to talk to the coordinator (minimal), or should it grow toward monitor-like power (session switching, plan, inbox, etc.)?

2. **Session attachment**: Should the coordinator TUI support `/attach <session-id>` and `/new` (like the shell), or is attaching always done from the CLI before launching?

### B. Layout

3. **Single-column vs multi-panel**: Keep the current single-column chat layout, or add a side panel (e.g. Activity | Plan | Tasks) similar to the monitor’s right column?

4. **Compact vs dashboard**: Should the coordinator have a compact mode (small terminals) and a wider dashboard mode, like the monitor?

### C. Content & panels

5. **Plan**: Do you want a Plan panel (checklist + details) in the coordinator? If yes, as a tab or always visible?

6. **Inbox/Outbox**: Should the coordinator show Inbox (pending tasks) and Outbox? If yes, how (tabs, collapsible, etc.)?

7. **Thoughts**: Should the coordinator show a Thoughts/reasoning stream? This could add cognitive load for a “light” coordinator view.

8. **Subagents**: In team mode, should the coordinator show Subagents (child runs) and allow switching focus?

### D. Commands & input

9. **Command set**: Which monitor commands should the coordinator support? Options:
   - Minimal: `/pause`, `/resume`, `/stop`, `/help`, `/quit` (current)
   - Extended: add `/new`, `/attach`, `/reconnect`, `/sessions`, `/team`
   - Full parity: all monitor commands

10. **Command palette**: Should the coordinator have a command palette (e.g. Ctrl+P) like the monitor, or stay with slash commands only?

11. **Help**: Full help modal (like monitor) or keep the current inline feedback?

### E. Visual style

12. **Consistency**: Should the coordinator reuse the monitor’s styles (colors, borders, panel chrome) for consistency, or keep its own lighter chat aesthetic?

13. **Header**: Keep the current header (session, connection, mode, role) or align with the monitor’s header (e.g. session picker, run status)?

---

## 6. Decisions (Feb 2025)

| Question         | Decision                                                                                                                   |
| ---------------- | -------------------------------------------------------------------------------------------------------------------------- |
| **Scope**        | Minimal – like Claude Code / Codex CLI. Chat-only, no extra panels.                                                        |
| **Layout**       | Single-column chat. No side panels.                                                                                        |
| **Commands**     | Extended: add `/new`, `/attach`, `/reconnect` to current set.                                                              |
| **Plan**         | Inline in transcript, only when updated. When the plan checklist tool writes, render it in the markdown (Codex CLI style). |
| **Inbox/Outbox** | Not in coordinator.                                                                                                        |
| **Visual style** | Keep lighter chat look. Refine component by component.                                                                     |

### Reference: Codex CLI / Claude Code

- Chat-first, minimal chrome
- Plan appears inline in the stream when the plan tool updates
- No separate plan panel – plan content is part of the conversation/transcript

### Tool calls – target rendering

**Format:**

```
│   ● architect                                    30s ago       │
│     Read  src/auth/handler.go                                    │
│     └ Done                                                       │
│     Bash  go test ./...                                          │
│     └ running ⠹                                                  │
│     Plan updated                                                 │
│     └ Done                                                       │
│     └  - [x] Set up project structure                            │
│     └  - [x] Add auth module                                     │
│     └  - [ ] Implement login flow                                │
│     └  - [ ] Add tests                                           │
```

- **Header**: `● role` + timestamp (no "operations")
- **Per entry**: `  Verb  arg preview` — human-friendly verb + args from Title/path
- **Status**: `  └ Done` or `  └ running ⠹` or `  └ pending …`
- **Plan**: For plan_checklist_write, render checklist items as `└  - [x] item` lines

**Kind → Verb mapping (all tools):**

| Kind                 | Verb                                                |
| -------------------- | --------------------------------------------------- |
| fs_read              | Read                                                |
| fs_list              | List                                                |
| fs_write             | Write                                               |
| fs_append            | Append                                              |
| fs_edit              | Edit                                                |
| fs_patch             | Patch                                               |
| fs_search            | Search                                              |
| shell_exec           | Bash                                                |
| http_fetch           | Fetch                                               |
| browser              | Browse                                              |
| code_exec            | Python                                              |
| email                | Email                                               |
| agent_spawn          | Spawn                                               |
| task_create          | Dispatch task (coordinator) / Create task           |
| task_review          | Review task                                         |
| trace_run            | Trace                                               |
| workdir.changed      | Workdir                                             |
| llm.web.search       | Web search                                          |
| plan_checklist_write | Plan updated                                        |
| tool_call / custom   | Tool name as verb (e.g. from Data["tool"] or Title) |

**Full example – all tools in one agent block:**

```
│   ● architect                                    30s ago       │
│     Read  src/auth/handler.go                                    │
│     └ Done                                                       │
│     List  /workspace/src                                         │
│     └ Done                                                       │
│     Write  /workspace/main.go                                    │
│     └ Done                                                       │
│     Append  /workspace/log.txt                                   │
│     └ Done                                                       │
│     Edit  src/utils.go                                           │
│     └ Done                                                       │
│     Patch  src/config.go                                         │
│     └ Done                                                       │
│     Search  /memory  "auth patterns"                             │
│     └ Done                                                       │
│     Bash  go test ./...                                          │
│     └ running ⠹                                                  │
│     Fetch  GET [https://api.example.com/data]                     │
│     └ Done                                                       │
│     Browse  navigate https://example.com                          │
│     └ Done                                                       │
│     Python  Run python code                                      │
│     └ Done                                                       │
│     Email  team@example.com: Daily report                        │
│     └ Done                                                       │
│     Spawn  Spawn child agent: compute checksum                    │
│     └ Done                                                       │
│     Dispatch task  Add unit tests for auth module → cto           │
│     └ Done                                                       │
│     Review task  approve callback-task-1                         │
│     └ Done                                                       │
│     Trace  trace.events.latest run-123                            │
│     └ Done                                                       │
│     Workdir  /project changed                                     │
│     └ Done                                                       │
│     Web search  query arg preview                                │
│     └ Done                                                       │
│     Plan updated                                                 │
│     └ Done                                                       │
│     └  - [x] Set up project structure                            │
│     └  - [x] Add auth module                                     │
│     └  - [ ] Implement login flow                                │
│     └  - [ ] Add tests                                           │
│     my_custom_tool  arg preview                                  │
│     └ Done                                                       │
```

---

### code_exec grouping – child tools under Python block

When `code_exec` runs, it executes Python that can call multiple bridge tools (fs_read, fs_write, shell_exec, etc.). Those child tools should be grouped under the "Python Run python code" block.

**Data today:** Activities are flat (same runID, ordered by seq). `code_exec` response has `toolCallCount` and `runtimeMs`. No explicit parent link for nested tools.

**Ideas:**

1. **Temporal grouping (client-side)**  
   Group activities by time window: any activity that starts after `code_exec` request and before `code_exec` response (same runID) is treated as a child.
   - Pros: No backend changes, works with current data.
   - Cons: Brittle if ordering is wrong; other ops in the same window could be mis-grouped.

2. **Nested block with `└` lines (like plan)**  
   Render child tools as continuation lines under the Python block:

   ```
   │     Python  Run python code                                    │
   │     └ running ⠹                                                │
   │     └  Read  src/auth/handler.go                               │
   │     └  Done                                                    │
   │     └  Write  /workspace/output.json                          │
   │     └  Done                                                    │
   │     └  Done  3 tool calls, 120ms                              │
   ```

   - Pros: Matches plan style, clear hierarchy.
   - Cons: Needs reliable parent–child relationship (temporal or backend).

3. **Collapsed by default, expand on focus**  
   Show Python with a summary; expand to show child tools when selected:

   ```
   │     Python  Run python code  (3 tools)                         │
   │     └ Done  120ms                                              │
   ```

   Expanded:

   ```
   │     Python  Run python code                                    │
   │     ├ Read  src/auth/handler.go                                │
   │     │ └ Done                                                   │
   │     ├ Write  /workspace/output.json                            │
   │     │ └ Done                                                   │
   │     └ Done  120ms                                              │
   ```

   - Pros: Keeps feed compact.
   - Cons: Coordinator is minimal; expand/collapse adds UI complexity.

4. **Backend: add `parentOpId`**  
   When code_exec invokes a bridge tool, set `parentOpId` in the event to the code_exec `opId`.
   - Pros: Explicit parent–child link, robust grouping.
   - Cons: Requires executor/event changes.

5. **Summary-only (no child list)**  
   Show only the Python block with `toolCallCount` and `runtimeMs`:

   ```
   │     Python  Run python code                                    │
   │     └ Done  3 tools, 120ms                                     │
   ```

   - Pros: Simple, no grouping logic.
   - Cons: Child tools are hidden.

**Decision:** Use **5 (summary-only)**. The coordinator is minimal and there is a separate `activity` command for detailed inspection.

---

### Thinking progress – avoid "hanging" appearance

When the model is thinking (reasoning tokens, LLM call), show progress so it doesn't look like the app is hanging. No need to stream thoughts or show them all the time; can be collapsed. But the user should know when the model is thinking.

**Data options:**

1. **Events polling** – Coordinator already has `runID`. Poll `events.listPaginated` with `AfterSeq` to get new events. Infer status from event types:
   - **"Thinking…"** – only when we see reasoning tokens (e.g. `llm.usage.total` with `reasoning` > 0, or reasoning stream events)
   - **"Processing…"** – LLM call in progress but no reasoning (`task.start`, `agent.step` without reasoning)
   - **"🔧 {tool} …"** – `agent.op.request`
   - **"Processing…"** – `agent.op.response` (agent will call LLM again)
   - **"✓ Done"** – `task.done`

2. **Heuristic from activity** – If last activity is `pending`/`running`, show "Working…". If run is `running` and last activity finished >N seconds ago, show "Thinking…". Simpler but less accurate.

3. **Run status only** – If run is `running`, show generic "Working…" with spinner. No distinction between thinking vs tool.

**Display options:**

| Option                      | Description                                                                                              |
| --------------------------- | -------------------------------------------------------------------------------------------------------- |
| **Header pill**             | Small "● Thinking… ⠹" or "● Processing…" in header when active                                           |
| **Footer status**           | Replace or augment scroll hint with "Thinking…" when active                                              |
| **Collapsed block in feed** | Minimal agent block at bottom: `● architect  Thinking… ⠹` – collapsed by default, no expand for thoughts |
| **Input bar hint**          | "Model is thinking…" next to input when idle/focused                                                     |

**Recommendation:** Header pill. Reserve **"Thinking…"** for reasoning tokens only; use **"Processing…"** for regular LLM calls. Use events polling for accurate status; fall back to heuristic if events RPC unavailable. No thoughts panel; keep coordinator minimal.

---

### Coordinator-specific groupings – task dispatch & review

In the coordinator context, make it clear when:

- **Task dispatch** – coordinator is dispatching tasks (`task_create`)
- **Task review** – coordinator or reviewer is reviewing tasks (`task_review`)

Use coordinator-specific verbs in the kind→verb mapping:

- `task_create` → **Dispatch task** (coordinator delegating to a role)
- `task_review` → **Review task** (coordinator/reviewer approving, retrying, or escalating)

Flat feed; rely on clear verb + arg preview. No extra visual grouping for now.

---

### Plan rendering format

When the plan checklist tool updates, render it as part of the agent block:

```
│   ● architect                                       30s ago       │
│     Plan updated                                                 │
│     └ Done                                                       │
│     └  - [x] Set up project structure                             │
│     └  - [x] Add auth module                                      │
│     └  - [ ] Implement login flow                                 │
│     └  - [ ] Add tests                                            │
```

- Agent role + timestamp in header
- "Plan updated" as the operation summary
- "└ Done" for status
- Checklist items as indented `└` lines under the status

---

## 7. Implementation Roadmap

1. **Extended commands**: Add `/new`, `/attach`, `/reconnect` to coordinator TUI.
2. **Inline plan**: When activity/events include plan checklist updates, render plan markdown inline in the feed.
3. **Visual refinements**: Iterate component by component (header, feed blocks, input, footer).

---

_Document created for the coordinator TUI feature branch. Decisions captured Feb 2025._
