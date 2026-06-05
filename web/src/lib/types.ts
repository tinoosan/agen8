// Mirrors the Go protocol types used by the web UI.

export interface Project {
  id: string;
  locationId: string;
  root: string;
  title?: string;
  status: "open" | "archived" | string;
  createdAt?: string;
  updatedAt?: string;
}

export interface LocationAddress {
  host?: string;
  port?: number;
  username?: string;
}

export interface LocationCapability {
  name: string;
  status: "passed" | "failed" | "unknown" | string;
}

export interface LocationProbe {
  status?: "passed" | "failed" | "unknown" | string;
  failureCode?: string;
  message?: string;
  probedAt?: string;
}

export interface ExecutionLocation {
  id: string;
  kind: "local" | "ssh" | "managed" | string;
  label: string;
  address?: LocationAddress;
  status: "online" | "offline" | "not_ready" | string;
  ready: boolean;
  capabilities?: LocationCapability[];
  auth?: {
    mode?: string;
    credentialId?: string;
    hasCredential?: boolean;
  };
  lastProbe?: LocationProbe;
  createdAt?: string;
  updatedAt?: string;
}

export interface BridgeProject {
  id: string;
  name: string;
  root?: string;
}

export interface BridgeConnection {
  connected: boolean;
  connectionId?: string;
  status?: string;
  connectedAt?: string;
  lastSeenAt?: string;
  agentVersion?: string;
  platform?: string;
  projects?: BridgeProject[];
}

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role?: string;
  lifecycle?: string;
  createdAt: string;
  updatedAt?: string;
}

export interface AuthAPIKey {
  id: string;
  prefix: string;
  name: string;
  createdAt: string;
  lastUsed?: string;
}

export interface AuthStatus {
  enabled: boolean;
  hostedMode?: boolean;
  authenticated: boolean;
  user?: AuthUser | null;
  bridge?: BridgeConnection | null;
}

export interface ProjectSpaceSummary {
  projectId: string;
  spaceId: string;
  title?: string;
  spaceName?: string;
  status: string;
  sortOrder: number;
  pinned: boolean;
  spaceOpen: boolean;
  members?: ProjectMemberLifecycleSummary[];
  reconcileStatus?:
    | "converged"
    | "drifting"
    | "reconciling"
    | "failed"
    | string;
  reconcileReason?: string;
  lifecyclePhase?:
    | "ready"
    | "stopped"
    | "progressing"
    | "degraded"
    | "deleting"
    | string;
  lifecycleReason?: string;
  lifecycleMessage?: string;
  managedBy?: string;
  diagnostic?: SpaceRecoveryDiagnostic;
  projectRoot?: string;
  userId?: string;
  createdAt?: string;
  updatedAt?: string;
  desiredEnabled?: boolean;
  manifestPresent?: boolean;
  metadata?: Record<string, unknown>;
  memberLifecycles?:
    | ProjectMemberLifecycleSummary[]
    | Record<string, ProjectMemberLifecycleSummary>;
  memberLifecycle?:
    | ProjectMemberLifecycleSummary[]
    | Record<string, ProjectMemberLifecycleSummary>;
  lifecycleByMember?: Record<string, ProjectMemberLifecycleSummary>;
  reconcileByMember?: Record<string, ProjectMemberLifecycleSummary>;
}

export interface ProjectMemberLifecycleSummary {
  memberId?: string;
  label?: string;
  member: string;
  spaceId?: string;
  status?: string;
  desiredEnabled?: boolean;
  reconcileStatus?:
    | "converged"
    | "drifting"
    | "reconciling"
    | "failed"
    | string;
  reconcileReason?: string;
  lifecyclePhase?:
    | "ready"
    | "stopped"
    | "progressing"
    | "degraded"
    | "deleting"
    | string;
  lifecycleReason?: string;
  lifecycleMessage?: string;
  managedBy?: string;
  updatedAt?: string;
}

export interface SpaceRecoveryDiagnostic {
  severity?: "info" | "warning" | "error" | string;
  reasonCode?: string;
  summary?: string;
  detail?: string;
  affectedMembers?: string[];
  attemptCount?: number;
  firstObservedAt?: string;
  lastObservedAt?: string;
}

export interface ProjectRecoveryDiagnosticNotification {
  projectRoot?: string;
  projectId?: string;
  spaceId?: string;
  spaceName?: string;
  diagnostic?: SpaceRecoveryDiagnostic;
  manifestBindingCount?: number;
  runtimeBindingCount?: number;
  staleBindingCount?: number;
  coordinatorPointerStale?: boolean;
  tickAt?: string;
}

export interface SpaceMemberStatus {
  memberLabel: string;
  info: string;
}

