import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { rpcCall, rpcUnwrap, rpcUnwrapList } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import type { MissionView, KeyResultView, MissionStatus, KeyResultStatus, ProgressEntryView } from '../lib/types'

type CreateKeyResultInput = {
  missionId: string
  title: string
  description?: string
  measurementType: string
  direction: string
  targetValue?: number
  unit?: string
  baseline?: number
}

type UpdateKeyResultInput = {
  keyResultId: string
  missionId: string // used for cache invalidation only
  title?: string
  description?: string
  measurementType?: string
  direction?: string
  targetValue?: number
  currentValue?: number
  unit?: string
  baseline?: number
  progressPercent?: number
  status?: KeyResultStatus
}

/* ── Query hooks ─────────────────────────────────────── */

export function useMissions(projectId: string | null, status?: MissionStatus) {
  return useQuery<MissionView[]>({
    queryKey: qk.missions(projectId, status),
    queryFn: async () => {
      const params: Record<string, unknown> = { projectId: projectId ?? '' }
      if (status) params.status = [status]
      return rpcUnwrapList<MissionView>('mission.list', params, 'missions')
    },
    enabled: !!projectId,
    refetchInterval: 10_000,
  })
}

export function useKeyResults(missionId: string | null) {
  return useQuery<KeyResultView[]>({
    queryKey: qk.keyResults(missionId),
    queryFn: async () => {
      return rpcUnwrapList<KeyResultView>('mission.kr.list', { missionId: missionId ?? '' }, 'keyResults')
    },
    enabled: !!missionId,
    refetchInterval: 10_000,
  })
}

export function useKeyResult(keyResultId: string | null) {
  return useQuery<KeyResultView>({
    queryKey: qk.keyResultGet(keyResultId),
    queryFn: async () => {
      return rpcUnwrap<KeyResultView>('mission.kr.get', { keyResultId: keyResultId ?? '' }, 'keyResult')
    },
    enabled: !!keyResultId,
    refetchInterval: 10_000,
  })
}

/* ── Mission mutation hooks ──────────────────────────────────────────────
 *
 * Cross-surface invalidation strategy:
 * All mission/KR mutations invalidate ['mission.list'] with prefix matching
 * (no trailing projectId/status), which clears every cached mission query
 * across all projects and status filters. This ensures the board, dashboard,
 * and mission panels stay consistent after any mutation.
 *
 * ────────────────────────────────────────────────────────────────────────── */

export function useCreateMission() {
  const queryClient = useQueryClient()
  return useMutation<
    { mission: MissionView },
    Error,
    { projectId: string; title: string; description?: string; startDate?: string; endDate?: string }
  >({
    mutationFn: (params) => rpcCall<{ mission: MissionView }>('mission.create', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
      queryClient.invalidateQueries({ queryKey: qk.sidebarGlobalMissionsRoot })
      queryClient.invalidateQueries({ queryKey: qk.missionGetAll })
    },
  })
}

export function useUpdateMission() {
  const queryClient = useQueryClient()
  return useMutation<
    { mission: MissionView },
    Error,
    { missionId: string; title?: string; description?: string; status?: MissionStatus }
  >({
    mutationFn: (params) => rpcCall<{ mission: MissionView }>('mission.update', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
      queryClient.invalidateQueries({ queryKey: qk.sidebarGlobalMissionsRoot })
      queryClient.invalidateQueries({ queryKey: qk.missionGetAll })
    },
  })
}

export function useDeleteMission() {
  const queryClient = useQueryClient()
  return useMutation<{ mission: MissionView }, Error, { missionId: string }>({
    mutationFn: (params) => rpcCall<{ mission: MissionView }>('mission.delete', params),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
      queryClient.invalidateQueries({ queryKey: qk.sidebarGlobalMissionsRoot })
      queryClient.invalidateQueries({ queryKey: qk.missionGetAll })
    },
  })
}

/* ── Key Result mutation hooks ───────────────────────── */

export function useCreateKeyResult() {
  const queryClient = useQueryClient()
  return useMutation<
    { keyResult: KeyResultView },
    Error,
    CreateKeyResultInput
  >({
    mutationFn: (params) => rpcCall<{ keyResult: KeyResultView }>('mission.kr.create', normalizeKeyResultMutation(params)),
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.keyResults(vars.missionId) })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsListAllRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsByMissionSetRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultGetAll })
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
    },
  })
}

