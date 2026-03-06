// Mirrors the Go protocol types used by the web UI.

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
  assignedTo?: string
  assignedToType?: string
  claimedByAgentId?: string
  taskKind?: string
  goal: string
  status: string
  summary?: string
  error?: string
  artifacts?: string[]
  costUSD?: number
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

export interface ActivityEvent {
  seq?: number
  runId?: string
  type?: string
  role?: string
  summary?: string
  detail?: string
  createdAt?: string
  data?: unknown
}

export interface Artifact {
  artifactId?: string
  vpath?: string
  role?: string
  createdAt?: string
  sizeBytes?: number
}
