# Agen8 Protocol

The Agen8 App Server exposes a JSON-RPC 2.0 API over stdio. This protocol allows clients (such as the TUI or web interfaces) to manage threads, turns, and items, and to search artifacts.

## Overview

-   **Transport**: JSON-RPC 2.0 over standard input/output.
-   **Notifications**: The server sends notifications for asynchronous updates (e.g., `item.delta`, `turn.update`).
-   **Versioning**: All requests must include `"jsonrpc": "2.0"`.

## Methods

### `thread.create`
Creates a new conversation thread.

**Parameters:**
-   `title` (string, optional): The initial title of the thread.
-   `activeModel` (string, optional): The model to use for this thread.

**Result:**
-   `thread`: [Thread](#thread) object.

### `thread.get`
Retrieves an existing thread.

**Parameters:**
-   `threadId` (string): The ID of the thread to retrieve.

**Result:**
-   `thread`: [Thread](#thread) object.

### `turn.create`
Starts a new turn (user message -> agent response) in a thread.

**Parameters:**
-   `threadId` (string): The thread ID.
-   `input` ([UserMessageContent](#usermessagecontent)): The user's input message.

**Result:**
-   `turn`: [Turn](#turn) object (initially in `pending` status).

### `turn.cancel`
Cancels an in-progress turn.

**Parameters:**
-   `turnId` (string): The ID of the turn to cancel.

**Result:**
-   `turn`: [Turn](#turn) object (status updated to `canceled`).

### `item.list`
Lists items (messages, tool calls, etc.) within a turn.

**Parameters:**
-   `threadId` (string, optional): Filter by thread ID.
-   `turnId` (string, optional): Filter by turn ID.
-   `cursor` (string, optional): Opaque cursor for pagination.
-   `limit` (int, optional): Maximum number of items to return.

**Result:**
-   `items`: List of [Item](#item) objects.
-   `nextCursor` (string, optional): Cursor for the next page.

### `artifact.list`
Lists artifact groups (files, tasks, etc.) for browsing.

**Parameters:**
-   `threadId` (string): The thread context.
-   `teamId` (string, optional): Filter by team.
-   `dayBucket` (string, optional): Filter by day (YYYY-MM-DD).
-   `role` (string, optional): Filter by role.
-   `taskKind` (string, optional): Filter by task kind.
-   `taskId` (string, optional): Filter by task ID.
-   `limit` (int, optional): Max items to return.

**Result:**
-   `nodes`: List of [ArtifactNode](#artifactnode) objects.

### `artifact.search`
Searches implementation artifacts.

**Parameters:**
-   `threadId` (string): The thread context.
-   `query` (string): Search query string.
-   `scopeKey` (string, optional): Scope to restrict search (e.g., `day:2024-01-01`).
-   `limit` (int, optional): Max results.

**Result:**
-   `nodes`: List of [ArtifactNode](#artifactnode) objects matching the query.
-   `matchCount` (int): Total number of matches.

### `artifact.get`
Retrieves the content of a specific artifact.

**Parameters:**
-   `threadId` (string): The thread context.
-   `artifactId` (string, optional): The artifact ID.
-   `vpath` (string, optional): Virtual path of the artifact.
-   `maxBytes` (int, optional): ongoing Max bytes to read.

**Result:**
-   `artifact`: [ArtifactNode](#artifactnode) metadata.
-   `content` (string): The content of the artifact (or a chunk).
-   `truncated` (bool): Whether the content was truncated.

## Types

### Thread
Represents a conversation session.
```json
{
  "id": "string",
  "title": "string",
  "createdAt": "timestamp",
  "activeModel": "string",
  "activeRunId": "string"
}
```

### Turn
Represents a single exchange cycle.
```json
{
  "id": "string",
  "threadId": "string",
  "status": "pending|in_progress|completed|failed|canceled",
  "createdAt": "timestamp",
  "error": { "code": "int", "message": "string" }
}
```

### Item
An atomic unit of work (message, tool call, etc.).
```json
{
  "id": "string",
  "turnId": "string",
  "type": "user_message|agent_message|tool_execution|reasoning",
  "status": "started|streaming|completed|failed",
  "content": { ... }
}
```

#### UserMessageContent
payload for `user_message` type.
```json
{
  "text": "string",
  "attachments": [ { "uri": "string" } ]
}
```

#### AgentMessageContent
payload for `agent_message` type.
```json
{
  "text": "string",
  "isPartial": boolean
}
```

#### ToolExecutionContent
payload for `tool_execution` type.
```json
{
  "toolName": "string",
  "input": { ... },
  "output": { ... },
  "ok": boolean
}
```

#### ReasoningContent
payload for `reasoning` type.
```json
{
  "summary": "string",
  "step": int
}
```

### ArtifactNode
Represents a file or grouping node in the artifact index.
```json
{
  "nodeKey": "string",
  "parentKey": "string",
  "kind": "day|role|task|file",
  "label": "string",
  "artifactId": "string",
  "vpath": "string",
  "diskPath": "string"
}
```