export interface SpaceRunStatus {
  runId: string;
  memberLabel: string;
  info: string;
}

export interface SpaceGetStatusResult {
  pending: number;
  active: number;
  done: number;
  members: SpaceMemberStatus[];
  runs: SpaceRunStatus[];
  runIds: string[];
  memberLabelByRunId: Record<string, string>;
  totalTokensIn: number;
  totalTokensOut: number;
  totalTokens: number;
  totalCostUSD: number;
  pricingKnown: boolean;
}

export interface SpaceManifestMember {
  memberLabel: string;
  runId: string;
  description?: string;
  skills?: string[];
  allowedTools?: string[];
  runtimeKind?: string;
}

export interface SpaceManifestModelChange {
  requestedModel?: string;
  status?: string;
  requestedAt?: string;
  appliedAt?: string;
  reason?: string;
  error?: string;
}

export interface SpaceGetManifestResult {
  spaceId: string;
  spaceDescription?: string;
  spaceModel?: string;
  supervisedBlockedTools?: string[];
  modelChange?: SpaceManifestModelChange;
  coordinatorMember: string;
  reviewerMember?: string;
  coordinatorRunId: string;
  members: SpaceManifestMember[];
  createdAt: string;
}

export interface SpaceRosterEntry {
  spaceId: string;
  memberLabel: string;
  runId: string;
  model?: string;
  runtimeKind?: string;
  activeReasoningEffort?: string;
  workerPresent: boolean;
  effectiveStatus?: string;
  runTotalTokens?: number;
  runTotalCostUSD?: number;
  lifecyclePhase?:
    | "ready"
    | "stopped"
    | "progressing"
    | "degraded"
    | "deleting"
    | string;
  lifecycleReason?: string;
  lifecycleMessage?: string;
  diagnostic?: SpaceRecoveryDiagnostic;
}

export interface SpaceGetRosterResult {
  spaceId: string;
  members: SpaceRosterEntry[];
}

export interface SpaceListResult {
  spaces: Space[];
  totalCount?: number;
}

export interface SpaceUpdateResult {
  space: Space;
}

/**
 * Closed palette of color keys that the sidebar accent + future surfaces
 * understand. Each key resolves to a theme-aware CSS variable in
 * index.css (--space-color-{key}). Adding a key here without adding the
 * matching CSS variable will render no accent color.
 *
 * Keep in lockstep with the validSpaceColors map on the backend
 * (internal/services/space/rpc/space.go).
 */
export const SPACE_COLOR_KEYS = [
  'slate',
  'blue',
  'violet',
  'green',
  'amber',
  'orange',
  'rose',
  'pink',
] as const

export type SpaceColorKey = typeof SPACE_COLOR_KEYS[number]

/**
 * SpaceCustomization holds user-controlled visual identity for a space.
 *
 * - icon: a Lucide icon name in kebab-case (e.g. "rocket", "git-branch").
 *         Renders a fallback Box icon if the name doesn't resolve client-side.
 * - color: one of SPACE_COLOR_KEYS, resolved through CSS variables so it
 *          renders correctly across dark/light/dim themes.
 *
 * Sending the literal string "none" for either field via space.update
 * explicitly clears that field. Empty strings preserve the existing value.
 */
export interface SpaceCustomization {
  icon?: string;
  color?: SpaceColorKey | string;
}

export interface Space {
  id: string;
  projectId?: string;
  status?: "open" | "closed" | string;
  title?: string;
  createdAt?: string;
  updatedAt?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  costUSD?: number;
  cacheReadInputTokens?: number;
  customization?: SpaceCustomization;
}

export interface Task {
  id: string;
  projectId?: string;
  spaceId?: string;
  assignedTo?: string;
  assignedToLabel?: string;
  claimedByMemberId?: string;
  claimedByMemberLabel?: string;
  createdBy?: string;
  createdByLabel?: string;
  taskKind?: string;
  title?: string;
  description: string;
  acceptanceCriteria?: AcceptanceCriterion[];
  status: string;
  summary?: string;
  error?: string;
  artifacts?: string[];
  keyResultRef?: string;
  missionRef?: string;
  metadata?: Record<string, unknown>;
  createdAt?: string;
  startedAt?: string;
  completedAt?: string;
  updatedAt?: string;
}

export interface AcceptanceCriterion {
  id: string;
  text: string;
  satisfied: boolean;
}

export interface TaskActivity {
  eventId?: string;
  timestamp: string;
  kind: string;
  actor: string;
  agent_id?: string;
  summary: string;
  details?: Record<string, unknown>;
}

