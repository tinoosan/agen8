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

/** Mirrors Go types.Channel. A member address within a space workspace. */
export interface Channel {
  id: string;
  spaceId: string;
  projectId?: string;
  runId?: string;
  memberId?: string;
  memberLabel?: string;
  title?: string;
  status?: string;
  createdAt: string;
  updatedAt?: string;
  /** ISO timestamp of the most recent message published into this address. */
  lastMessageAt?: string;
  /**
   * Computed per-user at list time. True iff the user has a message newer
   * than their last seen marker, or has never read the address after it
   * received a message. Cleared by calling channel.markRead.
   */
  unread?: boolean;
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
  planMode?: PlanMode;
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
  /**
   * Space-scoped execution mode. This is the effective planMode for this
   * specific space — it may differ from the template definition's configured
   * default mode. Always prefer this over manifest.planMode when displaying
   * state to the user in the context of a specific conversation/space.
   */
  planMode?: PlanMode | string;
  createdAt?: string;
  updatedAt?: string;
  inputTokens?: number;
  outputTokens?: number;
  totalTokens?: number;
  costUSD?: number;
  cacheReadInputTokens?: number;
  customization?: SpaceCustomization;
}

export interface SpaceMember {
  id: string;
  userId?: string;
  projectId?: string;
  spaceId: string;
  channelId: string;
  displayName: string;
  memberType: string;
  lifecycleState: string;
  harnessKind: string;
  model: string;
  effort: string;
  harnessPermissionMode?: string;
  harnessConfigRef?: string;
  currentRunId?: string;
  registeredAt?: string;
  updatedAt?: string;
  lastSeenAt?: string;
}

export interface SpaceMemberListResult {
  members: SpaceMember[];
}

export interface SpaceMemberRemoveResult {
  member: SpaceMember;
}

export interface SpaceContextStatus {
  usedTokens?: number;
  maxTokens?: number;
  remainingTokens?: number;
  compactedAt?: string;
}

export type SpaceDetailEntryKind =
  | "user_message"
  | "agent_message"
  | "thinking"
  | "tool_call"
  | "note"
  | "error";

export interface SpaceDetailEntry {
  id: string;
  kind: SpaceDetailEntryKind | string;
  runId?: string;
  turnId?: string;
  messageId?: string;
  toolCallId?: string;
  sequence?: number;
  member?: string;
  title?: string;
  text?: string;
  status?: string;
  createdAt: string;
  completedAt?: string;
  live?: boolean;
  data?: Record<string, string>;
}

export interface SpaceDetailResult {
  space: Space;
  entries: SpaceDetailEntry[];
  context?: SpaceContextStatus;
}

