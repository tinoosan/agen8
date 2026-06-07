import type { CredentialListParams } from '../hooks/useCredentials'

/**
 * Central TanStack Query key factory.
 *
 * Every query/mutation cache key in the app is built here so there is one place
 * that defines the shape of each entry. Two kinds of members:
 *
 *   - **Builders** (functions) return the FULL key a query subscribes with,
 *     e.g. `qk.missions(projectId, status)` → `['mission.list', projectId, status]`.
 *   - **Roots** (bare arrays, suffixed `*All`/`*Root`) are the prefixes that
 *     mutations invalidate. TanStack matches invalidation keys by prefix, so
 *     `invalidateQueries({ queryKey: qk.missionsAll })` clears every cached
 *     mission query regardless of project/status.
 *
 * The keys here are intentionally byte-identical to the inline literals they
 * replaced — this module is a consolidation, not a cache reshape. (Closing the
 * known invalidation gaps is a separate, deliberate change.)
 */
export const qk = {
  // ── missions ──────────────────────────────────────────
  missionsAll: ['mission.list'] as const,
  missions: (projectId: string | null, status?: string) =>
    ['mission.list', projectId ?? '', status ?? ''] as const,
  missionGet: (missionId: string | null) =>
    ['mission.get', missionId ?? ''] as const,
  sidebarGlobalMissions: (projectIds: string[]) =>
    ['sidebar.globalMissions', projectIds] as const,

  // ── key results ───────────────────────────────────────
  keyResultsAll: ['keyResult.list'] as const,
  keyResults: (missionId: string | null) =>
    ['keyResult.list', missionId ?? ''] as const,
  keyResultGet: (keyResultId: string | null) =>
    ['keyResult.get', keyResultId ?? ''] as const,
  keyResultsListAllRoot: ['keyResult.listAll'] as const,
  keyResultsListAll: (projectId: string | null, missionIds: string[]) =>
    ['keyResult.listAll', projectId ?? '', missionIds.join(',')] as const,
  keyResultsByMissionSet: (missionIds: string[]) =>
    ['keyResult.listByMissionSet', missionIds.join(',')] as const,
  keyResultProgressHistoryRoot: ['keyResult.progressHistory'] as const,
  keyResultProgressHistory: (keyResultId: string | null) =>
    ['keyResult.progressHistory', keyResultId ?? ''] as const,

  // ── decisions ─────────────────────────────────────────
  decisionsAll: ['decision.list'] as const,
  decisionList: (
    projectId: string,
    source: string,
    query: string,
    since: string,
    until: string,
    sort: string,
  ) => ['decision.list', projectId, source, query, since, until, sort] as const,
  decisionGet: (decisionId: string | null) =>
    ['decision.get', decisionId ?? ''] as const,
  decisionLogAll: ['decision.log'] as const,
  decisionLog: (
    projectId: string,
    source: string,
    tagsKey: string,
    query: string,
    since: string,
    until: string,
    sort: string,
    page: number,
    pageSize: number,
  ) =>
    ['decision.log', projectId, source, tagsKey, query, since, until, sort, page, pageSize] as const,
  decisionStats: (
    projectId: string,
    source: string,
    tagsKey: string,
    query: string,
    since: string,
    until: string,
  ) => ['decision.stats', projectId, source, tagsKey, query, since, until] as const,

  // ── tasks ─────────────────────────────────────────────
  tasksBoardAll: ['project.tasks.board'] as const,
  tasksBoard: (projectId: string | null) =>
    ['project.tasks.board', projectId ?? ''] as const,
  taskGetAll: ['task.get'] as const,
  taskGet: (taskId: string | null) => ['task.get', taskId ?? ''] as const,

  // ── projects ──────────────────────────────────────────
  projectsAll: ['project.list'] as const,
  projects: (includeArchived: boolean) =>
    ['project.list', includeArchived ? 'withArchived' : 'active'] as const,
  projectMembersAll: ['project.member.list'] as const,
  projectMembers: (projectId: string | null) =>
    ['project.member.list', projectId ?? ''] as const,

  // ── locations ─────────────────────────────────────────
  locations: ['location.list'] as const,

  // ── config ────────────────────────────────────────────
  config: ['config.get'] as const,
  projectConfig: ['config.getProject'] as const,

  // ── auth ──────────────────────────────────────────────
  authStatus: ['auth.status'] as const,

  // ── credentials ───────────────────────────────────────
  credentialsAll: ['credential.list'] as const,
  credentials: (params: CredentialListParams) =>
    ['credential.list', params] as const,

  // ── files ─────────────────────────────────────────────
  filePreview: (
    projectId: string | null,
    projectRoot: string | null,
    filePath: string | null,
  ) => ['files.get.preview', projectId, projectRoot, filePath] as const,

  // ── graph links ───────────────────────────────────────
  graphLinksByTarget: (targetType: string, targetId: string) =>
    ['graph.linksByTarget', targetType, targetId] as const,
  graphLinksBySource: (sourceType: string, sourceId: string) =>
    ['graph.linksBySource', sourceType, sourceId] as const,
}
