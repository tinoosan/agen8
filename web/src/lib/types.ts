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

export interface Item {
  id: string
  type: string
  role?: string
  content?: string
  delta?: string
  status?: string
  createdAt?: string
}

export interface Task {
  id: string
  threadId?: string
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
