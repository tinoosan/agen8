import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import type { Node, Edge } from '@xyflow/react'
import { rpcCall } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { useMissions } from '../../hooks/useMissions'
import type { KeyResultView } from '../../lib/types'
import type { MissionNodeData } from './MissionNode'
import type { KRNodeData } from './KRNode'

export function useMissionKRNodes(projectId: string | null, _projectRoot: string | null, options: { showArchived?: boolean } = {}): {
  nodes: Node[]
  edges: Edge[]
  isLoading: boolean
} {
  const missionsQuery = useMissions(projectId)
  const missions = useMemo(
    () => {
      const data = missionsQuery.data ?? []
      return options.showArchived ? data : data.filter((mission) => mission.status !== 'archived')
    },
    [missionsQuery.data, options.showArchived],
  )
  const missionIds = useMemo(() => missions.map((m) => m.id), [missions])

  // Consolidated map query to avoid one active query observer per mission.
  const krByMissionQuery = useQuery<Record<string, KeyResultView[]>>({
    queryKey: qk.keyResultsByMissionSet(missionIds),
    queryFn: async () => {
      const pairs = await Promise.all(
        missionIds.map(async (missionId) => {
          const res = await rpcCall<{ keyResults: KeyResultView[] }>('mission.kr.list', { missionId })
          return [missionId, res.keyResults ?? []] as const
        }),
      )
      return Object.fromEntries(pairs)
    },
    enabled: missionIds.length > 0,
    refetchInterval: 30_000,
    staleTime: 20_000,
    refetchOnWindowFocus: false,
  })
  const krByMission = useMemo(() => krByMissionQuery.data ?? {}, [krByMissionQuery.data])

  const isLoading =
    (missionsQuery.isLoading && !missionsQuery.data) ||
    (missionIds.length > 0 && krByMissionQuery.isLoading && !krByMissionQuery.data)

  const { nodes, edges } = useMemo(() => {
    const nodes: Node[] = []
    const edges: Edge[] = []

    missions.forEach((mission) => {
      const krs = krByMission[mission.id] ?? []
      const avgProgress =
        krs.length > 0
          ? Math.round(krs.reduce((sum, kr) => sum + kr.progressPercent, 0) / krs.length)
          : 0

      const missionData: MissionNodeData = {
        mission,
        avgProgress,
        krCount: krs.length,
      }
      nodes.push({
        id: mission.id,
        type: 'mission',
        position: { x: 0, y: 0 }, // overwritten by dagre layout
        data: missionData,
      })

      krs.forEach(kr => {
        const krData: KRNodeData = { kr }
        nodes.push({
          id: kr.id,
          type: 'keyResult',
          position: { x: 0, y: 0 },
          data: krData,
        })
        edges.push({
          id: `${mission.id}-->${kr.id}`,
          source: mission.id,
          target: kr.id,
          type: 'statusEdge',
          animated: false,
          data: { status: kr.status },
        })
      })
    })

    return { nodes, edges }
  }, [missions, krByMission])

  return { nodes, edges, isLoading }
}
