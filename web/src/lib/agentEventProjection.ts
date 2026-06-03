import type { AgentEvent } from './types'
import { parseSpaceListTranscript } from './spaceRegistryPayload'
import { resolveMcpServer, stripMcpNamespace } from '../components/conversation/mcp/mcpRegistry'

const SEMANTIC_TOOL_OPS = new Set([
  'task',
  'mission',
  'plan',
  'operator',
  'decision',
  'schedule',
  'graph_query',
  'space',
  'space_message',
  'list_spaces',
  'shell_exec',
  'bash',
  'code_compile',
  'read_file',
  'write_file',
  'edit_file',
  'file_change',
  'list_files',
  'search_files',
  'delete_file',
  'http',
  'browser',
  'metrics',
  'tool',
  'web_search',
  'tool_search',
  'image_generation',
])

function stripNamespacePrefix(value: string): string {
  let base = value
  // Strip namespace prefixes in the same patterns that MCP tool names
  // use: slash (agen8/decision), colon (agen8:decision), dot
  // (agen8.decision), double-underscore (mcp__agen8__decision).
  const slashIdx = base.lastIndexOf('/')
  if (slashIdx >= 0 && slashIdx < base.length - 1) base = base.slice(slashIdx + 1)
  const colonIdx = base.lastIndexOf(':')
  if (colonIdx >= 0 && colonIdx < base.length - 1) base = base.slice(colonIdx + 1)
  const dotIdx = base.lastIndexOf('.')
  if (dotIdx >= 0 && dotIdx < base.length - 1) base = base.slice(dotIdx + 1)
  const doubleUnderscoreIdx = base.lastIndexOf('__')
  if (doubleUnderscoreIdx >= 0 && doubleUnderscoreIdx < base.length - 2) base = base.slice(doubleUnderscoreIdx + 2)
  return base
}

function normalizeToolOp(op: string): string {
  const raw = String(op ?? '')
    .trim()
    .toLowerCase()
    .replaceAll('-', '_')
    .replaceAll(' ', '_')

  if (raw.startsWith('bash.') || raw.startsWith('shell_exec.') || raw.startsWith('shell.')) {
    return 'bash'
  }
  if (raw.startsWith('http.')) {
    return 'http'
  }
  if (raw === 'web.run' || raw.startsWith('web.run.')) {
    return 'web_search'
  }
  if (raw === 'image_gen' || raw.startsWith('image_gen.') || raw === 'imagegen' || raw.startsWith('imagegen.')) {
    return 'image_generation'
  }
  switch (raw) {
    case 'web_run':
    case 'web':
      return 'web_search'
    case 'toolsearch':
    case 'tool_search':
    case 'tool_search_tool':
    case 'tool_search.tool':
      return 'tool_search'
    case 'image_generation':
    case 'image_generation_call':
    case 'imagegen':
    case 'image_gen':
      return 'image_generation'
    case 'functions.exec_command':
    case 'exec_command':
      return 'bash'
    case 'functions.apply_patch':
      return 'edit_file'
  }

  const normalized = stripNamespacePrefix(raw)
  switch (normalized) {
    case 'decision_log':
      return 'decision'
    case 'operator_action':
      return 'operator'
    case 'graphquery':
      return 'graph_query'
    case 'message':
      return 'space_message'
    case 'shell':
    case 'shell_command':
    case 'shellcommand':
    case 'command_execution':
    case 'exec_command':
    case 'shell_exec':
    case 'bash':
      return 'bash'
    case 'write':
    case 'writefile':
    case 'create_file':
      return 'write_file'
    case 'edit':
    case 'editfile':
    case 'apply_patch':
    case 'multiedit':
    case 'multi_edit':
      return 'edit_file'
    case 'read':
    case 'readfile':
      return 'read_file'
    case 'glob':
    case 'grep':
      return 'search_files'
    case 'ls':
    case 'list_dir':
    case 'list_directory':
      return 'list_files'
    case 'read_file':
      return 'read_file'
    case 'write_file':
      return 'write_file'
    case 'edit_file':
      return 'edit_file'
    case 'file_change':
    case 'filechange':
      return 'file_change'
    case 'list_files':
      return 'list_files'
    case 'search_files':
      return 'search_files'
    case 'http_request':
      return 'http'
    case 'websearch':
    case 'web_search':
      return 'web_search'
    case 'imagegeneration':
    case 'image_generation':
    case 'image_generation_call':
    case 'imagegen':
    case 'image_gen':
      return 'image_generation'
    case 'toolsearch':
    case 'tool_search':
      return 'tool_search'
    case 'list_mcp_resources':
    case 'list_mcp_resource_templates':
      return 'tool'
    default:
      return normalized
  }
}