export interface AttemptReview {
  decision: string;
  feedback?: string;
  reviewedBy?: string;
  reviewerRole?: string;
  reviewedAt?: string;
}

export interface TaskAttempt {
  attempt: number;
  workerRole?: string;
  summary?: string;
  startedAt?: string;
  completedAt?: string;
  outcome?: string;
  review?: AttemptReview;
}

// ---- Agent event types ----

export type AgentEventStatus = "pending" | "ok" | "error" | "canceled";

export interface AgentEvent {
  id: string;
  kind: string;
  title: string;
  status: AgentEventStatus;
  startedAt: string;
  completedAt?: string;
  duration?: number;
  from?: string;
  to?: string;
  path?: string;
  maxBytes?: string;
  textPreview?: string;
  textTruncated?: boolean;
  textRedacted?: boolean;
  textIsJSON?: boolean;
  textBytes?: string;
  ok?: string;
  error?: string;
  outputPreview?: string;
  bytesLen?: string;
  truncated?: boolean;
  data?: Record<string, string>;
}

export interface ArtifactNode {
  nodeKey: string;
  parentKey?: string;
  kind: "day" | "member" | "stream" | "task" | "file";
  label: string;
  spaceId?: string;
  runId?: string;
  dayBucket?: string;
  member?: string;
  taskKind?: string;
  taskId?: string;
  status?: string;
  artifactId?: string;
  displayName?: string;
  vpath?: string;
  diskPath?: string;
  isSummary?: boolean;
  producedAt?: string;
}

export interface ArtifactGetResult {
  artifact: ArtifactNode;
  content: string;
  contentKind?: "text" | "image" | "pdf" | "binary";
  contentType?: string;
  contentEncoding?: "utf8" | "base64";
  bytesB64?: string;
  truncated: boolean;
  bytesRead: number;
  fileSize?: number;
}

export interface FilesListDirEntry {
  name: string;
  displayName?: string;
  path: string;
  isDir: boolean;
  writable: boolean;
  rootKind?: string;
  rootLabel?: string;
  relativePath?: string;
  size?: number;
  hasSize?: boolean;
  modifiedAt?: string;
}

export interface FilesListDirResult {
  path: string;
  entries: FilesListDirEntry[];
  browseOnly?: boolean;
  displayName?: string;
  rootKind?: string;
  rootLabel?: string;
  relativePath?: string;
}

export interface EventRecord {
  eventId: string;
  runId: string;
  timestamp: string;
  type: string;
  message: string;
  data?: Record<string, string>;
  origin?: string;
}

export interface LogEntry {
  eventId: string;
  runId?: string;
  timestamp: string;
  type: string;
  message: string;
  origin?: string;
  severity: 'info' | 'warning' | 'error' | string;
  category: 'task' | 'agent' | 'llm' | 'system' | 'tools' | string;
  actor?: string;
  scope?: string;
  summary: string;
  details?: string[];
  typeLabel?: string;
  data?: Record<string, string>;
}

export interface ThinkingEntry {
  id: string;
  text: string;
  live: boolean;
  createdAt: number;
  completedAt?: number;
  member?: string;
}

export interface HostOpStreamEvent {
  runId?: string;
  spaceId?: string;
  opId: string;
  op: string;
  stream: "stdout" | "stderr" | string;
  seq: number;
  chunk: string;
  truncated?: boolean;
  timestamp?: string;
}

export interface HostOpStreamNotification {
  event: HostOpStreamEvent;
}

// ---- Notification types ----

export type NotificationSeverity = "info" | "warning" | "critical";

export interface NotificationLink {
  surface: string;
  url: string;
}

export interface NotificationItem {
  id: string;
  userId: string;
  source: string;
  trigger: string;
  severity: NotificationSeverity;
  title: string;
  body: string;
  link?: NotificationLink;
  throttleKey?: string;
  metadata?: Record<string, string>;
  createdAt: string;
  readAt?: string;
  dismissedAt?: string;
}

export interface NotificationRule {
  id: string;
  userId: string;
  source: string;
  trigger: string;
  minSeverity: NotificationSeverity;
  channels: string[];
  cooldownMinutes: number;
  enabled: boolean;
  webhookUrl?: string;
}

export interface NotificationsListResult {
  notifications: NotificationItem[];
}

export interface NotificationsUnreadCountResult {
  count: number;
}

export interface NotificationsRulesListResult {
  rules: NotificationRule[];
}

export interface NotificationsSourcesListResult {
  sources: string[];
  channels: string[];
}

// ---- Model types ----

export interface ModelEntry {
  id: string;
  name?: string;
  description?: string;
  provider: string;
  inputPerM?: number;
  outputPerM?: number;
  contextLength?: number;
  maxCompletionTokens?: number;
  isReasoning?: boolean;
  modality?: string[];
  tokenizer?: string;
  instructType?: string;
}

