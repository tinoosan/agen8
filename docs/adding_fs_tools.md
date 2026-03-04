# Adding `fs_*` Tools

Use this checklist when introducing a new filesystem tool (for example `fs_stat`, `fs_head`, `fs_move`).

## Key rule

- Any tool whose function name starts with `fs_` is always enabled by default, even when a profile sets `allowed_tools`.
- You do **not** need to add new `fs_*` tools to profile YAMLs for access.
- This behavior is enforced in `internal/app/tool_policy.go` via `isAlwaysEnabledTool`.

## Implementation checklist

1. Add the host op constant in `pkg/types/host_op_protocol.go`.
2. Add request validation in `HostOpRequest.Validate()` for the new op.
3. Add response fields to `types.HostOpResponse` only when needed (keep minimal for token efficiency).
4. Implement the model-facing tool in `pkg/agent/hosttools/<tool>.go`.
5. Register the tool in `pkg/agent/constructors.go` (`defaultHostTools()`).
6. Implement execution in `pkg/agent/host_ops_mock.go` and wire into `mockHostOperations`.
7. Add runtime op wiring in `pkg/runtime/host_operation.go`:
   - register in `defaultHostOperations()`
   - request/response formatting hooks
   - event enrichment fields for activity/telemetry
8. If the op needs VFS behavior, add methods/tests in `pkg/vfs/vfs.go`.

## UX and observability checklist

1. Request title formatting: `internal/opmeta/request_title.go`.
2. Operation catalog category/title sharing: `pkg/opcatalog/catalog.go`.
3. Response text formatting: `pkg/opformat/format.go`.
4. Feed/coordinator verbs:
   - `internal/tui/kit/feed.go`
   - `internal/tui/coordinator/data.go`
5. Activity details renderer: `internal/tui/activity_renderers.go`.
6. Suppression parity (if relevant to existing fs noise rules):
   - `internal/opmeta/request_title.go`
   - `pkg/protocol/mapper.go`
   - `internal/tui/render.go`

## Prompt and policy checklist

1. Ensure tool appears in prompt defaults (`pkg/prompts/base.go`) when it should be model-visible.
2. Add guidance for cost-efficient usage when applicable (for example, metadata-first before content reads).
3. Do **not** require profile `allowed_tools` edits for `fs_*` tool access.

## Tests to add/update

- `pkg/agent/hosttools/*_test.go` (tool schema + request mapping)
- `pkg/types/host_op_protocol_test.go` (validation)
- `pkg/vfs/vfs_test.go` (if VFS logic changed)
- `pkg/agent/host_ops_mock_test.go` (executor behavior)
- `pkg/runtime/executor_test.go` (event enrichment + formatted text)
- `internal/opmeta/request_title_test.go`
- `pkg/opcatalog/catalog_test.go`
- `pkg/opformat/format_test.go`
- `internal/tui/format_test.go`
- `internal/tui/activity_detail_test.go` (if custom renderer output)
- `internal/app/tool_policy_test.go` (always-enabled behavior for `fs_*`)