function inferToolOpFromName(toolName: string): string {
  const base = String(toolName ?? '').trim()
  if (!base) return ''
  return normalizeToolOp(base)
}

function actionFromInput(input: string): string {
  const trimmed = String(input ?? '').trim()
  if (!trimmed || !trimmed.startsWith('{')) return ''
  try {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>
    return String(parsed.action ?? '').trim().toLowerCase()
  } catch {
    return ''
  }
}

function parseInputObject(input: string): Record<string, unknown> | null {
  const trimmed = String(input ?? '').trim()
  if (!trimmed || !trimmed.startsWith('{')) return null
  try {
    const parsed = JSON.parse(trimmed) as Record<string, unknown>
    if (!parsed || Array.isArray(parsed)) return null
    return parsed
  } catch {
    return null
  }
}

function parseObjectCandidate(value: unknown): Record<string, unknown> | null {
  const trimmed = asString(value)
  if (!trimmed || !trimmed.startsWith('{')) return null
  try {
    const parsed = JSON.parse(trimmed) as unknown
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) return null
    return parsed as Record<string, unknown>
  } catch {
    return null
  }
}

function mergeSpaceResponsePayload(data: Record<string, string>, payload: Record<string, unknown>): void {
  for (const key of [
    'ok',
    'action',
    'broadcast',
    'correlationId',
    'destinationChannelId',
    'destinationMemberId',
    'destinationMemberLabel',
    'destinationSpaceId',
    'kind',
    'subject',
  ]) {
    const value = payload[key]
    if (value == null) continue
    const normalized = typeof value === 'string'
      ? value
      : typeof value === 'object'
        ? JSON.stringify(value)
        : String(value)
    if (asString(normalized)) data[key] = normalized
  }
}

function mergeMessageResponsePayload(data: Record<string, string>, payload: Record<string, unknown>): void {
  const deliveryChannelId = payload.channelId ?? payload.channelID ?? payload.channel_id
  if (deliveryChannelId != null) {
    const normalized = typeof deliveryChannelId === 'string'
      ? deliveryChannelId
      : typeof deliveryChannelId === 'object'
        ? JSON.stringify(deliveryChannelId)
        : String(deliveryChannelId)
    if (asString(normalized)) data.deliveryChannelId = normalized
  }
  for (const key of [
    'action',
    'correlationId',
    'destinationMemberId',
    'destinationMemberLabel',
    'kind',
    'messageId',
    'ok',
    'sourceMemberId',
    'sourceMemberLabel',
    'status',
    'subject',
  ]) {
    const value = payload[key]
    if (value == null) continue
    const normalized = typeof value === 'string'
      ? value
      : typeof value === 'object'
        ? JSON.stringify(value)
        : String(value)
    if (asString(normalized)) data[key] = normalized
  }
}

function inferAgen8WrappedTool(inputObj: Record<string, unknown> | null): { toolOp: string; action: string; input: Record<string, unknown> | null } | null {
  if (!inputObj) return null
  const server = asString(
    inputObj.server ??
    inputObj.server_name ??
    inputObj.source ??
    inputObj.source_id ??
    inputObj.mcp_server,
  ).toLowerCase()
  if (server !== 'agen8') return null
  const nestedToolName = asString(
    inputObj.tool ??
    inputObj.tool_name ??
    inputObj.toolName ??
    inputObj.name,
  )
  if (!nestedToolName || nestedToolName.toLowerCase() === 'tool') return null
  const toolOp = normalizeToolOp(nestedToolName)
  if (!toolOp || toolOp === 'tool') return null
  const nestedInputRaw = inputObj.arguments ?? inputObj.args ?? null
  const nestedInput = (nestedInputRaw && typeof nestedInputRaw === 'object' && !Array.isArray(nestedInputRaw))
    ? nestedInputRaw as Record<string, unknown>
    : null
  const action = asString(
    nestedInput?.action ??
    inputObj.action,
  ).toLowerCase()
  return { toolOp, action, input: nestedInput }
}

function asString(value: unknown): string {
  if (value == null) return ''
  return String(value).trim()
}

function inferBackgroundFromCommand(command: string): boolean {
  const trimmed = String(command ?? '').trim()
  if (!trimmed) return false
  if (/\s&$/.test(trimmed) || (trimmed.endsWith('&') && !trimmed.endsWith('&&'))) return true
  const lower = trimmed.toLowerCase()
  return lower.includes(' nohup ') || lower.includes(' disown')
}