export interface ModelListResult {
  providers: { name: string; count: number }[];
  models: ModelEntry[];
  total?: number;
  fetchedAt?: string;
}

// ---- Reasoning types ----

export type ReasoningEffort =
  | "none"
  | "minimal"
  | "low"
  | "medium"
  | "high"
  | "xhigh"
  | "max";
export type ReasoningSummary = "off" | "auto" | "concise" | "detailed";

// ---- Config types (matches pkg/protocol/config.go) ----

export interface ConfigLogging {
  level?: string;
  format?: string;
  quiet?: boolean;
  filePath?: string;
  maxSizeMB?: number;
}

export interface RuntimeConfig {
  logging: ConfigLogging;
}

export interface ConfigUpdateResult {
  config: RuntimeConfig;
  appliedNow: string[];
  restartRequired: string[];
  warnings?: string[];
}

export interface ProjectSettings {
  [key: string]: unknown;
}

export interface ProjectConfigUpdateResult {
  config: ProjectSettings;
}

// ── Manifest YAML editor types ──────────────────────

export interface ManifestGetProjectYamlResult {
  yaml: string;
  filePath: string;
}

export interface ManifestUpdateProjectYamlResult {
  yaml: string;
  filePath: string;
  warnings?: string[];
}

export interface ManifestGetTemplateYamlResult {
  yaml: string;
  filePath: string;
  templateId: string;
}

export interface ManifestUpdateTemplateYamlResult {
  yaml: string;
  filePath: string;
  templateId: string;
  warnings?: string[];
}

// ── Members ─────────────────────────────────────────────

export interface PromptConfig {
  systemPrompt?: string;
  systemPromptPath?: string;
  systemFragments?: Array<{ path?: string; inline?: string }>;
}

// ── Mission types ─────────────────────────────────────

export type MissionStatus =
  | "draft"
  | "active"
  | "paused"
  | "completed"
  | "archived";

export interface MissionView {
  id: string;
  projectId: string;
  title: string;
  description?: string;
  status: MissionStatus;
  startDate?: string;
  endDate?: string;
  createdAt: string;
  updatedAt: string;
  pausedAt?: string;
  completedAt?: string;
}

export type KeyResultStatus =
  | "open"
  | "on_track"
  | "at_risk"
  | "completed"
  | "dropped";

export type KeyResultMeasurementType =
  | "percentage"
  | "numeric"
  | "currency"
  | "binary"
  | "count";
export type KeyResultDirection = "increase" | "decrease" | "maintain";

export interface KeyResultView {
  id: string;
  missionId: string;
  title: string;
  description?: string;
  measurementType: KeyResultMeasurementType;
  direction: KeyResultDirection;
  unit?: string;
  baseline?: number;
  targetValue: number;
  currentValue: number;
  progressPercent: number;
  lastUpdatedBy?: string;
  lastUpdateNote?: string;
  lastMilestoneNotified: number;
  spaceId?: string;
  ownerSpaceName?: string;
  ownerAssignedAt?: string;
  status: KeyResultStatus;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
}

// ── Context Link types (graph links API) ────────────

export type ContextLinkEdgeType =
  | "blocked_by"
  | "resolved_by"
  | "completed_by"
  | "serves"
  | "informed_by"
  | "produced"
  | "made_during"
  | "spawned"
  | "relates_to";

export interface ContextLink {
  id: string;
  sourceType: string;
  sourceId: string;
  targetType: string;
  targetId: string;
  edgeType: ContextLinkEdgeType;
  confidence: number;
  metadata?: Record<string, string>;
  createdAt: string;
  createdBy?: string;
}

// ── Decision types ─────────────────────────────────────

export type DecisionSource = "agent";

export interface DecisionView {
  id: string;
  projectId: string;
  spaceId?: string;
  spaceName?: string;
  source: DecisionSource;
  kind?: string;
  // memberId is the asker's stable id. memberName is the resolved
  // display name from the space-member registry — UI surfaces should
  // prefer memberName so the raw id never lands in a card.
  memberId?: string;
  memberName?: string;
  sourceIdentity?: string;
  runId?: string;
  title: string;
  rationale: string;
  context?: string;
  alternativesRejected?: string;
  invalidationConditions?: string[];
  confidence: number;
  outcome?: string;
  taskRef?: string;
  keyResultRef?: string;
  missionRef?: string;
  correlationRef?: string;
  informedByRef?: string;
  tags?: string[];
  metadata?: Record<string, string>;
  createdAt: string;
}
