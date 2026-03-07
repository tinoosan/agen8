// Mirrors the Go protocol types used by the web UI.

export interface ProjectRegistrySummary {
  projectRoot: string
  projectId: string
  manifestPath?: string
  enabled: boolean
  createdAt?: string
  updatedAt?: string
  metadata?: Record<string, unknown>
}

export interface ProjectTeamSummary {
  teamId: string
  projectId?: string
  projectRoot?: string
  profileId?: string
  primarySessionId?: string
  coordinatorRunId?: string
  status?: string
  createdAt?: string
  updatedAt?: string
  manifestPresent?: boolean
  desiredEnabled?: boolean
  reconcileStatus?: 'converged' | 'drifting' | 'reconciling' | 'failed' | string
  managedBy?: string
}

export interface ProjectReconcileAction {
  action?: string
  profile?: string
  teamId?: string
  reason?: string
  managed?: boolean
}

export interface ProjectReconcileNotification {
  projectRoot?: string
  projectId?: string
  converged?: boolean
  status?: 'converged' | 'drifting' | 'reconciling' | 'failed' | string
  actions?: ProjectReconcileAction[]
  teamIds?: string[]
  tickAt?: string
  error?: string
}

export interface TeamRoleStatus {
  role: string
  info: string
}

export interface TeamGetStatusResult {
  pending: number
  active: number
  done: number
  roles: TeamRoleStatus[]
  runIds: string[]
  roleByRunId: Record<string, string>
  totalTokensIn: number
  totalTokensOut: number
  totalTokens: number
  totalCostUSD: number
  pricingKnown: boolean
}

export interface TeamManifestRole {
  roleName: string
  runId: string
  sessionId: string
}

export interface TeamManifestModelChange {
  requestedModel?: string
  status?: string
  requestedAt?: string
  appliedAt?: string
  reason?: string
  error?: string
}

export interface TeamGetManifestResult {
  teamId: string
  profileId: string
  teamModel?: string
  modelChange?: TeamManifestModelChange
  coordinatorRole: string
  reviewerRole?: string
  coordinatorRunId: string
  coordinatorThreadId?: string
  roles: TeamManifestRole[]
  desiredReplicasByRole?: Record<string, number>
  createdAt: string
}

// ---- Protocol Item types (matches pkg/protocol/item.go) ----

export type ItemType = 'user_message' | 'agent_message' | 'tool_execution' | 'reasoning'
export type ItemStatus = 'started' | 'streaming' | 'completed' | 'failed' | 'canceled'

export interface Item {
  id: string
  turnId: string
  runId?: string
  type: ItemType
  status: ItemStatus
  createdAt?: string
  content?: unknown // JSON: UserMessageContent | AgentMessageContent | ToolExecutionContent | ReasoningContent
  error?: { code: number; message: string }
}

export interface UserMessageContent {
  text: string
  attachments?: { id?: string; name?: string; uri?: string }[]
}

export interface AgentMessageContent {
  text: string
  isPartial?: boolean
  artifacts?: { id?: string; name?: string; uri?: string }[]
}

export interface ToolExecutionContent {
  toolName: string
  input?: unknown
  output?: unknown
  ok?: boolean
}

export interface ReasoningContent {
  summary?: string
  step?: number
}

// Notification param types (matches pkg/protocol/item.go)
export interface ItemDeltaParams {
  itemId: string
  delta: {
    textDelta?: string
    reasoningDelta?: string
  }
}

export interface ItemNotificationParams {
  item: Item
}

/** Extract displayable text from an Item's typed content. */
export function getItemText(item: Item): string {
  if (!item.content) return ''
  const c = item.content as Record<string, unknown>
  if (typeof c.text === 'string') return c.text
  if (typeof c.summary === 'string') return c.summary
  if (typeof c.toolName === 'string') {
    const out = c.output != null ? String(c.output).slice(0, 200) : ''
    return `${c.toolName}${out ? ': ' + out : ''}`
  }
  return ''
}

export interface Task {
  id: string
  threadId?: string
  sourceTeamId?: string
  destinationTeamId?: string
  teamId?: string
  runId?: string
  assignedRole?: string
  roleSnapshot?: string
  assignedTo?: string
  assignedToType?: string
  claimedByAgentId?: string
  createdBy?: string
  taskKind?: string
  goal: string
  status: string
  summary?: string
  error?: string
  artifacts?: string[]
  costUSD?: number
  totalTokens?: number
  metadata?: Record<string, unknown>
  createdAt: string
  completedAt?: string
}

export interface MailMessage {
  messageId: string
  threadId?: string
  runId?: string
  sourceTeamId?: string
  destinationTeamId?: string
  teamId?: string
  channel: string
  kind: string
  status: string
  subject?: string
  summary?: string
  bodyPreview?: string
  error?: string
  taskId?: string
  taskStatus?: string
  readOnly?: boolean
  canClaim?: boolean
  canComplete?: boolean
  createdAt: string
  updatedAt: string
  processedAt?: string
  task?: Task
}

export type ActivityStatus = 'pending' | 'ok' | 'error'

export interface ActivityEvent {
  id: string
  kind: string
  title: string
  status: ActivityStatus
  startedAt: string
  finishedAt?: string
  duration?: number
  from?: string
  to?: string
  path?: string
  maxBytes?: string
  textPreview?: string
  textTruncated?: boolean
  textRedacted?: boolean
  textIsJSON?: boolean
  textBytes?: string
  ok?: string
  error?: string
  outputPreview?: string
  bytesLen?: string
  truncated?: boolean
  data?: Record<string, string>
}

export interface ArtifactNode {
  nodeKey: string
  parentKey?: string
  kind: 'day' | 'role' | 'stream' | 'task' | 'file'
  label: string
  dayBucket?: string
  role?: string
  taskKind?: string
  taskId?: string
  status?: string
  artifactId?: string
  displayName?: string
  vpath?: string
  diskPath?: string
  isSummary?: boolean
  producedAt?: string
}

export interface ArtifactGetResult {
  artifact: ArtifactNode
  content: string
  truncated: boolean
  bytesRead: number
}

export interface RuntimeRunState {
  runId: string
  model: string
  status: string
  effectiveStatus: string
  workerPresent: boolean
  runTotalTokens: number
  runTotalCostUSD: number
}

export interface RuntimeGetSessionStateResult {
  sessionId: string
  runs: RuntimeRunState[]
}

// ---- Agent / Dashboard types ----

export interface AgentInfo {
  agentId: string
  runId: string
  role: string
  status: string
  profile?: string
  parentRunId?: string
  spawnIndex?: number
  createdAt?: string
}

export interface AgentListResult {
  agents: AgentInfo[]
}

export interface SessionTotals {
  totalTokensIn: number
  totalTokensOut: number
  totalTokens: number
  totalCostUSD: number
}

export interface EventRecord {
  eventId: string
  runId: string
  timestamp: string
  type: string
  message: string
  data?: Record<string, string>
  origin?: string
}

// ---- Plan types ----

export interface PlanGetResult {
  checklist: string
  checklistErr?: string
  details: string
  detailsErr?: string
  sourceRuns: string[]
}

// ---- Model types ----

export interface ModelEntry {
  id: string
  provider: string
  inputPerM?: number
  outputPerM?: number
  isReasoning?: boolean
}

export interface ModelListResult {
  providers: { name: string; count: number }[]
  models: ModelEntry[]
}

// ---- Reasoning types ----

export type ReasoningEffort = 'none' | 'minimal' | 'low' | 'medium' | 'high' | 'xhigh'
export type ReasoningSummary = 'off' | 'auto' | 'concise' | 'detailed'