export interface Task {
  id: string;
  spaceId: string;
  assignedTo?: string;
  assignedToLabel?: string;
  claimedByMemberId?: string;
  createdBy?: string;
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
  planPhaseId?: string;
  planTodoId?: string;
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

export interface MailMessage {
  messageId: string;
  correlationId?: string;
  spaceId?: string;
  runId?: string;
  sourceSpaceId?: string;
  destinationSpaceId?: string;
  actorMemberId?: string;
  targetMemberId?: string;
  broadcast?: boolean;
  channel: string;
  kind: string;
  senderType?: string;
  senderName?: string;
  senderSpace?: string;
  status: string;
  subject?: string;
  summary?: string;
  bodyPreview?: string;
  error?: string;
  taskId?: string;
  taskStatus?: string;
  readOnly?: boolean;
  canClaim?: boolean;
  canComplete?: boolean;
  createdAt: string;
  updatedAt: string;
  processedAt?: string;
  task?: Task;
}

// ---- Message domain types (matches pkg/types/message.go + agent_message.go) ----

/** Canonical message kind constants. */
export const MessageKind = {
  Task: "task",
  UserInput: "user_input",
  Inform: "inform",
  Query: "query",
  Response: "response",
  Timeout: "timeout",
  System: "system",
} as const;
export type MessageKind = (typeof MessageKind)[keyof typeof MessageKind];

/** Message channel (inbox / outbox). */
export const MessageChannel = {
  Inbox: "inbox",
  Outbox: "outbox",
} as const;
export type MessageChannel =
  (typeof MessageChannel)[keyof typeof MessageChannel];

/** Message delivery status. */
export const MessageStatus = {
  Pending: "pending",
  Claimed: "claimed",
  Acked: "acked",
  Nacked: "nacked",
  Deadletter: "deadletter",
} as const;
export type MessageStatus = (typeof MessageStatus)[keyof typeof MessageStatus];

/**
 * Domain-facing communication record.
 * First-class message entity with explicit sender identity.
 */
export interface Message {
  messageId: string;
  correlationId?: string;
  spaceId?: string;
  sourceSpaceId?: string;
  destinationSpaceId?: string;
  sourceMemberId?: string;
  destinationMemberId?: string;
  senderType?: string;
  senderName?: string;
  senderSpace?: string;
  assignedToType?: string;
  assignedTo?: string;
  kind: string;
  subject?: string;
  body?: string;
  taskRef?: string;
  metadata?: Record<string, unknown>;
  createdAt?: string;
  processedAt?: string;
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

export interface RuntimeRunState {
  runId: string;
  model: string;
  runtimeKind?: string;
  status: string;
  effectiveStatus: string;
  workerPresent: boolean;
  runTotalTokens: number;
  runTotalCostUSD: number;
}

export interface RuntimeGetSpaceStateResult {
  spaceId: string;
  runs: RuntimeRunState[];
}

// ---- Agent / Dashboard types ----

export interface AgentInfo {
  agentId: string;
  runId: string;
  member: string;
  status: string;
  profile?: string;
  createdAt?: string;
}

export interface AgentListResult {
  agents: AgentInfo[];
}

export interface SpaceTotals {
  totalTokensIn: number;
  totalTokensOut: number;
  totalTokens: number;
  totalCostUSD: number;
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
  category: 'task' | 'agent' | 'operator' | 'llm' | 'system' | 'tools' | string;
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

// ---- Plan types ----

export type PlanMode = "autonomous" | "supervised";
export type PlanStatus =
  | "draft"
  | "pending_approval"
  | "active"
  | "completed"
  | "abandoned";

export interface PlanView {
  id: string;
  spaceId: string;
  missionId: string;
  krRefs?: string[];
  title: string;
  description?: string;
  mode: PlanMode | string;
  status: PlanStatus | string;
  createdBy: string;
  createdAt: string;
  updatedAt: string;
  abandonedReason?: string;
  version: number;
}

export interface PlanPhaseView {
  id: string;
  planId: string;
  title: string;
  order: number;
  status: string;
  createdAt: string;
  completedAt?: string;
  version: number;
}

export interface PlanTodoView {
  id: string;
  phaseId: string;
  planId: string;
  text: string;
  done: boolean;
  order: number;
  createdAt: string;
  completedAt?: string;
  version: number;
}

export interface PlanCommentView {
  id: string;
  planId: string;
  phaseId?: string;
  todoId?: string;
  authorType: string;
  authorId: string;
  text: string;
  createdAt: string;
}

export interface PlanChangeView {
  kind: string;
  data?: Record<string, unknown>;
  meta?: Record<string, string>;
}

export interface PlanAmendmentView {
  id: string;
  planId: string;
  proposedBy: string;
  rationale?: string;
  diff?: PlanChangeView[];
  status: string;
  vetoDeadline: string;
  vetoedBy?: string;
  vetoReason?: string;
  createdAt: string;
  resolvedAt?: string;
  version: number;
}

export interface PlanReviewView {
  id: string;
  planId: string;
  decision: string;
  note?: string;
  createdAt: string;
}

export interface PlanGetV2Result {
  plan: PlanView;
  phases?: PlanPhaseView[];
  todos?: PlanTodoView[];
  comments?: PlanCommentView[];
  unread: number;
  amendments?: PlanAmendmentView[];
  reviews?: PlanReviewView[];
}

export interface PlanListResult {
  plans: PlanView[];
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

export interface ConfigSpaceMessage {
  queryTimeoutSec?: number;
}

export interface RuntimeConfig {
  logging: ConfigLogging;
  spaceMessage?: ConfigSpaceMessage;
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

export interface HeartbeatJob {
  name: string;
  interval: string;
  schedule?: string; // cron expression (e.g. "0 9 * * 1-5"); takes precedence over interval
  goal: string;
  paused?: boolean; // per-job pause flag; paused jobs excluded from EffectiveHeartbeats()
  maxOccurrences?: number; // max number of runs; 0 = unlimited
  expiresAt?: string; // ISO 8601 timestamp; schedule expiry
  errorBudget?: ErrorBudget; // circuit breaker config; nil = defaults (3 failures, 1h cooldown)
}

// Circuit breaker thresholds for heartbeat error budget
export interface ErrorBudget {
  maxConsecutiveFailures?: number; // consecutive failures before circuit opens
  cooldownInterval?: string; // Go duration string (e.g. "1h") — cooldown before half-open probe
}

// Structured outcome from heartbeat execution
export interface HeartbeatOutcome {
  status: string; // "ok" | "warning" | "critical" | "error"
  summary: string; // one-line finding from the heartbeat run
  actions?: string[]; // actions taken (coordinator heartbeats only)
}

// Calendar API response entry — one per heartbeat execution or skip
export interface HeartbeatHistoryEntry {
  jobName: string;
  member: string;
  memberType: string; // "coordinator" | "worker"
  taskId: string;
  executedAt: string; // ISO 8601
  duration: number; // milliseconds
  outcome?: HeartbeatOutcome; // nil if task failed before setting outcome
  reviewDecision?: string; // "approve" | "retry" | "escalate" | "" (no review)
  reviewFeedback?: string;
  triggerSource?: string; // "scheduled" | "agent" | "webhook"
  triggerReason?: string;
  skipped: boolean;
  skipReason?: string; // "backpressure" | "maxOccurrences" | "expired"
  source?: string; // "config" | "agent" | "operator"
  scheduleEntryId?: string; // for agent/operator entries
  entryStatus?: string; // "active" | "pending_approval" — for agent-created entries (F31)
  guardrailReason?: string; // why the entry is pending approval (which guardrail was exceeded)
}

// Agent/operator-created schedule entry (F29)
export interface ScheduleEntry {
  entryId: string;
  spaceId: string;
  memberId: string;
  createdBy: string; // member label, operator, or system
  name: string;
  goal: string;
  context?: ScheduleEntryContext;
  priority: number;
  scheduleType: string; // "one_off" | "recurring"
  scheduleExpr: string; // cron expression or Go duration
  status: string; // "active" | "pending_approval" | "triggered" | "expired" | "cancelled"
  guardrailReason?: string;
  dedupeKey?: string;
  expiresAt?: string; // ISO 8601
  nextRunAt?: string; // ISO 8601
  createdAt: string; // ISO 8601
  updatedAt: string; // ISO 8601
  runs?: ScheduleRun[];
}

export interface ScheduleRun {
  id: string;
  dueAt: string;
  startedAt: string;
  finishedAt?: string;
  status: string;
  targetKind: string;
  targetObjectId?: string;
  error?: string;
}

// Structured context on agent-scheduled entries (F28)
export interface ScheduleEntryContext {
  relatedTaskId?: string;
  checkCondition?: string;
  successCriteria?: string;
}

// Operator-configured limits on agent self-scheduling (F30)
export interface ScheduleGuardrails {
  maxActiveHeartbeats?: number; // max concurrent agent-created entries
  minInterval?: string; // minimum interval between runs (Go duration)
  maxLookahead?: string; // max how far ahead an agent can schedule (Go duration)
  dailyBudget?: number; // max runs per day
  weeklyBudget?: number; // max runs per week
  defaultTtl?: string; // default TTL for agent entries (Go duration, e.g. "24h")
}

export interface HeartbeatConfig {
  enabled?: boolean;
  jobs?: HeartbeatJob[];
  guardrails?: ScheduleGuardrails; // per-member guardrails for agent self-scheduling
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

// ── Shared operator types ─────────────────────────────

export type OperatorUrgency = "low" | "medium" | "high" | "critical";
export type OperatorCategory =
  | "financial"
  | "legal"
  | "content"
  | "code"
  | "general"
  | "physical"
  | "communication"
  | "administrative";

// Legacy aliases (used by existing components — remove after full migration)
export type OAUrgency = OperatorUrgency;
export type OACategory = OperatorCategory;
export type OAResolution = EscalationResolution;

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

// ── Escalation types (escalation.* API) ──────────────

export type EscalationStatus = "pending" | "resolved" | "expired" | "canceled";
export type EscalationResolution =
  | "approve"
  | "reject"
  | "redirect"
  | "defer"
  | "delegate";

export interface EscalationView {
  id: string;
  projectId: string;
  spaceId?: string;
  taskRef?: string;
  keyResultRef?: string;
  runId?: string;
  source: string;
  sourceMemberLabel?: string;
  category: OperatorCategory;
  urgency: OperatorUrgency;
  title: string;
  description: string;
  recommendation?: string;
  confidence?: number;
  status: EscalationStatus;
  resolution?: EscalationResolution;
  resolutionNote?: string;
  delegatedTo?: string;
  deadline?: string;
  escalatedAt?: string;
  originalUrgency?: string;
  metadata?: Record<string, string>;
  createdAt: string;
  resolvedAt?: string;
  resolvedBy?: string;
}

// ── Operator Action types (opAction.* lifecycle API) ──

export type OpActionStatus =
  | "pending"
  | "acknowledged"
  | "in_progress"
  | "pending_verification"
  | "completed"
  | "blocked"
  | "canceled";
export type OpActionOutcomeStatus = "completed" | "partial" | "failed";

export interface OpActionAttachment {
  id: string;
  kind: string;
  filename?: string;
  contentType?: string;
  sizeBytes?: number;
  url?: string;
  label?: string;
  createdAt: string;
}

export interface OpActionNote {
  text: string;
  createdAt: string;
}

export interface OpActionComment {
  author: string;
  text: string;
  createdAt: string;
}

export interface OpActionView {
  id: string;
  projectId: string;
  spaceId?: string;
  taskRef?: string;
  keyResultRef?: string;
  runId?: string;
  blocking: boolean;
  source: string;
  sourceMemberLabel?: string;
  escalationRef?: string;
  category: string;
  urgency: string;
  title: string;
  description: string;
  requiresVerification: boolean;
  status: OpActionStatus;
  outcomeStatus?: OpActionOutcomeStatus;
  outcomeSummary?: string;
  outcomePairs?: Record<string, string>;
  attachments?: OpActionAttachment[];
  progressNotes?: OpActionNote[];
  comments?: OpActionComment[];
  deadline?: string;
  metadata?: Record<string, string>;
  createdAt: string;
  acknowledgedAt?: string;
  startedAt?: string;
  completedAt?: string;
  verifiedAt?: string;
}

// Legacy alias for components still importing the old name
export type OAStatus = EscalationStatus;
export type OperatorActionView = EscalationView;

// ── Decision types ─────────────────────────────────────

export type DecisionSource = "agent" | "operator" | "policy";

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
  questions?: Array<{
    id: string;
    text: string;
    type: "multiple_choice" | "free_form";
    options?: string[];
    allowFreeForm?: boolean;
    recommendation?: string;
    blocking?: boolean;
  }>;
  answers?: Array<{
    questionId: string;
    selectedOption?: string;
    freeFormText?: string;
  }>;
  cancelled?: boolean;
  alternativesRejected?: string;
  invalidationConditions?: string[];
  confidence: number;
  outcome?: string;
  taskRef?: string;
  keyResultRef?: string;
  missionRef?: string;
  planRef?: string;
  operatorActionRef?: string;
  escalationRef?: string;
  correlationRef?: string;
  informedByRef?: string;
  tags?: string[];
  metadata?: Record<string, string>;
  createdAt: string;
}