export function useUpdateKeyResult() {
  const queryClient = useQueryClient()
  return useMutation<
    { keyResult: KeyResultView },
    Error,
    UpdateKeyResultInput
  >({
    mutationFn: (vars) => {
      const { missionId: _, ...params } = vars // eslint-disable-line @typescript-eslint/no-unused-vars
      return rpcCall<{ keyResult: KeyResultView }>('mission.kr.update', normalizeKeyResultMutation(params))
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.keyResults(vars.missionId) })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsListAllRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsByMissionSetRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultGetAll })
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
    },
  })
}

function normalizeKeyResultMutation<T extends { measurementType?: string; direction?: string }>(input: T): T {
  const out = { ...input }
  if (out.measurementType) {
    out.measurementType = normalizeMeasurementType(out.measurementType)
  }
  if (out.measurementType === 'boolean') {
    delete out.direction
  }
  return out
}

function normalizeMeasurementType(value: string): string {
  switch (value) {
    case 'count':
    case 'numeric':
    case 'currency':
      return 'number'
    case 'binary':
      return 'boolean'
    default:
      return value
  }
}

export function useDeleteKeyResult() {
  const queryClient = useQueryClient()
  return useMutation<
    { keyResult: KeyResultView },
    Error,
    { keyResultId: string; missionId: string }
  >({
    mutationFn: (vars) => {
      const { missionId: _, ...params } = vars // eslint-disable-line @typescript-eslint/no-unused-vars
      return rpcCall<{ keyResult: KeyResultView }>('mission.kr.delete', params)
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.keyResults(vars.missionId) })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsListAllRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsByMissionSetRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultGetAll })
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
    },
  })
}

export function useUpdateKRProgress() {
  const queryClient = useQueryClient()
  return useMutation<
    { keyResult: KeyResultView },
    Error,
    { keyResultId: string; missionId: string; value: number; note: string }
  >({
    mutationFn: (vars) => {
      const { missionId: _, ...params } = vars // eslint-disable-line @typescript-eslint/no-unused-vars
      return rpcCall<{ keyResult: KeyResultView }>('mission.kr.progress', params)
    },
    onSuccess: (_data, vars) => {
      queryClient.invalidateQueries({ queryKey: qk.keyResults(vars.missionId) })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsListAllRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultsByMissionSetRoot })
      queryClient.invalidateQueries({ queryKey: qk.keyResultGetAll })
      queryClient.invalidateQueries({ queryKey: qk.keyResultProgressHistory(vars.keyResultId) })
      queryClient.invalidateQueries({ queryKey: qk.missionsAll })
    },
  })
}

// useProjectKRs fetches all KRs across every mission in the project and
// returns them as a flat Map<krId, KeyResultView>. Used by the board to
// resolve KR titles without per-card queries.
export function useProjectKRs(projectId: string | null) {
  const missionsQuery = useMissions(projectId)
  const missionIds = (missionsQuery.data ?? []).map((m) => m.id)

  return useQuery<Map<string, KeyResultView>>({
    queryKey: qk.keyResultsListAll(projectId, missionIds),
    queryFn: async () => {
      const results = await Promise.all(
        missionIds.map((id) =>
          rpcUnwrapList<KeyResultView>('mission.kr.list', { missionId: id }, 'keyResults'),
        ),
      )
      const map = new Map<string, KeyResultView>()
      for (const krs of results) {
        for (const kr of krs) {
          map.set(kr.id, kr)
          map.set(`key_result:${kr.id}`, kr)
          map.set(`keyResult:${kr.id}`, kr)
        }
      }
      return map
    },
    enabled: missionIds.length > 0,
    refetchInterval: 10_000,
  })
}

/* ── Progress history ───────────────────────────────── */

interface MissionProgressEntryRPCView {
  id: string
  keyResultId: string
  previousValue: number
  newValue: number
  progressPercent: number
  updatedBy: string
  note?: string
  createdAt: string
}

export function useProgressHistory(keyResultId: string | null) {
  return useQuery<ProgressEntryView[]>({
    queryKey: qk.keyResultProgressHistory(keyResultId),
    queryFn: async () => {
      const res = await rpcCall<{ entries: MissionProgressEntryRPCView[] }>(
        'mission.kr.progressHistory',
        { keyResultId: keyResultId ?? '' },
      )
      return (res.entries ?? []).map((entry) => ({
        id: entry.id,
        keyResultId: entry.keyResultId,
        value: entry.newValue,
        progress: entry.progressPercent,
        updatedBy: entry.updatedBy,
        note: entry.note,
        createdAt: entry.createdAt,
      }))
    },
    enabled: !!keyResultId,
    refetchInterval: 10_000,
  })
}