function inferPathFromInput(inputObj: Record<string, unknown> | null): string {
  if (!inputObj) return ''
  return asString(
    inputObj.path ??
      inputObj.file_path ??
      inputObj.filePath ??
      inputObj.filename ??
      inputObj.target_file ??
      inputObj.targetPath,
  )
}

function toPatchPath(path: string): string {
  const trimmed = String(path ?? '').trim()
  if (!trimmed) return 'file'
  return trimmed.startsWith('/') ? trimmed.slice(1) : trimmed
}

function lineDelta(value: string): number {
  const trimmed = String(value ?? '').trim()
  if (!trimmed) return 0
  return trimmed.split(/\r?\n/).length
}

function synthesizeEditPatch(path: string, beforeText: string, afterText: string): string {
  const before = String(beforeText ?? '').replace(/\r\n/g, '\n').trim()
  const after = String(afterText ?? '').replace(/\r\n/g, '\n').trim()
  if (!before && !after) return ''
  const patchPath = toPatchPath(path)
  return [
    `--- a/${patchPath}`,
    `+++ b/${patchPath}`,
    '@@',
    ...(before ? [`-${before}`] : []),
    ...(after ? [`+${after}`] : []),
  ].join('\n')
}

function synthesizeWritePatch(path: string, content: string): string {
  const body = String(content ?? '').replace(/\r\n/g, '\n').trim()
  if (!body) return ''
  const patchPath = toPatchPath(path)
  return [
    '--- /dev/null',
    `+++ b/${patchPath}`,
    '@@',
    `+${body}`,
  ].join('\n')
}

