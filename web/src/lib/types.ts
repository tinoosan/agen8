// Mirrors the Go protocol types used by the web UI.
import type { UserPreferences } from './store'

// Generic RPC list envelope: a response that carries a single named array
// field, e.g. RpcList<'tasks', Task> === { tasks: Task[] }. Replaces the
// per-hook one-off result interfaces (TaskListResult, CredentialListResult,
// …) so every "{ <field>: T[] }" RPC shape is described one way.
export type RpcList<K extends string, T> = { [P in K]: T[] };

export type ProjectStatus = "open" | "archived";

export interface Project {
  id: string;
  locationId: string;
  root: string;
  title?: string;
  status: ProjectStatus;
  createdAt?: string;
  updatedAt?: string;
}

export interface LocationAddress {
  host?: string;
  port?: number;
  username?: string;
}

// Outcome of a single location capability check or probe. Shared by
// LocationCapability and LocationProbe — same three-state result.
export type LocationCheckStatus = "passed" | "failed" | "unknown";
export type LocationKind = "local" | "ssh" | "managed";
export type LocationStatus = "online" | "offline" | "not_ready";

export interface LocationCapability {
  name: string;
  status: LocationCheckStatus;
}

export interface LocationProbe {
  status?: LocationCheckStatus;
  failureCode?: string;
  message?: string;
  probedAt?: string;
}

export interface ExecutionLocation {
  id: string;
  kind: LocationKind;
  label: string;
  address?: LocationAddress;
  status: LocationStatus;
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

export interface AuthUser {
  id: string;
  email: string;
  name: string;
  role?: string;
  lifecycle?: string;
  preferences?: UserPreferences;
  createdAt: string;
  updatedAt?: string;
}

export interface AuthAPIKey {
  id: string;
  prefix: string;
  name: string;
  createdAt: string;
  expiresAt?: string;
  revokedAt?: string;
  active?: boolean;
  lastUsed?: string;
}

export interface AuthStatus {
  enabled: boolean;
  hostedMode?: boolean;
  authenticated: boolean;
  setupOpen?: boolean;
  setupUrl?: string;
  user?: AuthUser | null;
}

export interface ProjectMember {
  id: string;
  userId?: string;
  projectId: string;
  nativeSessionRef?: string;
  channelId?: string;
  displayName?: string;
  memberType: string;
  /** Harness the daemon auto-detected at registration (e.g. "claude-code",
   *  "codex", "bridge"); "unknown" when no signal identified it. */
  harnessKind?: string;
  lifecycleState: string;
  registeredAt?: string;
  updatedAt?: string;
  lastSeenAt?: string;
}

export interface Task {
  id: string;
  projectId?: string;
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

export interface AttemptReview {
  decision: string;
  feedback?: string;
  summary?: string;
  note?: string;
  reason?: string;
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

export interface ArtifactNode {
  nodeKey: string;
  parentKey?: string;
  kind: "day" | "member" | "stream" | "task" | "file";
  label: string;
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
  // The unread tally rides along with the list so the bell badge updates in the
  // same round-trip the inbox does.
  unreadCount: number;
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
  projectId?: string;
  ownerProjectName?: string;
  ownerAssignedAt?: string;
  status: KeyResultStatus;
  createdAt: string;
  updatedAt: string;
  completedAt?: string;
}

// A single key-result progress sample, normalized for the UI (previous/new
// values folded to a single `value`, progressPercent → `progress`). The
// useProgressHistory hook maps the raw RPC shape onto this view.
export interface ProgressEntryView {
  id: string;
  keyResultId: string;
  value: number;
  progress: number;
  updatedBy: string;
  note?: string;
  createdAt: string;
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
  source: DecisionSource;
  kind?: string;
  // memberId is the asker's stable id. memberName is the resolved
  // display name from the project member registry — UI surfaces should
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

// DecisionStats is the aggregate summary returned by decision.stats — computed
// server-side over the full filtered set, not just the current page.
export interface DecisionStats {
  total: number;
  lowConfidence: number;
  unlinked: number;
  withInvalidationConditions: number;
}

// ── Credential types ───────────────────────────────────

export type CredentialKind = "ssh_agent" | "ssh_key" | "ssh_password" | "api_key";
export type CredentialStatus = "active" | "disabled" | "invalid";
export type CredentialFieldKind = "public" | "secret";

export interface CredentialFieldView {
  name: string;
  kind: CredentialFieldKind;
  configured: boolean;
}

export interface CredentialView {
  id: string;
  kind: CredentialKind;
  label: string;
  status: CredentialStatus;
  fields?: CredentialFieldView[];
  createdAt?: string;
  updatedAt?: string;
}
