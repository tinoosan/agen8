import { useMemo } from 'react'
import type { Node, Edge } from '@xyflow/react'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'
import type { DecisionNodeData } from './DecisionNode'
import type { TaskNodeData } from './TaskNode'

/**
 * Fetches leaf-level graph nodes: Decisions and Tasks.
 *
 * Edge routing:
 *   Decision   → KR (keyResultRef) OR Mission (missionRef)  — denormalised at write time
 *   Task       → KR (keyResultRef) OR Mission (missionRef)  — KR preferred when both
 * Nodes without a valid parent edge are not shown to keep the graph clean.
 */
export function useLeafNodes(projectId: string | null): {
  nodes: Node[]
  edges: Edge[]
  isLoading: boolean
} {
  const tasksQuery = useProjectTasks(projectId)
  const tasks = useMemo(() => tasksQuery.data ?? [], [tasksQuery.data])


  // Index tasks by ID so decisions can find their parent task.
  // Tasks with a keyResultRef OR a missionRef are in the graph — the
  // deepest-path rule checks membership here before preferring the task
  // node as parent.
  const taskMap = useMemo(() => {
    const m = new Map<string, (typeof tasks)[number]>()
    for (const t of tasks) m.set(t.id, t)
    return m
  }, [tasks])

  const decisionsQuery = useRecentDecisions(projectId, undefined, { refetchInterval: 30_000 })
  const decisions = useMemo(() => decisionsQuery.data ?? [], [decisionsQuery.data])

  const isLoading =
    (tasksQuery.isLoading && !tasksQuery.data) ||
    (decisionsQuery.isLoading && !decisionsQuery.data)


  const { nodes, edges } = useMemo(() => {
    const nodes: Node[] = []
    const edges: Edge[] = []
    const addedTaskIds = new Set<string>()

    const ensureTaskNode = (taskId: string): string | undefined => {
      const nodeId = `task:${taskId}`
      if (addedTaskIds.has(taskId)) return nodeId
      const task = taskMap.get(taskId)
      if (!task) return undefined
      const taskData: TaskNodeData = { task }
      nodes.push({
        id: nodeId,
        type: 'task',
        position: { x: 0, y: 0 },
        data: taskData,
      })
      addedTaskIds.add(taskId)
      // Attach to the KR when present, otherwise straight to the mission.
      // Either parent keeps the task reachable from a mission by the BFS
      // cluster walk so it inherits a cluster colour.
      const parentRef = task.keyResultRef ?? task.missionRef
      if (parentRef) {
        edges.push({
          id: `${nodeId}-->${parentRef}`,
          source: parentRef,
          target: nodeId,
          type: 'statusEdge',
          animated: false,
          data: { status: 'open' },
        })
      }
      return nodeId
    }

    // ── Tasks ───────────────────────────────────────────────────────────────
    // Eagerly add tasks that have a parent ref (KR or mission). Tasks with
    // neither are added on-demand via ensureTaskNode when referenced by a
    // decision (and stay colourless orphans).
    for (const task of tasks) {
      if (!task.keyResultRef && !task.missionRef) continue
      ensureTaskNode(task.id)
    }

    // Resolves a leaf's structural parent for graph attachment. Preference
    // order is chosen so the parent is guaranteed reachable from a mission
    // by the BFS cluster-meta walk — otherwise the leaf never gets a
    // clusterColor and its selection outline/edge highlighting falls back
    // to neutral gray:
    //
    //   1. Task with a kr/mission ref — deepest path, decision visually sits
    //                                  near its task, cluster via task→parent
    //   2. Direct keyResultRef       — cluster via kr→mission
    //   3. Direct missionRef         — cluster directly from mission
    //   4. Orphan task (no kr/mission) — last resort, leaf will be colourless
    //
    // If the leaf has a taskRef pointing to an *orphan* task, we skip the
    // task in favour of any direct KR/mission reference. This keeps the
    // leaf in the BFS tree while still letting ensureTaskNode render the
    // orphan task visually (it just won't be the structural parent).
    const resolveLeafParent = (
      taskRef: string | undefined,
      keyResultRef: string | undefined,
      missionRef?: string,
    ): string | undefined => {
      // Prefer the task only if it has a parent ref (KR or mission) — an
      // orphan task has no cluster identity, so a decision attached to it
      // would lose its colour. A mission-linked task is now reachable, so it
      // qualifies as a structural parent too.
      if (taskRef) {
        const linkedTask = taskMap.get(taskRef)
        if (linkedTask?.keyResultRef || linkedTask?.missionRef) {
          return ensureTaskNode(taskRef)
        }
      }
      if (keyResultRef) return keyResultRef
      if (missionRef) return missionRef
      // Last resort: render against the orphan task (if any) so the leaf
      // isn't dropped entirely, even though it won't get a cluster colour.
      if (taskRef) return ensureTaskNode(taskRef)
      return undefined
    }

    // ── Decisions ──────────────────────────────────────────────────────────
    for (const decision of decisions) {
      const targetId = resolveLeafParent(
        decision.taskRef,
        decision.keyResultRef,
        decision.missionRef,
      )
      if (!targetId) continue

      const nodeData: DecisionNodeData = { decision }
      nodes.push({
        id: `decision:${decision.id}`,
        type: 'decision',
        position: { x: 0, y: 0 },
        data: nodeData,
      })
      edges.push({
        id: `decision:${decision.id}-->${targetId}`,
        source: targetId,
        target: `decision:${decision.id}`,
        type: 'statusEdge',
        animated: false,
        data: { status: 'open' },
      })
    }

    return { nodes, edges }
  }, [decisions, tasks, taskMap])


  return { nodes, edges, isLoading }
}