function projectToolCall(event: AgentEvent): AgentEvent {
  const data = { ...(event.data ?? {}) }
  const op = normalizeToolOp(data.op || inferToolOpFromName(data.toolName ?? event.title ?? ''))
  const inputObj = parseInputObject(data.input ?? '')
  let resolvedOp = op
  let resolvedInputObj = inputObj
  if (resolvedOp === 'tool') {
    const wrapped = inferAgen8WrappedTool(inputObj)
    if (wrapped) {
      resolvedOp = wrapped.toolOp
      if (wrapped.input) {
        data.input = JSON.stringify(wrapped.input)
        resolvedInputObj = wrapped.input
      }
      if (wrapped.action) {
        data.action = wrapped.action
      }
      if (!asString(data.sourceId)) data.sourceId = 'agen8'
    }
  }
  if (resolvedOp) data.op = resolvedOp

  let action = String(data.action ?? '').trim().toLowerCase() || actionFromInput(data.input ?? '')
  if (resolvedOp === 'tool' && !action) {
    action = 'list'
  }

  if (resolvedOp === 'shell_exec' || resolvedOp === 'bash') {
    const inputCommand = asString(resolvedInputObj?.command ?? resolvedInputObj?.cmd ?? resolvedInputObj?.argv)
    if (!asString(data.command) && inputCommand) data.command = inputCommand
    if (!asString(data.argvPreview) && inputCommand) data.argvPreview = inputCommand

    const inputBackground = asString(resolvedInputObj?.background ?? resolvedInputObj?.isBackground ?? resolvedInputObj?.detached)
    if (!asString(data.background)) {
      if (inputBackground === 'true' || inputBackground === '1') {
        data.background = 'true'
      } else if (inferBackgroundFromCommand(data.command ?? data.argvPreview ?? '')) {
        data.background = 'true'
      }
    }
  }

  if (
    (
      resolvedOp === 'read_file' ||
      resolvedOp === 'list_files' ||
      resolvedOp === 'write_file' ||
      resolvedOp === 'edit_file' ||
      resolvedOp === 'file_change'
    ) && resolvedInputObj
  ) {
    const inferredPath = inferPathFromInput(resolvedInputObj)
    if (!asString(data.path) && inferredPath) data.path = inferredPath
  }

  if (resolvedOp === 'edit_file' && !asString(data.patchPreview) && resolvedInputObj) {
    const oldText = asString(resolvedInputObj.old_string ?? resolvedInputObj.oldString ?? resolvedInputObj.before ?? resolvedInputObj.old_text)
    const newText = asString(resolvedInputObj.new_string ?? resolvedInputObj.newString ?? resolvedInputObj.after ?? resolvedInputObj.new_text)
    const patchPreview = synthesizeEditPatch(asString(data.path), oldText, newText)
    if (patchPreview) {
      data.patchPreview = patchPreview
      if (!asString(data.writeMode)) data.writeMode = 'modified'
      if (!asString(data.linesAdded)) data.linesAdded = String(lineDelta(newText))
      if (!asString(data.linesRemoved)) data.linesRemoved = String(lineDelta(oldText))
      if (!asString(data.result)) data.result = patchPreview
      if (!asString(data.outputPreview)) data.outputPreview = patchPreview
    }
  }

  if (resolvedOp === 'write_file' && !asString(data.patchPreview) && resolvedInputObj) {
    const content = asString(resolvedInputObj.content ?? resolvedInputObj.text ?? resolvedInputObj.body ?? resolvedInputObj.value)
    const patchPreview = synthesizeWritePatch(asString(data.path), content)
    if (patchPreview) {
      data.patchPreview = patchPreview
      if (!asString(data.writeMode)) data.writeMode = 'created'
      if (!asString(data.linesAdded)) data.linesAdded = String(lineDelta(content))
      if (!asString(data.linesRemoved)) data.linesRemoved = '0'
      if (!asString(data.result)) data.result = patchPreview
      if (!asString(data.outputPreview)) data.outputPreview = patchPreview
    }
  }

  if (resolvedOp === 'space' && !asString(data.spacesJson)) {
    const rawOutput = asString(data.result)
      || asString(data.outputPreview)
      || asString(data.output)
      || asString(data.responseText)
      || asString(data.repr)
    const responsePayload = parseObjectCandidate(rawOutput)
    if (responsePayload) {
      mergeSpaceResponsePayload(data, responsePayload)
    }
    const spaces = parseSpaceListTranscript(rawOutput)
    if (spaces) {
      data.spacesJson = JSON.stringify({ action: 'list', count: spaces.length, empty: spaces.length === 0, spaces })
      if (!action) action = 'list'
    }
  }
  if (resolvedOp === 'space_message') {
    const rawOutput = asString(data.result)
      || asString(data.outputPreview)
      || asString(data.output)
      || asString(data.responseText)
      || asString(data.repr)
    const responsePayload = parseObjectCandidate(rawOutput)
    if (responsePayload) {
      mergeMessageResponsePayload(data, responsePayload)
    }
  }
  if (action) data.action = action

  // Spread input fields from the tool input JSON so semantic cards
  // can read domain-specific fields (title, goal, assignedRole, etc.)
  // that the backend only stores in the compacted `data.input` string.
  // Without this, cards render with empty input fields and fall back
  // to raw MCP tool paths for titles/labels.
  if (resolvedInputObj && resolvedOp && SEMANTIC_TOOL_OPS.has(resolvedOp)) {
    for (const [key, value] of Object.entries(resolvedInputObj)) {
      if (key === 'action') continue  // Already extracted above.
      if (key in data) continue        // Don't overwrite event metadata.
      if (value == null) continue
      data[key] = typeof value === 'string'
        ? value
        : typeof value === 'object'
          ? JSON.stringify(value)
          : String(value)
    }
  }

  let projectedKind = event.kind
  if (resolvedOp && SEMANTIC_TOOL_OPS.has(resolvedOp)) {
    projectedKind = resolvedOp
  }

  let projectedStatus = event.status
  let projectedCompletedAt = event.completedAt
  if (
    projectedKind === 'image_generation'
    && (asString(data.imageB64) || asString(data.imageUrl) || asString(data.url))
  ) {
    data.status = 'completed'
    projectedStatus = 'ok'
    projectedCompletedAt = projectedCompletedAt || event.startedAt
  }

  // MCP registry: project known server tools to mcp:<serverId> kind
  if (projectedKind === event.kind) {
    const mcpServer = resolveMcpServer({ ...event, data })
    if (mcpServer) {
      const baseTool = stripMcpNamespace(data.toolName ?? data.toolNameRaw ?? '')
      const mcpOp = mcpServer.resolveToolOp(baseTool)
      data.mcpServerId = mcpServer.id
      if (mcpOp) data.mcpOp = mcpOp
      projectedKind = `mcp:${mcpServer.id}`
    }
  }

  if (
    projectedKind === event.kind
    && data.op === event.data?.op
    && data.action === event.data?.action
    && data.spacesJson === event.data?.spacesJson
    && projectedStatus === event.status
    && projectedCompletedAt === event.completedAt
  ) {
    return event
  }
  return {
    ...event,
    kind: projectedKind,
    status: projectedStatus,
    completedAt: projectedCompletedAt,
    data,
  }
}

export function projectAgentEvent(event: AgentEvent): AgentEvent {
  if ((event.kind ?? '').toLowerCase() !== 'agent.tool.call') return event
  return projectToolCall(event)
}

export function projectAgentEvents(events: AgentEvent[]): AgentEvent[] {
  return events.map(projectAgentEvent)
}
